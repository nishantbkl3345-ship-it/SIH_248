// Package httpapi is the REST transport: routing, middleware, request
// validation and response shaping. It contains no business rules.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/service"
)

type Deps struct {
	Logger         *slog.Logger
	Auth           *service.AuthService
	Users          *service.UserService
	Scenarios      *service.ScenarioService
	Sessions       *service.SessionService
	Ping           func(context.Context) error
	CookieSecure   bool
	AllowedOrigins []string
}

func NewRouter(d Deps) *gin.Engine {
	useJSONFieldNames()

	r := gin.New()
	// Client IPs come from the socket unless a proxy is explicitly trusted.
	_ = r.SetTrustedProxies(nil)
	r.HandleMethodNotAllowed = true

	r.Use(
		requestID(),
		accessLog(d.Logger),
		recovery(d.Logger),
		cors(d.AllowedOrigins),
		bodyLimit(),
		errorHandler(d.Logger),
	)

	r.NoRoute(func(c *gin.Context) {
		writeError(c, apperr.NotFound("Route not found."))
	})
	r.NoMethod(func(c *gin.Context) {
		writeError(c, apperr.New(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed."))
	})

	health := &healthHandler{ping: d.Ping}
	r.GET("/healthz", health.live)
	r.GET("/readyz", health.ready)

	v1 := r.Group("/api/v1")

	authH := &authHandler{auth: d.Auth, cookieSecure: d.CookieSecure}
	authed := requireAuth(d.Auth)

	authG := v1.Group("/auth")
	authG.POST("/register", authH.register)
	authG.POST("/login", authH.login)
	authG.POST("/logout", authH.logout)
	authG.GET("/me", authed, authH.me)

	usersH := &adminUserHandler{users: d.Users}
	admin := v1.Group("/admin", authed, requireRole(domain.RoleAdmin))
	admin.GET("/users", usersH.list)
	admin.POST("/users", usersH.create)
	admin.PATCH("/users/:id", usersH.update)

	// Authoring is open to instructors and admins; the service narrows
	// instructors to their own scenarios.
	scenarioH := &scenarioHandler{scenarios: d.Scenarios}
	scenarios := v1.Group("/scenarios", authed, requireRole(domain.RoleInstructor, domain.RoleAdmin))
	scenarios.GET("", scenarioH.list)
	scenarios.POST("", scenarioH.create)
	scenarios.GET("/:id", scenarioH.get)
	scenarios.PUT("/:id", scenarioH.save)
	scenarios.DELETE("/:id", scenarioH.delete)
	scenarios.POST("/:id/publish", scenarioH.publish)
	scenarios.POST("/:id/unpublish", scenarioH.unpublish)

	// Sessions. Roles gate the route; the service checks the caller's
	// relationship to the particular session.
	sessionH := &sessionHandler{sessions: d.Sessions, allowedOrigins: d.AllowedOrigins}
	runs := requireRole(domain.RoleInstructor, domain.RoleAdmin)
	plays := requireRole(domain.RoleTrainee)
	sessions := v1.Group("/sessions", authed)
	sessions.GET("", sessionH.list)
	sessions.POST("", runs, sessionH.create)
	sessions.POST("/join", plays, sessionH.join)
	sessions.GET("/:id", sessionH.get)
	sessions.GET("/:id/state", sessionH.state)
	sessions.GET("/:id/ws", sessionH.stream)
	sessions.GET("/:id/timeline", runs, sessionH.timeline)
	sessions.PATCH("/:id/participants/:pid", runs, sessionH.assignTeam)
	sessions.POST("/:id/start", runs, sessionH.control((*service.SessionService).Start))
	sessions.POST("/:id/pause", runs, sessionH.control((*service.SessionService).Pause))
	sessions.POST("/:id/resume", runs, sessionH.control((*service.SessionService).Resume))
	sessions.POST("/:id/end", runs, sessionH.control((*service.SessionService).End))
	sessions.PATCH("/:id/speed", runs, sessionH.speed)
	sessions.POST("/:id/messages", plays, sessionH.sendMessage)
	sessions.POST("/:id/decisions", plays, sessionH.submitDecision)

	return r
}
