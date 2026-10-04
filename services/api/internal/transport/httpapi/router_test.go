package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"fogline/api/internal/auth"
	"fogline/api/internal/domain"
	"fogline/api/internal/repository/memrepo"
	"fogline/api/internal/runtime"
	"fogline/api/internal/service"
)

type testAPI struct {
	t      *testing.T
	router *gin.Engine
	users  *service.UserService
	auth   *service.AuthService
	dbDown bool

	// The exercise runtime has no background loop in tests and reads this
	// clock, so simulation time moves only when a test says so.
	clock    *testClock
	live     *runtime.Manager
	sessions *memrepo.SessionRepository
}

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := memrepo.NewUserRepository()
	hasher := auth.NewPasswordHasher(bcrypt.MinCost)
	tokens := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "fogline", time.Hour)
	authSvc, err := service.NewAuthService(repo, hasher, tokens, true)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	scenarios := memrepo.NewScenarioRepository()
	api := &testAPI{
		t: t, users: service.NewUserService(repo, hasher), auth: authSvc,
		clock:    &testClock{now: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)},
		sessions: memrepo.NewSessionRepository(repo),
	}
	api.live = runtime.NewManager(runtime.Options{Store: service.NewSessionStore(api.sessions), Logger: logger, Now: api.clock.Now})
	t.Cleanup(api.live.Shutdown)
	api.router = NewRouter(Deps{
		Logger:    logger,
		Auth:      authSvc,
		Users:     api.users,
		Scenarios: service.NewScenarioService(scenarios),
		Sessions:  service.NewSessionService(api.sessions, scenarios, api.live),
		Ping: func(context.Context) error {
			if api.dbDown {
				return errors.New("down")
			}
			return nil
		},
		AllowedOrigins: []string{"http://localhost:3000"},
	})
	return api
}

type reqOpt func(*http.Request)

func withBearer(token string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }
}

func withCookie(c *http.Cookie) reqOpt {
	return func(r *http.Request) { r.AddCookie(c) }
}

func withHeader(k, v string) reqOpt {
	return func(r *http.Request) { r.Header.Set(k, v) }
}

func (a *testAPI) do(method, path string, body any, opts ...reqOpt) *httptest.ResponseRecorder {
	a.t.Helper()
	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			a.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, want int) {
	t.Helper()
	if rec.Code != want {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, want, rec.Body.String())
	}
}

func wantErrorCode(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) errorPayload {
	t.Helper()
	wantStatus(t, rec, status)
	body := decode[errorBody](t, rec)
	if body.Error.Code != code {
		t.Fatalf("error code = %q, want %q", body.Error.Code, code)
	}
	if body.Error.RequestID == "" {
		t.Error("error response has no requestId")
	}
	return body.Error
}

// tokenFor creates a user with the given role and returns a token for it.
func (a *testAPI) tokenFor(role domain.Role) string {
	a.t.Helper()
	email := strings.ToLower(string(role)) + "@example.test"
	if _, err := a.users.Create(context.Background(), email, "password-123", string(role), role); err != nil {
		a.t.Fatalf("create %s: %v", role, err)
	}
	res, err := a.auth.Login(context.Background(), email, "password-123")
	if err != nil {
		a.t.Fatalf("login %s: %v", role, err)
	}
	return res.AccessToken
}

// login creates a user and returns a bearer token for them.
func (a *testAPI) login(email, name string, role domain.Role) string {
	a.t.Helper()
	if _, err := a.users.Create(context.Background(), email, "password-123", name, role); err != nil {
		a.t.Fatalf("create %s: %v", email, err)
	}
	res, err := a.auth.Login(context.Background(), email, "password-123")
	if err != nil {
		a.t.Fatalf("login %s: %v", email, err)
	}
	return res.AccessToken
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == SessionCookie {
			return c
		}
	}
	return nil
}

func TestHealth(t *testing.T) {
	api := newTestAPI(t)

	wantStatus(t, api.do(http.MethodGet, "/healthz", nil), http.StatusOK)
	wantStatus(t, api.do(http.MethodGet, "/readyz", nil), http.StatusOK)

	api.dbDown = true
	wantStatus(t, api.do(http.MethodGet, "/healthz", nil), http.StatusOK)
	wantStatus(t, api.do(http.MethodGet, "/readyz", nil), http.StatusServiceUnavailable)
}

