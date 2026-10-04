// Package repository declares the persistence interfaces the services depend on.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

// UserRepository returns domain.ErrNotFound for missing rows and
// domain.ErrEmailTaken when an email is already registered.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	List(ctx context.Context, limit, offset int) ([]domain.User, int64, error)
	Update(ctx context.Context, user *domain.User) error
	// TouchLastLogin sets only last_login_at, so it cannot overwrite a
	// concurrent admin edit of the same user.
	TouchLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error
}

// ScenarioRepository persists a scenario as one aggregate: the scenario row
// plus its channels, phases, reports, rules, decision points and options.
type ScenarioRepository interface {
	// Create inserts a new scenario and everything nested in it.
	Create(ctx context.Context, s *domain.Scenario) error
	// Get returns the full aggregate, children in display order.
	Get(ctx context.Context, id uuid.UUID) (*domain.Scenario, error)
	// List returns scenarios with their phases only, most recently updated
	// first. A nil ownerID lists every owner's scenarios.
	List(ctx context.Context, ownerID *uuid.UUID) ([]domain.Scenario, error)
	// Replace overwrites a DRAFT scenario's content in one transaction and
	// increments its revision. It returns domain.ErrRevisionConflict when
	// expectedRevision is stale, domain.ErrNotDraft when the scenario is not
	// a draft, and domain.ErrIDConflict when a child id belongs elsewhere.
	Replace(ctx context.Context, s *domain.Scenario, expectedRevision int) error
	// SetStatus moves a scenario from one status to another, returning
	// domain.ErrNotDraft-style mismatch as domain.ErrRevisionConflict.
	SetStatus(ctx context.Context, id uuid.UUID, from, to domain.ScenarioStatus, publishedAt *time.Time) error
	Delete(ctx context.Context, id uuid.UUID) error
	// CountSessions reports how many exercise sessions use the scenario.
	CountSessions(ctx context.Context, id uuid.UUID) (int64, error)
}

// SessionRecords is one batch of exercise history to store atomically.
type SessionRecords struct {
	Events    []domain.TimelineEvent
	Decisions []domain.Decision
	Messages  []domain.Message
}

// SessionProgress is the mutable summary kept on the session row.
type SessionProgress struct {
	Status     domain.SessionStatus
	SimMs      int64
	SpeedMilli int
	// StartedAt and EndedAt are recorded the first time they are supplied
	// and never overwritten.
	StartedAt *time.Time
	EndedAt   *time.Time
}

// SessionRepository persists exercise sessions and their history.
type SessionRepository interface {
	// Create inserts a session and its teams. It returns
	// domain.ErrJoinCodeTaken if the join code is already in use.
	Create(ctx context.Context, s *domain.ExerciseSession) error
	// Get returns a session with teams (by number) and participants (by join
	// time), each participant with its user.
	Get(ctx context.Context, id uuid.UUID) (*domain.ExerciseSession, error)
	FindByJoinCode(ctx context.Context, code string) (*domain.ExerciseSession, error)
	// ListForInstructor lists newest first; nil lists every instructor's.
	ListForInstructor(ctx context.Context, instructorID *uuid.UUID) ([]domain.ExerciseSession, error)
	// ListForUser lists sessions the user has joined, newest first.
	ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.ExerciseSession, error)
	// AddParticipant returns domain.ErrAlreadyJoined for a repeat join.
	AddParticipant(ctx context.Context, p *domain.SessionParticipant) error
	SetParticipantTeam(ctx context.Context, participantID, teamID uuid.UUID) error
	UpdateProgress(ctx context.Context, id uuid.UUID, p SessionProgress) error
	// AppendRecords stores a batch in one transaction. History is append-only.
	AppendRecords(ctx context.Context, r SessionRecords) error
	// Timeline returns events with seq > afterSeq, in order.
	Timeline(ctx context.Context, sessionID uuid.UUID, afterSeq int64, limit int) ([]domain.TimelineEvent, error)
	// InterruptLive marks sessions left RUNNING or PAUSED as ENDED and
	// reports how many there were.
	InterruptLive(ctx context.Context, at time.Time) (int64, error)
	// Delete removes a session that has no recorded history.
	Delete(ctx context.Context, id uuid.UUID) error
}
