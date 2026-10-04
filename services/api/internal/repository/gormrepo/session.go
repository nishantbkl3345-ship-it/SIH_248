package gormrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fogline/api/internal/domain"
	"fogline/api/internal/repository"
)

type SessionRepository struct {
	db *gorm.DB
}

func NewSessionRepository(db *gorm.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func uniqueViolation(err error, constraintHint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && strings.Contains(pgErr.ConstraintName, constraintHint)
}

func (r *SessionRepository) Create(ctx context.Context, s *domain.ExerciseSession) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(s).Error; err != nil {
			if uniqueViolation(err, "join_code") {
				return domain.ErrJoinCodeTaken
			}
			return err
		}
		for i := range s.Teams {
			s.Teams[i].SessionID = s.ID
		}
		if len(s.Teams) == 0 {
			return nil
		}
		return tx.Create(&s.Teams).Error
	})
}

func (r *SessionRepository) Get(ctx context.Context, id uuid.UUID) (*domain.ExerciseSession, error) {
	var s domain.ExerciseSession
	err := r.db.WithContext(ctx).
		Preload("Teams", orderBy("number")).
		Preload("Participants", orderBy("joined_at, id")).
		Preload("Participants.User").
		First(&s, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepository) FindByJoinCode(ctx context.Context, code string) (*domain.ExerciseSession, error) {
	var s domain.ExerciseSession
	err := r.db.WithContext(ctx).Select("id").First(&s, "join_code = ?", code).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, s.ID)
}

func (r *SessionRepository) ListForInstructor(ctx context.Context, instructorID *uuid.UUID) ([]domain.ExerciseSession, error) {
	q := r.db.WithContext(ctx).Order("created_at DESC, id")
	if instructorID != nil {
		q = q.Where("instructor_id = ?", *instructorID)
	}
	var out []domain.ExerciseSession
	return out, q.Find(&out).Error
}

func (r *SessionRepository) ListForUser(ctx context.Context, userID uuid.UUID) ([]domain.ExerciseSession, error) {
	var out []domain.ExerciseSession
	err := r.db.WithContext(ctx).
		Where("id IN (?)", r.db.Model(&domain.SessionParticipant{}).Select("session_id").Where("user_id = ?", userID)).
		Order("created_at DESC, id").
		Find(&out).Error
	return out, err
}

func (r *SessionRepository) AddParticipant(ctx context.Context, p *domain.SessionParticipant) error {
	err := r.db.WithContext(ctx).Omit(clause.Associations).Create(p).Error
	if uniqueViolation(err, "session_participants") {
		return domain.ErrAlreadyJoined
	}
	return err
}

func (r *SessionRepository) SetParticipantTeam(ctx context.Context, participantID, teamID uuid.UUID) error {
	res := r.db.WithContext(ctx).Model(&domain.SessionParticipant{}).
		Where("id = ?", participantID).
		Updates(map[string]any{"team_id": teamID, "updated_at": time.Now()})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SessionRepository) UpdateProgress(ctx context.Context, id uuid.UUID, p repository.SessionProgress) error {
	fields := map[string]any{
		"status":      p.Status,
		"sim_ms":      p.SimMs,
		"speed_milli": p.SpeedMilli,
		"updated_at":  time.Now(),
	}
	// Both are set once and then kept.
	if p.StartedAt != nil {
		fields["started_at"] = gorm.Expr("COALESCE(started_at, ?)", *p.StartedAt)
	}
	if p.EndedAt != nil {
		fields["ended_at"] = gorm.Expr("COALESCE(ended_at, ?)", *p.EndedAt)
	}
	res := r.db.WithContext(ctx).Model(&domain.ExerciseSession{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *SessionRepository) AppendRecords(ctx context.Context, rec repository.SessionRecords) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(rec.Messages) > 0 {
			if err := tx.Create(&rec.Messages).Error; err != nil {
				return err
			}
		}
		if len(rec.Decisions) > 0 {
			if err := tx.Create(&rec.Decisions).Error; err != nil {
				return err
			}
		}
		if len(rec.Events) > 0 {
			if err := tx.Create(&rec.Events).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SessionRepository) Timeline(ctx context.Context, sessionID uuid.UUID, afterSeq int64, limit int) ([]domain.TimelineEvent, error) {
	var out []domain.TimelineEvent
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND seq > ?", sessionID, afterSeq).
		Order("seq").Limit(limit).
		Find(&out).Error
	return out, err
}

func (r *SessionRepository) InterruptLive(ctx context.Context, at time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Model(&domain.ExerciseSession{}).
		Where("status IN ?", []domain.SessionStatus{domain.SessionRunning, domain.SessionPaused}).
		Updates(map[string]any{"status": domain.SessionEnded, "ended_at": at, "updated_at": at})
	return res.RowsAffected, res.Error
}

func (r *SessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&domain.ExerciseSession{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}
