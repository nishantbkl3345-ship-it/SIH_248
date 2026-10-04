package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/engine"
	"fogline/api/internal/repository"
	"fogline/api/internal/runtime"
)

// SessionService runs exercises: lobby, lifecycle, and the trainee commands.
// It decides who may do what; the engine decides what happens.
type SessionService struct {
	sessions  repository.SessionRepository
	scenarios repository.ScenarioRepository
	rt        *runtime.Manager
}

func NewSessionService(sessions repository.SessionRepository, scenarios repository.ScenarioRepository, rt *runtime.Manager) *SessionService {
	return &SessionService{sessions: sessions, scenarios: scenarios, rt: rt}
}

var errSessionNotFound = apperr.NotFound("Session not found.")

// joinAlphabet leaves out characters that are easily confused when read aloud
// or copied from a projector.
const joinAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newJoinCode() (string, error) {
	var b strings.Builder
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(joinAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(joinAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func newSeed() (int64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(buf[:]) >> 1), nil
}

// Create opens a lobby for a published scenario. seed may be nil; supplying
// one makes the run reproducible.
func (s *SessionService) Create(ctx context.Context, actor *domain.User, scenarioID uuid.UUID, name string, seed *int64) (*domain.ExerciseSession, error) {
	sc, err := s.scenarios.Get(ctx, scenarioID)
	if errors.Is(err, domain.ErrNotFound) || (err == nil && actor.Role != domain.RoleAdmin && sc.OwnerID != actor.ID) {
		return nil, apperr.NotFound("Scenario not found.")
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if sc.Status != domain.ScenarioPublished {
		return nil, apperr.Conflict("SCENARIO_NOT_PUBLISHED", "Only a published scenario can be run. Publish it first.")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = sc.Title
	}
	sess := &domain.ExerciseSession{
		ScenarioID: sc.ID, InstructorID: actor.ID, Name: name,
		Status: domain.SessionLobby, SpeedMilli: 1000,
	}
	if seed != nil {
		sess.Seed = *seed
	} else if sess.Seed, err = newSeed(); err != nil {
		return nil, apperr.Internal(err)
	}
	for n := 1; n <= sc.TeamCount; n++ {
		sess.Teams = append(sess.Teams, domain.Team{Number: n, Name: fmt.Sprintf("Team %d", n)})
	}

	// A collision on a six-character code is unlikely; try a few times anyway.
	for attempt := 0; attempt < 5; attempt++ {
		if sess.JoinCode, err = newJoinCode(); err != nil {
			return nil, apperr.Internal(err)
		}
		err = s.sessions.Create(ctx, sess)
		if !errors.Is(err, domain.ErrJoinCodeTaken) {
			break
		}
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return s.sessions.Get(ctx, sess.ID)
}

// Access is how an actor relates to a session.
type Access struct {
	Session     *domain.ExerciseSession
	Instructor  bool                       // runs this session (or is an admin)
	Participant *domain.SessionParticipant // set for a trainee who has joined
}

// TeamNumber is the participant's team, or 0 if unassigned.
func (a Access) TeamNumber() int {
	if a.Participant == nil || a.Participant.TeamID == nil {
		return 0
	}
	for _, t := range a.Session.Teams {
		if t.ID == *a.Participant.TeamID {
			return t.Number
		}
	}
	return 0
}

// Viewer is the realtime identity for this access.
func (a Access) Viewer() runtime.Viewer {
	if a.Instructor {
		return runtime.Viewer{Instructor: true}
	}
	return runtime.Viewer{Participant: engine.Participant{
		ID: a.Participant.ID, Name: displayName(a.Participant), Team: a.TeamNumber(),
	}}
}

func displayName(p *domain.SessionParticipant) string {
	if p.User != nil {
		return p.User.DisplayName
	}
	return "Trainee"
}

// Authorize loads a session the actor may see. Anyone else gets "not found".
func (s *SessionService) Authorize(ctx context.Context, actor *domain.User, id uuid.UUID) (Access, error) {
	sess, err := s.sessions.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return Access{}, errSessionNotFound
	}
	if err != nil {
		return Access{}, apperr.Internal(err)
	}
	if actor.Role == domain.RoleAdmin || (actor.Role == domain.RoleInstructor && sess.InstructorID == actor.ID) {
		return Access{Session: sess, Instructor: true}, nil
	}
	for i := range sess.Participants {
		if sess.Participants[i].UserID == actor.ID {
			return Access{Session: sess, Participant: &sess.Participants[i]}, nil
		}
	}
	return Access{}, errSessionNotFound
}

func (s *SessionService) instructor(ctx context.Context, actor *domain.User, id uuid.UUID) (Access, error) {
	a, err := s.Authorize(ctx, actor, id)
	if err != nil {
		return Access{}, err
	}
	if !a.Instructor {
		return Access{}, apperr.Forbidden("Only the instructor running this exercise can do that.")
	}
	return a, nil
}

func (s *SessionService) trainee(ctx context.Context, actor *domain.User, id uuid.UUID) (Access, error) {
	a, err := s.Authorize(ctx, actor, id)
	if err != nil {
		return Access{}, err
	}
	if a.Participant == nil {
		return Access{}, apperr.Forbidden("Only a trainee taking part in this exercise can do that.")
	}
	return a, nil
}

// List returns the sessions relevant to the actor.
func (s *SessionService) List(ctx context.Context, actor *domain.User) ([]domain.ExerciseSession, error) {
	var (
		out []domain.ExerciseSession
		err error
	)
	switch actor.Role {
	case domain.RoleAdmin:
		out, err = s.sessions.ListForInstructor(ctx, nil)
	case domain.RoleInstructor:
		out, err = s.sessions.ListForInstructor(ctx, &actor.ID)
	default:
		out, err = s.sessions.ListForUser(ctx, actor.ID)
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

// Join adds a trainee to a lobby by code, on the team with the fewest
// members. Joining again returns the existing membership.
func (s *SessionService) Join(ctx context.Context, actor *domain.User, code string) (Access, error) {
	code = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '-' {
			return -1
		}
		return unicode.ToUpper(r)
	}, code)

	sess, err := s.sessions.FindByJoinCode(ctx, code)
	if errors.Is(err, domain.ErrNotFound) {
		return Access{}, apperr.NotFound("No exercise has that join code.")
	}
	if err != nil {
		return Access{}, apperr.Internal(err)
	}
	for i := range sess.Participants {
		if sess.Participants[i].UserID == actor.ID {
			return Access{Session: sess, Participant: &sess.Participants[i]}, nil
		}
	}
	if sess.Status != domain.SessionLobby {
		return Access{}, apperr.Conflict("SESSION_NOT_JOINABLE", "This exercise has already started.")
	}
	sc, err := s.scenarios.Get(ctx, sess.ScenarioID)
	if err != nil {
		return Access{}, apperr.Internal(err)
	}
	if len(sess.Participants) >= sc.TraineeCount {
		return Access{}, apperr.Conflict("SESSION_FULL", "This exercise is full.")
	}

	size := map[uuid.UUID]int{}
	for _, p := range sess.Participants {
		if p.TeamID != nil {
			size[*p.TeamID]++
		}
	}
	team := sess.Teams[0]
	for _, t := range sess.Teams[1:] { // teams are ordered by number, so ties go to the lowest
		if size[t.ID] < size[team.ID] {
			team = t
		}
	}

	p := &domain.SessionParticipant{SessionID: sess.ID, UserID: actor.ID, TeamID: &team.ID}
	if err := s.sessions.AddParticipant(ctx, p); err != nil && !errors.Is(err, domain.ErrAlreadyJoined) {
		return Access{}, apperr.Internal(err)
	}
	return s.Authorize(ctx, actor, sess.ID)
}

// AssignTeam moves a participant to another team while still in the lobby.
func (s *SessionService) AssignTeam(ctx context.Context, actor *domain.User, sessionID, participantID uuid.UUID, teamNo int) (*domain.ExerciseSession, error) {
	a, err := s.instructor(ctx, actor, sessionID)
	if err != nil {
		return nil, err
	}
	if a.Session.Status != domain.SessionLobby {
		return nil, apperr.Conflict("SESSION_STARTED", "Teams cannot be changed once the exercise has started.")
	}
	var team *domain.Team
	for i := range a.Session.Teams {
		if a.Session.Teams[i].Number == teamNo {
			team = &a.Session.Teams[i]
		}
	}
	if team == nil {
		return nil, apperr.Validation(map[string]string{"team": fmt.Sprintf("must be between 1 and %d", len(a.Session.Teams))})
	}
	found := false
	for _, p := range a.Session.Participants {
		found = found || p.ID == participantID
	}
	if !found {
		return nil, apperr.NotFound("Participant not found.")
	}
	if err := s.sessions.SetParticipantTeam(ctx, participantID, team.ID); err != nil {
		return nil, apperr.Internal(err)
	}
	return s.sessions.Get(ctx, sessionID)
}

// Start begins the exercise with whoever is in the lobby.
func (s *SessionService) Start(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.ExerciseSession, error) {
	a, err := s.instructor(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if a.Session.Status != domain.SessionLobby {
		return nil, apperr.Conflict("ALREADY_STARTED", "This exercise has already been started.")
	}
	if len(a.Session.Participants) == 0 {
		return nil, apperr.Conflict("NO_PARTICIPANTS", "At least one trainee must join before the exercise can start.")
	}
	sc, err := s.scenarios.Get(ctx, a.Session.ScenarioID)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	roster := make([]engine.Participant, 0, len(a.Session.Participants))
	for i := range a.Session.Participants {
		p := &a.Session.Participants[i]
		roster = append(roster, Access{Session: a.Session, Participant: p}.Viewer().Participant)
	}
	err = s.rt.Start(ctx, id, engine.FromScenario(sc), roster, a.Session.Seed, int64(a.Session.SpeedMilli))
	if err != nil {
		return nil, engineErr(err)
	}
	return s.sessions.Get(ctx, id)
}

func (s *SessionService) control(ctx context.Context, actor *domain.User, id uuid.UUID, run func() error) (*domain.ExerciseSession, error) {
	if _, err := s.instructor(ctx, actor, id); err != nil {
		return nil, err
	}
	if err := run(); err != nil {
		return nil, engineErr(err)
	}
	return s.sessions.Get(ctx, id)
}

func (s *SessionService) Pause(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.ExerciseSession, error) {
	return s.control(ctx, actor, id, func() error { return s.rt.Pause(ctx, id) })
}

func (s *SessionService) Resume(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.ExerciseSession, error) {
	return s.control(ctx, actor, id, func() error { return s.rt.Resume(ctx, id) })
}

func (s *SessionService) End(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.ExerciseSession, error) {
	return s.control(ctx, actor, id, func() error { return s.rt.End(ctx, id) })
}

// SetSpeed takes a multiplier such as 5 for "1 real second = 5 simulated".
func (s *SessionService) SetSpeed(ctx context.Context, actor *domain.User, id uuid.UUID, speed float64) (*domain.ExerciseSession, error) {
	milli := int64(speed*1000 + 0.5)
	return s.control(ctx, actor, id, func() error { return s.rt.SetSpeed(ctx, id, milli) })
}

// State is the actor's current picture of the exercise.
func (s *SessionService) State(ctx context.Context, actor *domain.User, id uuid.UUID) (Access, runtime.Snapshot, error) {
	a, err := s.Authorize(ctx, actor, id)
	if err != nil {
		return Access{}, runtime.Snapshot{}, err
	}
	return a, s.rt.State(ctx, id, a.Viewer()), nil
}

// Timeline is the instructor's full record.
func (s *SessionService) Timeline(ctx context.Context, actor *domain.User, id uuid.UUID, afterSeq int64, limit int) ([]domain.TimelineEvent, error) {
	if _, err := s.instructor(ctx, actor, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	events, err := s.sessions.Timeline(ctx, id, max(afterSeq, 0), limit)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return events, nil
}

// SendMessage and SubmitDecision identify the trainee from the authenticated
// user. Nothing about who, when, or which team is taken from the request.
func (s *SessionService) SendMessage(ctx context.Context, actor *domain.User, id, channelID uuid.UUID, body string) (engine.MessageView, error) {
	a, err := s.trainee(ctx, actor, id)
	if err != nil {
		return engine.MessageView{}, err
	}
	view, err := s.rt.SendMessage(ctx, id, a.Participant.ID, channelID, body)
	return view, engineErr(err)
}

func (s *SessionService) SubmitDecision(ctx context.Context, actor *domain.User, id, pointID, optionID uuid.UUID, rationale string, confidence *int) (engine.DecisionRecord, error) {
	a, err := s.trainee(ctx, actor, id)
	if err != nil {
		return engine.DecisionRecord{}, err
	}
	rec, err := s.rt.SubmitDecision(ctx, id, engine.DecisionInput{
		ParticipantID: a.Participant.ID, DecisionPointID: pointID, OptionID: optionID,
		Rationale: rationale, Confidence: confidence,
	})
	return rec, engineErr(err)
}

// Connect opens a realtime subscription for the actor.
func (s *SessionService) Connect(ctx context.Context, actor *domain.User, id uuid.UUID) (*runtime.Client, func(), error) {
	a, err := s.Authorize(ctx, actor, id)
	if err != nil {
		return nil, nil, err
	}
	client, leave := s.rt.Connect(ctx, id, a.Viewer())
	return client, leave, nil
}

// RecoverInterrupted closes sessions that were live when the process last
// stopped: their engines are gone, so they cannot continue.
func (s *SessionService) RecoverInterrupted(ctx context.Context) (int64, error) {
	return s.sessions.InterruptLive(ctx, time.Now())
}

var engineStatus = map[string]int{
	"NOT_A_PARTICIPANT":      http.StatusForbidden,
	"RATIONALE_REQUIRED":     http.StatusUnprocessableEntity,
	"RATIONALE_TOO_LONG":     http.StatusUnprocessableEntity,
	"INVALID_CONFIDENCE":     http.StatusUnprocessableEntity,
	"INVALID_MESSAGE":        http.StatusUnprocessableEntity,
	"UNKNOWN_OPTION":         http.StatusUnprocessableEntity,
	"UNKNOWN_CHANNEL":        http.StatusUnprocessableEntity,
	"UNKNOWN_DECISION_POINT": http.StatusUnprocessableEntity,
	"SPEED_OUT_OF_RANGE":     http.StatusUnprocessableEntity,
}

// engineErr turns an engine rejection into an API error, keeping its code.
func engineErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, runtime.ErrNotLive) {
		return apperr.Conflict("SESSION_NOT_LIVE", "This exercise is not running.")
	}
	var rejected *engine.Error
	if errors.As(err, &rejected) {
		status, ok := engineStatus[rejected.Code]
		if !ok {
			status = http.StatusConflict
		}
		msg := rejected.Message
		if msg != "" {
			msg = strings.ToUpper(msg[:1]) + msg[1:] + "."
		}
		return apperr.New(status, rejected.Code, msg)
	}
	return apperr.Internal(err)
}

// ------------------------------------------------------------ history store

// SessionStore adapts the session repository to what the runtime needs.
type SessionStore struct {
	sessions repository.SessionRepository
}

func NewSessionStore(sessions repository.SessionRepository) *SessionStore {
	return &SessionStore{sessions: sessions}
}

func (s *SessionStore) Append(ctx context.Context, sessionID uuid.UUID, out engine.Output) error {
	rec := repository.SessionRecords{}
	for _, e := range out.Events {
		rec.Events = append(rec.Events, domain.TimelineEvent{
			SessionID: sessionID, Seq: e.Seq, SimMs: e.SimMs, WallTime: e.Wall, Type: string(e.Type),
			ActorParticipantID: e.ParticipantID, TeamNo: e.Team, Payload: domain.JSONB(e.Payload),
		})
	}
	for _, d := range out.Decisions {
		snapshot, err := json.Marshal(d.Snapshot)
		if err != nil {
			return err
		}
		row := domain.Decision{
			ID: d.ID, SessionID: sessionID, DecisionPointID: d.DecisionPointID, OptionID: d.OptionID,
			ParticipantID: d.ParticipantID, TeamNo: d.Team, Rationale: d.Rationale,
			SimMs: d.SimMs, WallTime: d.Wall, ResponseMs: d.ResponseMs, ResponseWallMs: d.ResponseWallMs,
			Snapshot: domain.JSONB(snapshot),
		}
		if d.Confidence != nil {
			c := int16(*d.Confidence)
			row.Confidence = &c
		}
		rec.Decisions = append(rec.Decisions, row)
	}
	for _, m := range out.Messages {
		rec.Messages = append(rec.Messages, domain.Message{
			ID: m.ID, SessionID: sessionID, ChannelID: m.ChannelID, SenderParticipantID: m.SenderID,
			TeamNo: m.Team, Body: m.Body, SimMs: m.SimMs, CreatedAt: m.Wall,
		})
	}
	return s.sessions.AppendRecords(ctx, rec)
}

func (s *SessionStore) Progress(ctx context.Context, sessionID uuid.UUID, status engine.Status, simMs, speedMilli int64, at time.Time) error {
	p := repository.SessionProgress{SimMs: simMs, SpeedMilli: int(speedMilli)}
	switch status {
	case engine.StatusRunning:
		p.Status = domain.SessionRunning
		p.StartedAt = &at
	case engine.StatusPaused:
		p.Status = domain.SessionPaused
	case engine.StatusEnded:
		p.Status = domain.SessionEnded
		p.EndedAt = &at
	default:
		return nil
	}
	return s.sessions.UpdateProgress(ctx, sessionID, p)
}
