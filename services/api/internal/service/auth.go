// Package service holds the application use-cases. Authorisation decisions
// that depend on data live here so every transport shares them.
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/auth"
	"fogline/api/internal/domain"
	"fogline/api/internal/repository"
)

// AuthResult is a signed-in user with a fresh access token.
type AuthResult struct {
	User        *domain.User
	AccessToken string
	ExpiresAt   time.Time
}

type AuthService struct {
	users             repository.UserRepository
	hasher            *auth.PasswordHasher
	tokens            *auth.TokenManager
	allowRegistration bool
	// dummyHash is compared against when the email is unknown so that login
	// takes about as long whether or not the account exists.
	dummyHash string
	now       func() time.Time
}

func NewAuthService(users repository.UserRepository, hasher *auth.PasswordHasher, tokens *auth.TokenManager, allowRegistration bool) (*AuthService, error) {
	dummy, err := hasher.Hash(uuid.NewString())
	if err != nil {
		return nil, err
	}
	return &AuthService{
		users:             users,
		hasher:            hasher,
		tokens:            tokens,
		allowRegistration: allowRegistration,
		dummyHash:         dummy,
		now:               time.Now,
	}, nil
}

var errInvalidCredentials = apperr.Unauthorized("INVALID_CREDENTIALS", "Email or password is incorrect.")

// Register creates a self-service account. Self-registration always yields a
// TRAINEE; other roles are assigned by an admin.
func (s *AuthService) Register(ctx context.Context, email, password, displayName string) (*AuthResult, error) {
	if !s.allowRegistration {
		return nil, apperr.Forbidden("Self-registration is disabled. Ask an administrator for an account.")
	}
	user, err := createUser(ctx, s.users, s.hasher, email, password, displayName, domain.RoleTrainee)
	if err != nil {
		return nil, err
	}
	return s.issue(user)
}

func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	user, err := s.users.FindByEmail(ctx, domain.NormalizeEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		s.hasher.Verify(s.dummyHash, password)
		return nil, errInvalidCredentials
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if !s.hasher.Verify(user.PasswordHash, password) {
		return nil, errInvalidCredentials
	}
	if !user.IsActive {
		return nil, apperr.Forbidden("This account has been deactivated.")
	}

	now := s.now()
	if err := s.users.TouchLastLogin(ctx, user.ID, now); err != nil {
		return nil, apperr.Internal(err)
	}
	user.LastLoginAt = &now
	return s.issue(user)
}

// Authenticate resolves an access token to its current, active user. The user
// is re-read on every call so deactivation and role changes apply immediately.
func (s *AuthService) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	unauthenticated := apperr.Unauthorized("UNAUTHENTICATED", "Sign in to continue.")

	claims, err := s.tokens.Parse(token)
	if err != nil {
		return nil, unauthenticated
	}
	id, _ := claims.UserID()
	user, err := s.users.FindByID(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, unauthenticated
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if !user.IsActive {
		return nil, unauthenticated
	}
	return user, nil
}

func (s *AuthService) issue(user *domain.User) (*AuthResult, error) {
	token, expiresAt, err := s.tokens.Issue(user.ID, user.Role)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return &AuthResult{User: user, AccessToken: token, ExpiresAt: expiresAt}, nil
}

func createUser(ctx context.Context, users repository.UserRepository, hasher *auth.PasswordHasher, email, password, displayName string, role domain.Role) (*domain.User, error) {
	hash, err := hasher.Hash(password)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	user := &domain.User{
		Email:        domain.NormalizeEmail(email),
		PasswordHash: hash,
		DisplayName:  strings.TrimSpace(displayName),
		Role:         role,
		IsActive:     true,
	}
	if err := users.Create(ctx, user); err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return nil, apperr.Conflict("EMAIL_TAKEN", "An account with this email already exists.")
		}
		return nil, apperr.Internal(err)
	}
	return user, nil
}
