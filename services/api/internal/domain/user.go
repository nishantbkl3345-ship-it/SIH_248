// Package domain holds the entities shared across the API. It imports no web
// or database framework; persistence mapping is expressed only through tags.
package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrEmailTaken = errors.New("email already in use")
)

type Role string

const (
	RoleAdmin      Role = "ADMIN"
	RoleInstructor Role = "INSTRUCTOR"
	RoleTrainee    Role = "TRAINEE"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleInstructor, RoleTrainee:
		return true
	}
	return false
}

type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string
	PasswordHash string
	DisplayName  string
	Role         Role
	IsActive     bool
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NormalizeEmail is the canonical form emails are stored and looked up in.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
