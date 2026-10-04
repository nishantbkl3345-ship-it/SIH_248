package memrepo

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
	"fogline/api/internal/repository"
)

type SessionRepository struct {
	mu        sync.RWMutex
	users     *UserRepository
	sessions  map[uuid.UUID]domain.ExerciseSession
	Events    map[uuid.UUID][]domain.TimelineEvent
	Decisions []domain.Decision
	Messages  []domain.Message
	// FailAppends makes the next N AppendRecords calls fail, for testing how
	// the runtime copes with a storage outage.
	FailAppends int
}

func NewSessionRepository(users *UserRepository) *SessionRepository {
	return &SessionRepository{
		users:    users,
		sessions: make(map[uuid.UUID]domain.ExerciseSession),
		Events:   make(map[uuid.UUID][]domain.TimelineEvent),
	}
}

func cloneSession(s domain.ExerciseSession) domain.ExerciseSession {
	raw, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	var out domain.ExerciseSession
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return out
}

func (r *SessionRepository) Create(_ context.Context, s *domain.ExerciseSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, other := range r.sessions {
		if other.JoinCode == s.JoinCode {
			return domain.ErrJoinCodeTaken
		}
	}
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	now := time.Now()
	s.CreatedAt, s.UpdatedAt = now, now
	for i := range s.Teams {
		s.Teams[i].ID = uuid.New()
		s.Teams[i].SessionID = s.ID
	}
	r.sessions[s.ID] = cloneSession(*s)
	return nil
}

func (r *SessionRepository) Get(ctx context.Context, id uuid.UUID) (*domain.ExerciseSession, error) {
	r.mu.RLock()
	s, ok := r.sessions[id]
	r.mu.RUnlock()
	if !ok {
		return nil, domain.ErrNotFound
	}
	out := cloneSession(s)
	for i := range out.Participants {
		if u, err := r.users.FindByID(ctx, out.Participants[i].UserID); err == nil {
			out.Participants[i].User = u
		}
	}
	return &out, nil
}

func (r *SessionRepository) FindByJoinCode(ctx context.Context, code string) (*domain.ExerciseSession, error) {
	r.mu.RLock()
	var id uuid.UUID
	for _, s := range r.sessions {
		if s.JoinCode == code {
			id = s.ID
		}
	}
	r.mu.RUnlock()
	if id == uuid.Nil {
		return nil, domain.ErrNotFound
	}
	return r.Get(ctx, id)
}

func (r *SessionRepository) list(keep func(domain.ExerciseSession) bool) []domain.ExerciseSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.ExerciseSession{}
	for _, s := range r.sessions {
		if keep(s) {
			out = append(out, cloneSession(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (r *SessionRepository) ListForInstructor(_ context.Context, instructorID *uuid.UUID) ([]domain.ExerciseSession, error) {
	return r.list(func(s domain.ExerciseSession) bool { return instructorID == nil || s.InstructorID == *instructorID }), nil
}

func (r *SessionRepository) ListForUser(_ context.Context, userID uuid.UUID) ([]domain.ExerciseSession, error) {
	return r.list(func(s domain.ExerciseSession) bool {
		for _, p := range s.Participants {
			if p.UserID == userID {
				return true
			}
		}
		return false
	}), nil
}

func (r *SessionRepository) AddParticipant(_ context.Context, p *domain.SessionParticipant) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[p.SessionID]
	if !ok {
		return domain.ErrNotFound
	}
	for _, other := range s.Participants {
		if other.UserID == p.UserID {
			return domain.ErrAlreadyJoined
		}
	}
	p.ID = uuid.New()
	p.JoinedAt = time.Now()
	stored := *p
	stored.User = nil
	s.Participants = append(s.Participants, stored)
	r.sessions[s.ID] = s
	return nil
}

func (r *SessionRepository) SetParticipantTeam(_ context.Context, participantID, teamID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, s := range r.sessions {
		for i := range s.Participants {
			if s.Participants[i].ID == participantID {
				s.Participants[i].TeamID = &teamID
				r.sessions[id] = s
				return nil
			}
		}
	}
	return domain.ErrNotFound
}

func (r *SessionRepository) UpdateProgress(_ context.Context, id uuid.UUID, p repository.SessionProgress) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return domain.ErrNotFound
	}
	s.Status, s.SimMs, s.SpeedMilli = p.Status, p.SimMs, p.SpeedMilli
	if p.StartedAt != nil && s.StartedAt == nil {
		s.StartedAt = p.StartedAt
	}
	if p.EndedAt != nil && s.EndedAt == nil {
		s.EndedAt = p.EndedAt
	}
	r.sessions[id] = s
	return nil
}

func (r *SessionRepository) AppendRecords(_ context.Context, rec repository.SessionRecords) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.FailAppends > 0 {
		r.FailAppends--
		return errStorage
	}
	for _, e := range rec.Events {
		r.Events[e.SessionID] = append(r.Events[e.SessionID], e)
	}
	r.Decisions = append(r.Decisions, rec.Decisions...)
	r.Messages = append(r.Messages, rec.Messages...)
	return nil
}

func (r *SessionRepository) Timeline(_ context.Context, sessionID uuid.UUID, afterSeq int64, limit int) ([]domain.TimelineEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := []domain.TimelineEvent{}
	for _, e := range r.Events[sessionID] {
		if e.Seq > afterSeq && len(out) < limit {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *SessionRepository) InterruptLive(_ context.Context, at time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for id, s := range r.sessions {
		if s.Status == domain.SessionRunning || s.Status == domain.SessionPaused {
			s.Status, s.EndedAt = domain.SessionEnded, &at
			r.sessions[id] = s
			n++
		}
	}
	return n, nil
}

func (r *SessionRepository) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.sessions, id)
	return nil
}

type storageError struct{}

func (storageError) Error() string { return "memrepo: simulated storage failure" }

var errStorage = storageError{}
