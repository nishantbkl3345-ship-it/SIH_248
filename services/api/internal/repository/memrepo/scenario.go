package memrepo

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

type ScenarioRepository struct {
	mu        sync.RWMutex
	scenarios map[uuid.UUID]domain.Scenario
	// Sessions lets tests mark a scenario as in use.
	Sessions map[uuid.UUID]int64
}

func NewScenarioRepository() *ScenarioRepository {
	return &ScenarioRepository{
		scenarios: make(map[uuid.UUID]domain.Scenario),
		Sessions:  make(map[uuid.UUID]int64),
	}
}

// clone deep-copies through JSON so callers never share slices with the store.
func clone(s domain.Scenario) domain.Scenario {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	var out domain.Scenario
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}

func (r *ScenarioRepository) Create(_ context.Context, s *domain.Scenario) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	now := time.Now()
	s.CreatedAt, s.UpdatedAt = now, now
	r.scenarios[s.ID] = clone(*s)
	return nil
}

func (r *ScenarioRepository) Get(_ context.Context, id uuid.UUID) (*domain.Scenario, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.scenarios[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	out := clone(s)
	return &out, nil
}

func (r *ScenarioRepository) List(_ context.Context, ownerID *uuid.UUID) ([]domain.Scenario, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.Scenario{}
	for _, s := range r.scenarios {
		if ownerID == nil || s.OwnerID == *ownerID {
			out = append(out, clone(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (r *ScenarioRepository) Replace(_ context.Context, s *domain.Scenario, expectedRevision int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.scenarios[s.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if cur.Status != domain.ScenarioDraft {
		return domain.ErrNotDraft
	}
	if cur.Revision != expectedRevision {
		return domain.ErrRevisionConflict
	}
	s.Revision = expectedRevision + 1
	s.UpdatedAt = time.Now()
	next := clone(*s)
	next.OwnerID, next.Status = cur.OwnerID, cur.Status
	next.CreatedAt, next.PublishedAt = cur.CreatedAt, cur.PublishedAt
	r.scenarios[s.ID] = next
	return nil
}

func (r *ScenarioRepository) SetStatus(_ context.Context, id uuid.UUID, from, to domain.ScenarioStatus, publishedAt *time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.scenarios[id]
	if !ok || s.Status != from {
		return domain.ErrRevisionConflict
	}
	s.Status, s.PublishedAt = to, publishedAt
	s.Revision++
	s.UpdatedAt = time.Now()
	r.scenarios[id] = s
	return nil
}

func (r *ScenarioRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.scenarios[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.scenarios, id)
	return nil
}

func (r *ScenarioRepository) CountSessions(_ context.Context, id uuid.UUID) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Sessions[id], nil
}
