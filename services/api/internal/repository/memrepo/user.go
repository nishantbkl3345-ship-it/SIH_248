// Package memrepo is an in-memory implementation of the repository interfaces
// for tests that should not need a database.
package memrepo

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

type UserRepository struct {
	mu    sync.RWMutex
	users map[uuid.UUID]domain.User
}

func NewUserRepository() *UserRepository {
	return &UserRepository{users: make(map[uuid.UUID]domain.User)}
}

func (r *UserRepository) Create(_ context.Context, user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if u.Email == user.Email {
			return domain.ErrEmailTaken
		}
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now()
	user.CreatedAt, user.UpdatedAt = now, now
	r.users[user.ID] = *user
	return nil
}

func (r *UserRepository) FindByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (r *UserRepository) List(_ context.Context, limit, offset int) ([]domain.User, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	all := make([]domain.User, 0, len(r.users))
	for _, u := range r.users {
		all = append(all, u)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID.String() < all[j].ID.String()
		}
		return all[i].CreatedAt.Before(all[j].CreatedAt)
	})
	total := int64(len(all))
	if offset >= len(all) {
		return []domain.User{}, total, nil
	}
	end := min(offset+limit, len(all))
	return all[offset:end], total, nil
}

func (r *UserRepository) Update(_ context.Context, user *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.users[user.ID]; !ok {
		return domain.ErrNotFound
	}
	user.UpdatedAt = time.Now()
	r.users[user.ID] = *user
	return nil
}

func (r *UserRepository) TouchLastLogin(_ context.Context, id uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.LastLoginAt = &at
	r.users[id] = u
	return nil
}
