package gormrepo

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"fogline/api/internal/domain"
)

type ScenarioRepository struct {
	db *gorm.DB
}

func NewScenarioRepository(db *gorm.DB) *ScenarioRepository {
	return &ScenarioRepository{db: db}
}

func (r *ScenarioRepository) Create(ctx context.Context, s *domain.Scenario) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit(clause.Associations).Create(s).Error; err != nil {
			return mapScenarioErr(err)
		}
		return insertChildren(tx, s)
	})
}

func (r *ScenarioRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Scenario, error) {
	var s domain.Scenario
	err := r.db.WithContext(ctx).
		Preload("Channels", orderBy("ord, id")).
		Preload("Phases", orderBy("ord")).
		Preload("Phases.Reports", orderBy("ord, id")).
		Preload("Phases.Rules", orderBy("ord, id")).
		Preload("Phases.DecisionPoints", orderBy("ord, id")).
		Preload("Phases.DecisionPoints.Options", orderBy("ord")).
		First(&s, "id = ?", id).Error
	if err != nil {
		return nil, mapScenarioErr(err)
	}
	return &s, nil
}

func (r *ScenarioRepository) List(ctx context.Context, ownerID *uuid.UUID) ([]domain.Scenario, error) {
	q := r.db.WithContext(ctx).Preload("Phases", orderBy("ord")).Order("updated_at DESC, id")
	if ownerID != nil {
		q = q.Where("owner_id = ?", *ownerID)
	}
	var out []domain.Scenario
	return out, q.Find(&out).Error
}

func (r *ScenarioRepository) Replace(ctx context.Context, s *domain.Scenario, expectedRevision int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// The row lock serialises concurrent saves of the same scenario.
		var cur domain.Scenario
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("id", "status", "revision").
			First(&cur, "id = ?", s.ID).Error
		if err != nil {
			return mapScenarioErr(err)
		}
		if cur.Status != domain.ScenarioDraft {
			return domain.ErrNotDraft
		}
		if cur.Revision != expectedRevision {
			return domain.ErrRevisionConflict
		}

		// Deletes are scoped to this scenario, so they can never touch
		// another scenario's rows whatever ids the caller supplied.
		for _, model := range []any{
			&domain.DecisionPoint{}, // options cascade
			&domain.DegradationRule{},
			&domain.InformationReport{},
			&domain.ScenarioPhase{},
			&domain.CommunicationChannel{},
		} {
			if err := tx.Where("scenario_id = ?", s.ID).Delete(model).Error; err != nil {
				return err
			}
		}

		now := time.Now()
		s.Revision = expectedRevision + 1
		s.UpdatedAt = now
		err = tx.Model(&domain.Scenario{}).Where("id = ?", s.ID).Updates(map[string]any{
			"title":            s.Title,
			"summary":          s.Summary,
			"objectives":       s.Objectives,
			"difficulty":       s.Difficulty,
			"est_duration_sec": s.EstDurationSec,
			"team_count":       s.TeamCount,
			"trainee_count":    s.TraineeCount,
			"revision":         s.Revision,
			"updated_at":       now,
		}).Error
		if err != nil {
			return err
		}
		return insertChildren(tx, s)
	})
}

func (r *ScenarioRepository) SetStatus(ctx context.Context, id uuid.UUID, from, to domain.ScenarioStatus, publishedAt *time.Time) error {
	res := r.db.WithContext(ctx).Model(&domain.Scenario{}).
		Where("id = ? AND status = ?", id, from).
		Updates(map[string]any{
			"status":       to,
			"published_at": publishedAt,
			"revision":     gorm.Expr("revision + 1"),
			"updated_at":   time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrRevisionConflict
	}
	return nil
}

func (r *ScenarioRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&domain.Scenario{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *ScenarioRepository) CountSessions(ctx context.Context, id uuid.UUID) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.ExerciseSession{}).Where("scenario_id = ?", id).Count(&n).Error
	return n, err
}

// insertChildren writes every nested row with a plain INSERT. GORM's
// association auto-save is avoided on purpose: it upserts on the primary key,
// which would let a caller-supplied id take over a row in another scenario.
func insertChildren(tx *gorm.DB, s *domain.Scenario) error {
	var (
		reports []domain.InformationReport
		rules   []domain.DegradationRule
		points  []domain.DecisionPoint
		options []domain.DecisionOption
	)
	for _, p := range s.Phases {
		reports = append(reports, p.Reports...)
		rules = append(rules, p.Rules...)
		points = append(points, p.DecisionPoints...)
		for _, d := range p.DecisionPoints {
			options = append(options, d.Options...)
		}
	}

	insert := func(rows any, n int) error {
		if n == 0 {
			return nil
		}
		return mapScenarioErr(tx.Omit(clause.Associations).Create(rows).Error)
	}
	if err := insert(&s.Channels, len(s.Channels)); err != nil {
		return err
	}
	if err := insert(&s.Phases, len(s.Phases)); err != nil {
		return err
	}
	if err := insert(&reports, len(reports)); err != nil {
		return err
	}
	if err := insert(&rules, len(rules)); err != nil {
		return err
	}
	if err := insert(&points, len(points)); err != nil {
		return err
	}
	return insert(&options, len(options))
}

func orderBy(columns string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB { return db.Order(columns) }
}

func mapScenarioErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		return domain.ErrIDConflict
	}
	return err
}
