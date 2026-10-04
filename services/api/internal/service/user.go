package service

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/auth"
	"fogline/api/internal/domain"
	"fogline/api/internal/repository"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 200
)

// UserService is the admin-facing user management use-case.
type UserService struct {
	users  repository.UserRepository
	hasher *auth.PasswordHasher
}

func NewUserService(users repository.UserRepository, hasher *auth.PasswordHasher) *UserService {
	return &UserService{users: users, hasher: hasher}
}

func (s *UserService) List(ctx context.Context, limit, offset int) ([]domain.User, int64, error) {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	limit = min(limit, MaxPageSize)
	offset = max(offset, 0)

	users, total, err := s.users.List(ctx, limit, offset)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	return users, total, nil
}

func (s *UserService) Create(ctx context.Context, email, password, displayName string, role domain.Role) (*domain.User, error) {
	if !role.Valid() {
		return nil, apperr.Validation(map[string]string{"role": "must be ADMIN, INSTRUCTOR or TRAINEE"})
	}
	return createUser(ctx, s.users, s.hasher, email, password, displayName, role)
}

type UserPatch struct {
	DisplayName *string
	Role        *domain.Role
	IsActive    *bool
}

// Update applies patch to the target user on behalf of actorID. An admin may
// not demote or deactivate their own account, which would risk locking every
// admin out.
func (s *UserService) Update(ctx context.Context, actorID, targetID uuid.UUID, patch UserPatch) (*domain.User, error) {
	user, err := s.users.FindByID(ctx, targetID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, apperr.NotFound("User not found.")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if patch.Role != nil && !patch.Role.Valid() {
		return nil, apperr.Validation(map[string]string{"role": "must be ADMIN, INSTRUCTOR or TRAINEE"})
	}
	if actorID == targetID {
		if patch.Role != nil && *patch.Role != user.Role {
			return nil, apperr.Conflict("SELF_MODIFICATION", "You cannot change your own role.")
		}
		if patch.IsActive != nil && !*patch.IsActive {
			return nil, apperr.Conflict("SELF_MODIFICATION", "You cannot deactivate your own account.")
		}
	}

	if patch.DisplayName != nil {
		user.DisplayName = strings.TrimSpace(*patch.DisplayName)
	}
	if patch.Role != nil {
		user.Role = *patch.Role
	}
	if patch.IsActive != nil {
		user.IsActive = *patch.IsActive
	}

	if err := s.users.Update(ctx, user); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, apperr.NotFound("User not found.")
		}
		return nil, apperr.Internal(err)
	}
	return user, nil
}

// EnsureAdmin creates an ADMIN with the given credentials unless a user with
// that email already exists. It reports whether a user was created.
func (s *UserService) EnsureAdmin(ctx context.Context, email, password string) (bool, error) {
	_, err := s.users.FindByEmail(ctx, domain.NormalizeEmail(email))
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return false, err
	}
	if _, err := createUser(ctx, s.users, s.hasher, email, password, "Administrator", domain.RoleAdmin); err != nil {
		return false, err
	}
	return true, nil
}
