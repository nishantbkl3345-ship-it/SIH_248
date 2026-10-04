package engine

import (
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

// Error is a rejected command. Code is stable and machine-readable.
type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

func reject(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// idNamespace scopes the ids the engine derives; any fixed value will do.
var idNamespace = uuid.MustParse("6f1c2a0e-8f4b-4d53-9a55-1d0b7c9e2f10")

// Engine runs one exercise. Construct it with New, then drive it with Start
// and the command methods; call Advance regularly to process whatever has
// become due.
type Engine struct {
	def     Definition
	seed    int64
	roster  []Participant // ordered by id for deterministic iteration
	byID    map[uuid.UUID]Participant
	channel map[uuid.UUID]Channel

	clock Clock
	sched Scheduler
	rec   Recorder
	st    State

	// Per-call context.
	wall time.Time
	now  SimMs
	out  Output
}

// New prepares an exercise. speedMilli is the initial speed in thousandths
// (1000 = real time).
func New(def Definition, roster []Participant, seed, speedMilli int64) (*Engine, error) {
	if speedMilli < MinSpeedMilli || speedMilli > MaxSpeedMilli {
		return nil, ErrSpeedOutOfRange
	}
	if def.TeamCount < 1 || len(def.Phases) == 0 || def.TotalMs <= 0 {
		return nil, reject("INVALID_DEFINITION", "a scenario needs at least one team and one phase with a duration")
	}
	e := &Engine{
		def:     def,
		seed:    seed,
		roster:  append([]Participant(nil), roster...),
		byID:    make(map[uuid.UUID]Participant, len(roster)),
		channel: make(map[uuid.UUID]Channel, len(def.Channels)),
		clock:   NewClock(speedMilli),
		st:      newState(),
	}
	sort.Slice(e.roster, func(i, j int) bool { return e.roster[i].ID.String() < e.roster[j].ID.String() })
	for _, p := range e.roster {
		if p.Team < 1 || p.Team > def.TeamCount {
			return nil, reject("INVALID_ROSTER", "participant %s is on team %d, but the scenario has %d", p.ID, p.Team, def.TeamCount)
		}
		e.byID[p.ID] = p
	}
	for _, c := range def.Channels {
		e.channel[c.ID] = c
	}
	return e, nil
}

func (e *Engine) Status() Status           { return e.st.status }
func (e *Engine) Now(wall time.Time) SimMs { return e.clock.Now(wall) }
func (e *Engine) SpeedMilli() int64        { return e.clock.SpeedMilli() }
func (e *Engine) Definition() Definition   { return e.def }
func (e *Engine) Timeline(afterSeq int64) []Event {
	return e.rec.Since(afterSeq)
}

// begin and finish bracket every public call.
func (e *Engine) begin(wall time.Time) {
	e.wall = wall
	e.now = e.clock.Now(wall)
	e.out = Output{}
}

func (e *Engine) finish() Output {
	out := e.out
	e.out = Output{}
	return out
}

// record appends to the timeline and mirrors the entry to the instructor.
func (e *Engine) record(typ EventType, team int, participant *uuid.UUID, payload any) Event {
	ev := e.rec.append(e.now, e.wall, typ, team, participant, payload)
	e.out.Events = append(e.out.Events, ev)
	e.out.Notifications = append(e.out.Notifications, Notification{
		Type:     instructorNames[typ],
		SimMs:    e.now,
		Audience: Audience{Instructor: true},
		Payload:  ev,
	})
	return ev
}

// tell queues a notification for trainees. Payloads passed here must contain
// only what that audience is allowed to know.
func (e *Engine) tell(typ string, to Audience, payload any) {
	to.Instructor = false
	e.out.Notifications = append(e.out.Notifications, Notification{Type: typ, SimMs: e.now, Audience: to, Payload: payload})
}

// newID derives a fresh id from the seed and a counter, so ids are the same
// on every run of the same exercise.
func (e *Engine) newID(kind string) uuid.UUID {
	e.st.idCounter++
	return uuid.NewSHA1(idNamespace, fmt.Appendf(nil, "%d/%s/%d", e.seed, kind, e.st.idCounter))
}

type clockPayload struct {
	Status     Status `json:"status"`
	SimMs      SimMs  `json:"simMs"`
	SpeedMilli int64  `json:"speedMilli"`
	TotalMs    SimMs  `json:"totalMs"`
}

func (e *Engine) clockPayload() clockPayload {
	return clockPayload{Status: e.st.status, SimMs: e.now, SpeedMilli: e.clock.SpeedMilli(), TotalMs: e.def.TotalMs}
}

// Start begins the exercise and processes everything scheduled for T=0.
func (e *Engine) Start(wall time.Time) (Output, error) {
	if e.st.status != StatusIdle {
		return Output{}, reject("ALREADY_STARTED", "the exercise has already been started")
	}
	e.begin(wall)
	e.schedule()
	e.st.status = StatusRunning
	e.clock.Resume(wall)
	e.record(EvExerciseStarted, 0, nil, e.clockPayload())
	e.tell(NSimulationStarted, Audience{AllTrainees: true}, e.clockPayload())
	e.advance()
	return e.finish(), nil
}

// Advance processes every task that has become due by the given wall time.
func (e *Engine) Advance(wall time.Time) Output {
	e.begin(wall)
	e.advance()
	return e.finish()
}

func (e *Engine) Pause(wall time.Time) (Output, error) {
	if e.st.status != StatusRunning {
		return Output{}, reject("NOT_RUNNING", "the exercise is not running")
	}
	e.begin(wall)
	e.advance()
	if e.st.status != StatusRunning { // it ended while catching up
		return e.finish(), nil
	}
	e.clock.Pause(wall)
	e.st.status = StatusPaused
	e.record(EvExercisePaused, 0, nil, e.clockPayload())
	e.tell(NSimulationPaused, Audience{AllTrainees: true}, e.clockPayload())
	return e.finish(), nil
}

func (e *Engine) Resume(wall time.Time) (Output, error) {
	if e.st.status != StatusPaused {
		return Output{}, reject("NOT_PAUSED", "the exercise is not paused")
	}
	e.begin(wall)
	e.clock.Resume(wall)
	e.st.status = StatusRunning
	e.record(EvExerciseResumed, 0, nil, e.clockPayload())
	e.tell(NSimulationResumed, Audience{AllTrainees: true}, e.clockPayload())
	return e.finish(), nil
}

// SetSpeed changes how fast simulation time passes from now on.
func (e *Engine) SetSpeed(wall time.Time, speedMilli int64) (Output, error) {
	if e.st.status != StatusRunning && e.st.status != StatusPaused {
		return Output{}, reject("NOT_RUNNING", "the exercise is not in progress")
	}
	if speedMilli < MinSpeedMilli || speedMilli > MaxSpeedMilli {
		return Output{}, reject("SPEED_OUT_OF_RANGE", "speed must be between %.2fx and %dx", float64(MinSpeedMilli)/1000, MaxSpeedMilli/1000)
	}
	e.begin(wall)
	e.advance() // everything up to now happens at the old speed
	if e.st.status == StatusEnded {
		return e.finish(), nil
	}
	if err := e.clock.SetSpeed(wall, speedMilli); err != nil {
		return Output{}, err
	}
	e.record(EvSpeedChanged, 0, nil, e.clockPayload())
	e.tell(NSimulationTime, Audience{AllTrainees: true}, e.clockPayload())
	return e.finish(), nil
}

// End stops the exercise early.
func (e *Engine) End(wall time.Time) (Output, error) {
	if e.st.status != StatusRunning && e.st.status != StatusPaused {
		return Output{}, reject("NOT_RUNNING", "the exercise is not in progress")
	}
	e.begin(wall)
	e.advance()
	if e.st.status != StatusEnded {
		e.end("ENDED_BY_INSTRUCTOR")
	}
	return e.finish(), nil
}

// TimeUpdate is a clock reading for clients to resynchronise against. It is
// not recorded.
func (e *Engine) TimeUpdate(wall time.Time) Notification {
	p := e.clockPayload()
	p.SimMs = e.clock.Now(wall)
	return Notification{Type: NSimulationTime, SimMs: p.SimMs, Audience: Audience{Instructor: true, AllTrainees: true}, Payload: p}
}

func (e *Engine) end(reason string) {
	e.clock.StopAt(e.now)
	e.sched.Clear()
	e.st.status = StatusEnded
	type payload struct {
		clockPayload
		Reason string `json:"reason"`
	}
	p := payload{e.clockPayload(), reason}
	e.record(EvExerciseEnded, 0, nil, p)
	e.tell(NSimulationCompleted, Audience{AllTrainees: true}, p)
}

// participant resolves a trainee for a command that needs a live exercise.
func (e *Engine) participant(id uuid.UUID) (Participant, *Error) {
	p, ok := e.byID[id]
	if !ok {
		return Participant{}, reject("NOT_A_PARTICIPANT", "you are not a participant in this exercise")
	}
	switch e.st.status {
	case StatusRunning:
		return p, nil
	case StatusPaused:
		return Participant{}, reject("EXERCISE_PAUSED", "the exercise is paused")
	case StatusEnded:
		return Participant{}, reject("EXERCISE_ENDED", "the exercise has ended")
	default:
		return Participant{}, reject("NOT_RUNNING", "the exercise has not started")
	}
}

func (e *Engine) teams(team int) []int {
	if team != 0 {
		return []int{team}
	}
	all := make([]int, e.def.TeamCount)
	for i := range all {
		all[i] = i + 1
	}
	return all
}

// activeRules returns the rules currently in force, in definition order.
func (e *Engine) activeRules() []Rule {
	rules := make([]Rule, len(e.st.activeRules))
	for i, idx := range e.st.activeRules {
		rules[i] = e.def.Rules[idx]
	}
	return rules
}
