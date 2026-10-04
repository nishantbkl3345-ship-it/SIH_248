package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"fogline/api/internal/apperr"
	"fogline/api/internal/auth"
	"fogline/api/internal/domain"
	"fogline/api/internal/repository/memrepo"
)

type fixture struct {
	repo  *memrepo.UserRepository
	auth  *AuthService
	users *UserService
}

func newFixture(t *testing.T, allowRegistration bool) fixture {
	t.Helper()
	repo := memrepo.NewUserRepository()
	hasher := auth.NewPasswordHasher(bcrypt.MinCost)
	tokens := auth.NewTokenManager("0123456789abcdef0123456789abcdef", "fogline", time.Hour)
	authSvc, err := NewAuthService(repo, hasher, tokens, allowRegistration)
	if err != nil {
		t.Fatalf("NewAuthService: %v", err)
	}
	return fixture{repo: repo, auth: authSvc, users: NewUserService(repo, hasher)}
}

func wantAppErr(t *testing.T, err error, status int, code string) {
	t.Helper()
	var ae *apperr.Error
	if !errors.As(err, &ae) {
		t.Fatalf("error = %v, want *apperr.Error %d %s", err, status, code)
	}
	if ae.Status != status || ae.Code != code {
		t.Fatalf("error = %d %s, want %d %s", ae.Status, ae.Code, status, code)
	}
}

func TestRegisterCreatesTraineeAndNormalizesEmail(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()

	res, err := f.auth.Register(ctx, "  Mira@Example.TEST ", "password-123", "  Mira  ")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if res.User.Role != domain.RoleTrainee {
		t.Errorf("role = %s, want TRAINEE", res.User.Role)
	}
	if res.User.Email != "mira@example.test" || res.User.DisplayName != "Mira" {
		t.Errorf("user = %+v", res.User)
	}
	if res.User.PasswordHash == "password-123" || res.AccessToken == "" {
		t.Error("password stored in plaintext or token missing")
	}

	_, err = f.auth.Register(ctx, "MIRA@example.test", "password-456", "Other")
	wantAppErr(t, err, http.StatusConflict, "EMAIL_TAKEN")
}

func TestRegisterDisabled(t *testing.T) {
	f := newFixture(t, false)
	_, err := f.auth.Register(context.Background(), "a@example.test", "password-123", "A")
	wantAppErr(t, err, http.StatusForbidden, "FORBIDDEN")
}

func TestLogin(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	if _, err := f.auth.Register(ctx, "a@example.test", "password-123", "A"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	res, err := f.auth.Login(ctx, "A@example.test", "password-123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.User.LastLoginAt == nil {
		t.Error("LastLoginAt was not recorded")
	}

	// Wrong password and unknown email must be indistinguishable.
	_, err = f.auth.Login(ctx, "a@example.test", "wrong-password")
	wantAppErr(t, err, http.StatusUnauthorized, "INVALID_CREDENTIALS")
	_, err = f.auth.Login(ctx, "nobody@example.test", "password-123")
	wantAppErr(t, err, http.StatusUnauthorized, "INVALID_CREDENTIALS")
}

func TestDeactivatedUserCannotLoginOrUseExistingToken(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	admin, err := f.users.Create(ctx, "admin@example.test", "password-123", "Admin", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("Create admin: %v", err)
	}
	res, err := f.auth.Register(ctx, "a@example.test", "password-123", "A")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := f.auth.Authenticate(ctx, res.AccessToken); err != nil {
		t.Fatalf("Authenticate before deactivation: %v", err)
	}

	inactive := false
	if _, err := f.users.Update(ctx, admin.ID, res.User.ID, UserPatch{IsActive: &inactive}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	_, err = f.auth.Authenticate(ctx, res.AccessToken)
	wantAppErr(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
	_, err = f.auth.Login(ctx, "a@example.test", "password-123")
	wantAppErr(t, err, http.StatusForbidden, "FORBIDDEN")
}

func TestAuthenticateRejectsBadToken(t *testing.T) {
	f := newFixture(t, true)
	_, err := f.auth.Authenticate(context.Background(), "nope")
	wantAppErr(t, err, http.StatusUnauthorized, "UNAUTHENTICATED")
}

func TestAdminCannotDemoteOrDeactivateSelf(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	admin, err := f.users.Create(ctx, "admin@example.test", "password-123", "Admin", domain.RoleAdmin)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	trainee, inactive := domain.RoleTrainee, false
	_, err = f.users.Update(ctx, admin.ID, admin.ID, UserPatch{Role: &trainee})
	wantAppErr(t, err, http.StatusConflict, "SELF_MODIFICATION")
	_, err = f.users.Update(ctx, admin.ID, admin.ID, UserPatch{IsActive: &inactive})
	wantAppErr(t, err, http.StatusConflict, "SELF_MODIFICATION")

	name := "Renamed"
	got, err := f.users.Update(ctx, admin.ID, admin.ID, UserPatch{DisplayName: &name})
	if err != nil || got.DisplayName != "Renamed" {
		t.Fatalf("renaming self: user=%+v err=%v", got, err)
	}
}

func TestUpdateChangesRole(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	admin, _ := f.users.Create(ctx, "admin@example.test", "password-123", "Admin", domain.RoleAdmin)
	res, _ := f.auth.Register(ctx, "a@example.test", "password-123", "A")

	instructor := domain.RoleInstructor
	got, err := f.users.Update(ctx, admin.ID, res.User.ID, UserPatch{Role: &instructor})
	if err != nil || got.Role != domain.RoleInstructor {
		t.Fatalf("user=%+v err=%v", got, err)
	}

	bogus := domain.Role("ROOT")
	_, err = f.users.Update(ctx, admin.ID, res.User.ID, UserPatch{Role: &bogus})
	wantAppErr(t, err, http.StatusUnprocessableEntity, "VALIDATION_FAILED")
}

func TestEnsureAdminIsIdempotent(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()

	created, err := f.users.EnsureAdmin(ctx, "Root@example.test", "password-123")
	if err != nil || !created {
		t.Fatalf("first EnsureAdmin: created=%v err=%v", created, err)
	}
	created, err = f.users.EnsureAdmin(ctx, "root@example.test", "different-password")
	if err != nil || created {
		t.Fatalf("second EnsureAdmin: created=%v err=%v", created, err)
	}
	if _, err := f.auth.Login(ctx, "root@example.test", "password-123"); err != nil {
		t.Fatalf("original password should still work: %v", err)
	}
}

func TestListClampsPaging(t *testing.T) {
	f := newFixture(t, true)
	ctx := context.Background()
	for _, e := range []string{"a@example.test", "b@example.test", "c@example.test"} {
		if _, err := f.auth.Register(ctx, e, "password-123", e); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}
	users, total, err := f.users.List(ctx, -5, -1)
	if err != nil || total != 3 || len(users) != 3 {
		t.Fatalf("len=%d total=%d err=%v", len(users), total, err)
	}
	users, _, _ = f.users.List(ctx, 2, 2)
	if len(users) != 1 {
		t.Fatalf("page 2 len = %d, want 1", len(users))
	}
}