func TestRegisterLoginMeLogout(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "Mira@example.test", "password": "password-123", "displayName": "Mira",
	})
	wantStatus(t, rec, http.StatusCreated)
	if strings.Contains(rec.Body.String(), "password") {
		t.Errorf("response leaks a password field: %s", rec.Body.String())
	}
	reg := decode[authResponse](t, rec)
	if reg.User.Role != domain.RoleTrainee || reg.User.Email != "mira@example.test" || reg.AccessToken == "" {
		t.Fatalf("register response = %+v", reg)
	}

	rec = api.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "mira@example.test", "password": "password-123",
	})
	wantStatus(t, rec, http.StatusOK)
	login := decode[authResponse](t, rec)
	cookie := sessionCookie(rec)
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Value != login.AccessToken {
		t.Fatalf("session cookie = %+v", cookie)
	}

	// Both the bearer header and the cookie authenticate.
	for name, opt := range map[string]reqOpt{"bearer": withBearer(login.AccessToken), "cookie": withCookie(cookie)} {
		rec = api.do(http.MethodGet, "/api/v1/auth/me", nil, opt)
		wantStatus(t, rec, http.StatusOK)
		me := decode[struct{ User userResponse }](t, rec)
		if me.User.ID != reg.User.ID {
			t.Errorf("%s: me = %+v", name, me.User)
		}
	}

	rec = api.do(http.MethodPost, "/api/v1/auth/logout", nil)
	wantStatus(t, rec, http.StatusNoContent)
	if c := sessionCookie(rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("logout did not clear the cookie: %+v", c)
	}
}

func TestAuthFailures(t *testing.T) {
	api := newTestAPI(t)
	api.tokenFor(domain.RoleTrainee)

	rec := api.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "trainee@example.test", "password": "wrong-password",
	})
	wantErrorCode(t, rec, http.StatusUnauthorized, "INVALID_CREDENTIALS")

	rec = api.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "trainee@example.test", "password": "password-123", "displayName": "Dup",
	})
	wantErrorCode(t, rec, http.StatusConflict, "EMAIL_TAKEN")

	wantErrorCode(t, api.do(http.MethodGet, "/api/v1/auth/me", nil), http.StatusUnauthorized, "UNAUTHENTICATED")
	wantErrorCode(t, api.do(http.MethodGet, "/api/v1/auth/me", nil, withBearer("junk")), http.StatusUnauthorized, "UNAUTHENTICATED")
	wantErrorCode(t, api.do(http.MethodGet, "/api/v1/auth/me", nil, withHeader("Authorization", "Basic abc")), http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestValidation(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "not-an-email", "password": "short",
	})
	e := wantErrorCode(t, rec, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	for _, field := range []string{"email", "password", "displayName"} {
		if e.Details[field] == "" {
			t.Errorf("missing validation detail for %q: %v", field, e.Details)
		}
	}

	rec = api.do(http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "a@example.test", "password": strings.Repeat("p", 73), "displayName": "A",
	})
	wantErrorCode(t, rec, http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/auth/login", "{not json"), http.StatusBadRequest, "INVALID_JSON")
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/auth/login", ""), http.StatusBadRequest, "INVALID_JSON")
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/auth/login", `{"email": 5, "password": "x"}`), http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	big := `{"email":"a@example.test","password":"` + strings.Repeat("p", maxBodyBytes) + `"}`
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/auth/login", big), http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE")
}

func TestRBACMatrix(t *testing.T) {
	api := newTestAPI(t)
	tokens := map[string]string{
		"anonymous":  "",
		"TRAINEE":    api.tokenFor(domain.RoleTrainee),
		"INSTRUCTOR": api.tokenFor(domain.RoleInstructor),
		"ADMIN":      api.tokenFor(domain.RoleAdmin),
	}

	routes := []struct {
		method, path string
		body         any
		want         map[string]int
	}{
		{http.MethodGet, "/api/v1/auth/me", nil, map[string]int{
			"anonymous": 401, "TRAINEE": 200, "INSTRUCTOR": 200, "ADMIN": 200,
		}},
		{http.MethodGet, "/api/v1/admin/users", nil, map[string]int{
			"anonymous": 401, "TRAINEE": 403, "INSTRUCTOR": 403, "ADMIN": 200,
		}},
		{http.MethodPost, "/api/v1/admin/users", map[string]string{}, map[string]int{
			"anonymous": 401, "TRAINEE": 403, "INSTRUCTOR": 403, "ADMIN": 422,
		}},
		{http.MethodPatch, "/api/v1/admin/users/00000000-0000-0000-0000-000000000000", map[string]string{}, map[string]int{
			"anonymous": 401, "TRAINEE": 403, "INSTRUCTOR": 403, "ADMIN": 404,
		}},
	}

	for _, rt := range routes {
		for who, want := range rt.want {
			t.Run(rt.method+" "+rt.path+" as "+who, func(t *testing.T) {
				var opts []reqOpt
				if tokens[who] != "" {
					opts = append(opts, withBearer(tokens[who]))
				}
				wantStatus(t, api.do(rt.method, rt.path, rt.body, opts...), want)
			})
		}
	}
}

