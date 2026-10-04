package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/service"
)

// ---------------------------------------------------------------- responses

type userResponse struct {
	ID          uuid.UUID   `json:"id"`
	Email       string      `json:"email"`
	DisplayName string      `json:"displayName"`
	Role        domain.Role `json:"role"`
	IsActive    bool        `json:"isActive"`
	LastLoginAt *time.Time  `json:"lastLoginAt"`
	CreatedAt   time.Time   `json:"createdAt"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Role:        u.Role,
		IsActive:    u.IsActive,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}
}

type authResponse struct {
	User        userResponse `json:"user"`
	AccessToken string       `json:"accessToken"`
	ExpiresAt   time.Time    `json:"expiresAt"`
}

// ------------------------------------------------------------------- health

type healthHandler struct {
	ping func(context.Context) error
}

func (h *healthHandler) live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *healthHandler) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "database": "down"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "database": "up"})
}

// --------------------------------------------------------------------- auth

type authHandler struct {
	auth         *service.AuthService
	cookieSecure bool
}

type registerRequest struct {
	Email       string `json:"email" binding:"required,email,max=254"`
	Password    string `json:"password" binding:"required,min=8,max=72"`
	DisplayName string `json:"displayName" binding:"required,min=1,max=80"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,max=72"`
}

func (h *authHandler) register(c *gin.Context) {
	var req registerRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.auth.Register(c.Request.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	h.respond(c, http.StatusCreated, res)
}

func (h *authHandler) login(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.auth.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		_ = c.Error(err)
		return
	}
	h.respond(c, http.StatusOK, res)
}

func (h *authHandler) logout(c *gin.Context) {
	h.setCookie(c, "", -1)
	c.Status(http.StatusNoContent)
}

func (h *authHandler) me(c *gin.Context) {
	user, _ := currentUser(c)
	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(user)})
}

func (h *authHandler) respond(c *gin.Context, status int, res *service.AuthResult) {
	h.setCookie(c, res.AccessToken, int(time.Until(res.ExpiresAt).Seconds()))
	c.JSON(status, authResponse{
		User:        toUserResponse(res.User),
		AccessToken: res.AccessToken,
		ExpiresAt:   res.ExpiresAt,
	})
}

func (h *authHandler) setCookie(c *gin.Context, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     SessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// -------------------------------------------------------------- admin users

type adminUserHandler struct {
	users *service.UserService
}

type createUserRequest struct {
	Email       string `json:"email" binding:"required,email,max=254"`
	Password    string `json:"password" binding:"required,min=8,max=72"`
	DisplayName string `json:"displayName" binding:"required,min=1,max=80"`
	Role        string `json:"role" binding:"required,oneof=ADMIN INSTRUCTOR TRAINEE"`
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName" binding:"omitempty,min=1,max=80"`
	Role        *string `json:"role" binding:"omitempty,oneof=ADMIN INSTRUCTOR TRAINEE"`
	IsActive    *bool   `json:"isActive"`
}

func (h *adminUserHandler) list(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))

	users, total, err := h.users.List(c.Request.Context(), limit, offset)
	if err != nil {
		_ = c.Error(err)
		return
	}
	out := make([]userResponse, len(users))
	for i := range users {
		out[i] = toUserResponse(&users[i])
	}
	c.JSON(http.StatusOK, gin.H{"users": out, "total": total})
}

func (h *adminUserHandler) create(c *gin.Context) {
	var req createUserRequest
	if !bindJSON(c, &req) {
		return
	}
	user, err := h.users.Create(c.Request.Context(), req.Email, req.Password, req.DisplayName, domain.Role(req.Role))
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": toUserResponse(user)})
}

func (h *adminUserHandler) update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperr.NotFound("User not found."))
		return
	}
	var req updateUserRequest
	if !bindJSON(c, &req) {
		return
	}

	patch := service.UserPatch{DisplayName: req.DisplayName, IsActive: req.IsActive}
	if req.Role != nil {
		role := domain.Role(*req.Role)
		patch.Role = &role
	}

	actor, _ := currentUser(c)
	user, err := h.users.Update(c.Request.Context(), actor.ID, id, patch)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": toUserResponse(user)})
}
