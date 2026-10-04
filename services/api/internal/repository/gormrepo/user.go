// Package gormrepo implements the repository interfaces on PostgreSQL via GORM.
package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"fogline/api/internal/domain"
)

const pgUniqueViolation = "23505"

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	return mapUserErr(r.db.WithContext(ctx).Create(user).Error)
}

func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, mapUserErr(err)
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	var user domain.User
	if err := r.db.WithContext(ctx).First(&user, "email = ?", email).Error; err != nil {
		return nil, mapUserErr(err)
	}
	return &user, nil
}

func (r *UserRepository) List(ctx context.Context, limit, offset int) ([]domain.User, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&domain.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []domain.User
	err := r.db.WithContext(ctx).
		Order("created_at ASC, id ASC").
		Limit(limit).Offset(offset).
		Find(&users).Error
	return users, total, err
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	res := r.db.WithContext(ctx).Model(user).Select("*").Omit("id", "created_at").Updates(user)
	if res.Error != nil {
		return mapUserErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) TouchLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error {
	return r.db.WithContext(ctx).Model(&domain.User{}).
		Where("id = ?", id).
		UpdateColumn("last_login_at", at).Error
}

func mapUserErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return domain.ErrEmailTaken
	}
	return err
}