func TestAdminUserManagement(t *testing.T) {
	api := newTestAPI(t)
	admin := withBearer(api.tokenFor(domain.RoleAdmin))

	rec := api.do(http.MethodPost, "/api/v1/admin/users", map[string]string{
		"email": "lead@example.test", "password": "password-123", "displayName": "Lead", "role": "INSTRUCTOR",
	}, admin)
	wantStatus(t, rec, http.StatusCreated)
	created := decode[struct{ User userResponse }](t, rec).User
	if created.Role != domain.RoleInstructor || !created.IsActive {
		t.Fatalf("created = %+v", created)
	}

	rec = api.do(http.MethodPost, "/api/v1/admin/users", map[string]string{
		"email": "x@example.test", "password": "password-123", "displayName": "X", "role": "ROOT",
	}, admin)
	wantErrorCode(t, rec, http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	// Deactivating the instructor invalidates the token they already hold.
	login := decode[authResponse](t, api.do(http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "lead@example.test", "password": "password-123",
	}))
	rec = api.do(http.MethodPatch, "/api/v1/admin/users/"+created.ID.String(), map[string]any{"isActive": false}, admin)
	wantStatus(t, rec, http.StatusOK)
	wantErrorCode(t, api.do(http.MethodGet, "/api/v1/auth/me", nil, withBearer(login.AccessToken)), http.StatusUnauthorized, "UNAUTHENTICATED")

	rec = api.do(http.MethodGet, "/api/v1/admin/users?limit=1", nil, admin)
	wantStatus(t, rec, http.StatusOK)
	list := decode[struct {
		Users []userResponse
		Total int64
	}](t, rec)
	if list.Total != 2 || len(list.Users) != 1 {
		t.Fatalf("list = %+v", list)
	}

	wantErrorCode(t, api.do(http.MethodPatch, "/api/v1/admin/users/not-a-uuid", map[string]any{}, admin), http.StatusNotFound, "NOT_FOUND")
}

func TestRoutingErrorsUseStandardShape(t *testing.T) {
	api := newTestAPI(t)
	wantErrorCode(t, api.do(http.MethodGet, "/api/v1/nope", nil), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodDelete, "/api/v1/auth/login", nil), http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
}

func TestPanicIsRecovered(t *testing.T) {
	api := newTestAPI(t)
	api.router.GET("/boom", func(*gin.Context) { panic("kaboom") })

	rec := api.do(http.MethodGet, "/boom", nil)
	e := wantErrorCode(t, rec, http.StatusInternalServerError, "INTERNAL")
	if strings.Contains(e.Message, "kaboom") {
		t.Error("panic value leaked to the client")
	}
}

func TestRequestID(t *testing.T) {
	api := newTestAPI(t)
	if got := api.do(http.MethodGet, "/healthz", nil).Header().Get(headerRequestID); got == "" {
		t.Error("no request id generated")
	}
	rec := api.do(http.MethodGet, "/healthz", nil, withHeader(headerRequestID, "abc-123"))
	if got := rec.Header().Get(headerRequestID); got != "abc-123" {
		t.Errorf("request id = %q, want the caller's", got)
	}
}

func TestCORS(t *testing.T) {
	api := newTestAPI(t)

	rec := api.do(http.MethodOptions, "/api/v1/auth/login", nil, withHeader("Origin", "http://localhost:3000"))
	wantStatus(t, rec, http.StatusNoContent)
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" ||
		rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("preflight headers = %v", rec.Header())
	}

	rec = api.do(http.MethodGet, "/healthz", nil, withHeader("Origin", "https://evil.example"))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unlisted origin was allowed: %q", got)
	}
}
