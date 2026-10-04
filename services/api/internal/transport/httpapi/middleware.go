package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/service"
)

const (
	headerRequestID = "X-Request-ID"
	ctxRequestID    = "request_id"
	ctxUser         = "user"

	// SessionCookie carries the access token for browser clients.
	SessionCookie = "fogline_token"

	maxBodyBytes = 1 << 20
)

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
	RequestID string            `json:"requestId"`
}

func writeError(c *gin.Context, e *apperr.Error) {
	c.AbortWithStatusJSON(e.Status, errorBody{Error: errorPayload{
		Code:      e.Code,
		Message:   e.Message,
		Details:   e.Details,
		RequestID: c.GetString(ctxRequestID),
	}})
}

// requestID accepts a caller-supplied id or generates one, and echoes it back.
func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" || len(id) > 64 {
			id = uuid.NewString()
		}
		c.Set(ctxRequestID, id)
		c.Header(headerRequestID, id)
		c.Next()
	}
}

// accessLog writes one structured line per request. Bodies are never logged.
func accessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"request_id", c.GetString(ctxRequestID),
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		}
		if u, ok := currentUser(c); ok {
			attrs = append(attrs, "user_id", u.ID.String())
		}
		switch status := c.Writer.Status(); {
		case status >= 500:
			log.Error("request", attrs...)
		case status >= 400:
			log.Warn("request", attrs...)
		default:
			log.Info("request", attrs...)
		}
	}
}

// recovery turns a panic into a logged 500 in the standard error shape.
func recovery(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic recovered",
					"request_id", c.GetString(ctxRequestID),
					"panic", fmt.Sprint(r),
					"stack", string(debug.Stack()),
				)
				writeError(c, apperr.Internal(nil))
			}
		}()
		c.Next()
	}
}

// errorHandler is the single place errors become HTTP responses. Handlers
// report failures with c.Error(err) and return.
func errorHandler(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		e := apperr.From(c.Errors.Last().Err)
		if e.Status >= http.StatusInternalServerError {
			log.Error("request failed",
				"request_id", c.GetString(ctxRequestID),
				"error", e.Error(),
			)
		}
		if !c.Writer.Written() {
			writeError(c, e)
		}
	}
}

func bodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
		}
		c.Next()
	}
}

// cors allows credentialed requests from the configured origins only.
func cors(allowed []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && slices.Contains(allowed, origin) {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Add("Vary", "Origin")
			if c.Request.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, "+headerRequestID)
				h.Set("Access-Control-Max-Age", "600")
				c.AbortWithStatus(http.StatusNoContent)
				return
			}
		}
		c.Next()
	}
}

// requireAuth resolves the bearer token (header first, then session cookie)
// to an active user and stores it on the context.
func requireAuth(auth *service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			writeError(c, apperr.Unauthorized("UNAUTHENTICATED", "Sign in to continue."))
			return
		}
		user, err := auth.Authenticate(c.Request.Context(), token)
		if err != nil {
			writeError(c, apperr.From(err))
			return
		}
		c.Set(ctxUser, user)
		c.Next()
	}
}

// requireRole must run after requireAuth.
func requireRole(roles ...domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := currentUser(c)
		if !ok {
			writeError(c, apperr.Unauthorized("UNAUTHENTICATED", "Sign in to continue."))
			return
		}
		if !slices.Contains(roles, user.Role) {
			writeError(c, apperr.Forbidden("You do not have permission to do this."))
			return
		}
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		scheme, token, ok := strings.Cut(h, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token)
		}
		return ""
	}
	if cookie, err := c.Cookie(SessionCookie); err == nil {
		return cookie
	}
	return ""
}

func currentUser(c *gin.Context) (*domain.User, bool) {
	v, ok := c.Get(ctxUser)
	if !ok {
		return nil, false
	}
	u, ok := v.(*domain.User)
	return u, ok
}
