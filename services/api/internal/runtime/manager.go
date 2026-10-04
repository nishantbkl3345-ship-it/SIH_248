package runtime

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/engine"
)

// ErrNotLive means the session has no running engine in this process.
var ErrNotLive = errors.New("runtime: session is not live")

// Store is where a live exercise's history goes. Append must be atomic.
type Store interface {
	Append(ctx context.Context, sessionID uuid.UUID, out engine.Output) error
	Progress(ctx context.Context, sessionID uuid.UUID, status engine.Status, simMs, speedMilli int64, at time.Time) error
}

// timeUpdateEvery is how often clients are sent a clock reading.
const timeUpdateEvery = time.Second

type Options struct {
	Store  Store
	Logger *slog.Logger
	// Now is the wall clock. Tests inject a controllable one.
	Now func() time.Time
	// TickEvery is how often live exercises are advanced. Zero disables the
	// background loop; the caller then drives the manager with Tick.
	TickEvery time.Duration
}

// Manager owns every live exercise in this process.
type Manager struct {
	opts Options

	mu    sync.Mutex
	rooms map[uuid.UUID]*room

	stop chan struct{}
	done chan struct{}
}

// room is one session: its subscribers, and its engine once started. mu
// serialises every use of the engine, and is held across "persist, then
// broadcast" so that order on the wire matches order in the record.
type room struct {
	id  uuid.UUID
	hub *Hub

	mu      sync.Mutex
	eng     *engine.Engine
	backlog []engine.Output // produced but not yet stored
	lastTU  time.Time
}

func NewManager(opts Options) *Manager {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	m := &Manager{opts: opts, rooms: make(map[uuid.UUID]*room), stop: make(chan struct{}), done: make(chan struct{})}
	if opts.TickEvery > 0 {
		go m.loop()
	} else {
		close(m.done)
	}
	return m
}

func (m *Manager) loop() {
	defer close(m.done)
	t := time.NewTicker(m.opts.TickEvery)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.Tick(context.Background())
		}
	}
}

// Shutdown stops the background loop and disconnects every subscriber.
func (m *Manager) Shutdown() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.rooms {
		r.hub.Close()
	}
}

func (m *Manager) room(id uuid.UUID) *room {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rooms[id]
	if !ok {
		r = &room{id: id, hub: NewHub()}
		m.rooms[id] = r
	}
	return r
}

func (m *Manager) liveRooms() []*room {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*room, 0, len(m.rooms))
	for _, r := range m.rooms {
		out = append(out, r)
	}
	return out
}

// Tick advances every live exercise to the current wall time.
func (m *Manager) Tick(ctx context.Context) {
	now := m.opts.Now()
	for _, r := range m.liveRooms() {
		r.mu.Lock()
		if r.eng != nil && r.eng.Status() != engine.StatusEnded {
			m.apply(ctx, r, r.eng.Advance(now), now)
			if r.eng.Status() == engine.StatusRunning && now.Sub(r.lastTU) >= timeUpdateEvery && len(r.backlog) == 0 {
				r.lastTU = now
				r.hub.Broadcast([]engine.Notification{r.eng.TimeUpdate(now)})
			}
		} else if len(r.backlog) > 0 {
			m.flush(ctx, r, now)
		}
		r.mu.Unlock()
	}
}

// apply stores what the engine produced and only then tells anyone. If the
// store is unavailable the output waits in order and nothing is broadcast, so
// clients never see something that is not on the record.
func (m *Manager) apply(ctx context.Context, r *room, out engine.Output, now time.Time) {
	if len(out.Events) > 0 || len(out.Notifications) > 0 {
		r.backlog = append(r.backlog, out)
	}
	m.flush(ctx, r, now)
}

func (m *Manager) flush(ctx context.Context, r *room, now time.Time) {
	stored := false
	for len(r.backlog) > 0 {
		out := r.backlog[0]
		if len(out.Events) > 0 {
			if err := m.opts.Store.Append(ctx, r.id, out); err != nil {
				m.opts.Logger.Error("storing exercise history failed; will retry",
					"session_id", r.id.String(), "events", len(out.Events), "error", err.Error())
				return
			}
			stored = true
		}
		r.backlog = r.backlog[1:]
		r.hub.Broadcast(out.Notifications)
	}
	// The session row is a summary; it only needs touching when something happened.
	if stored && r.eng != nil {
		if err := m.opts.Store.Progress(ctx, r.id, r.eng.Status(), r.eng.Now(now), r.eng.SpeedMilli(), now); err != nil {
			m.opts.Logger.Error("storing session progress failed", "session_id", r.id.String(), "error", err.Error())
		}
	}
}

// Start creates the engine for a session and begins the exercise.
func (m *Manager) Start(ctx context.Context, sessionID uuid.UUID, def engine.Definition, roster []engine.Participant, seed, speedMilli int64) error {
	eng, err := engine.New(def, roster, seed, speedMilli)
	if err != nil {
		return err
	}
	r := m.room(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eng != nil {
		return &engine.Error{Code: "ALREADY_STARTED", Message: "the exercise has already been started"}
	}
	now := m.opts.Now()
	out, err := eng.Start(now)
	if err != nil {
		return err
	}
	r.eng = eng
	r.lastTU = now
	r.hub.SetRoster(roster)
	m.apply(ctx, r, out, now)
	return nil
}

// do runs one command against a live engine under the room lock.
func (m *Manager) do(ctx context.Context, sessionID uuid.UUID, run func(*engine.Engine, time.Time) (engine.Output, error)) error {
	r := m.room(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eng == nil {
		return ErrNotLive
	}
	now := m.opts.Now()
	out, err := run(r.eng, now)
	// Even a rejected command may have advanced the exercise first.
	m.apply(ctx, r, out, now)
	return err
}

func (m *Manager) Pause(ctx context.Context, id uuid.UUID) error {
	return m.do(ctx, id, func(e *engine.Engine, now time.Time) (engine.Output, error) { return e.Pause(now) })
}

func (m *Manager) Resume(ctx context.Context, id uuid.UUID) error {
	return m.do(ctx, id, func(e *engine.Engine, now time.Time) (engine.Output, error) { return e.Resume(now) })
}

func (m *Manager) End(ctx context.Context, id uuid.UUID) error {
	return m.do(ctx, id, func(e *engine.Engine, now time.Time) (engine.Output, error) { return e.End(now) })
}

func (m *Manager) SetSpeed(ctx context.Context, id uuid.UUID, speedMilli int64) error {
	return m.do(ctx, id, func(e *engine.Engine, now time.Time) (engine.Output, error) { return e.SetSpeed(now, speedMilli) })
}

// SendMessage and SubmitDecision take only what the trainee chose. Time,
// team and the information picture are supplied by the engine.
func (m *Manager) SendMessage(ctx context.Context, id, participantID, channelID uuid.UUID, body string) (engine.MessageView, error) {
	var view engine.MessageView
	err := m.do(ctx, id, func(e *engine.Engine, now time.Time) (engine.Output, error) {
		v, out, err := e.SendMessage(now, participantID, channelID, body)
		view = v
		return out, err
	})
	return view, err
}

func (m *Manager) SubmitDecision(ctx context.Context, id uuid.UUID, in engine.DecisionInput) (engine.DecisionRecord, error) {
	var rec engine.DecisionRecord
	err := m.do(ctx, id, func(e *engine.Engine, now time.Time) (engine.Output, error) {
		r, out, err := e.SubmitDecision(now, in)
		rec = r
		return out, err
	})
	return rec, err
}

// Snapshot is what a client is given when it connects: where the exercise
// stands, as far as that viewer is allowed to know.
type Snapshot struct {
	Live       bool                   `json:"live"`
	Trainee    *engine.TraineeView    `json:"trainee,omitempty"`
	Instructor *engine.InstructorView `json:"instructor,omitempty"`
}

func (r *room) snapshot(v Viewer, now time.Time) Snapshot {
	if r.eng == nil {
		return Snapshot{}
	}
	if v.Instructor {
		view := r.eng.InstructorView(now)
		return Snapshot{Live: true, Instructor: &view}
	}
	view, err := r.eng.TraineeView(now, v.Participant.ID)
	if err != nil {
		return Snapshot{Live: true}
	}
	return Snapshot{Live: true, Trainee: &view}
}

// State returns the viewer's current snapshot, advancing the exercise first
// so it is up to date.
func (m *Manager) State(ctx context.Context, sessionID uuid.UUID, v Viewer) Snapshot {
	r := m.room(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := m.opts.Now()
	if r.eng != nil && r.eng.Status() != engine.StatusEnded {
		m.apply(ctx, r, r.eng.Advance(now), now)
	}
	return r.snapshot(v, now)
}

// Connect subscribes a viewer. The snapshot is taken and the subscription
// registered under the same lock, so the client's first frame and the stream
// that follows have no gap and no overlap.
func (m *Manager) Connect(ctx context.Context, sessionID uuid.UUID, v Viewer) (*Client, func()) {
	r := m.room(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	now := m.opts.Now()
	if r.eng != nil && r.eng.Status() != engine.StatusEnded {
		m.apply(ctx, r, r.eng.Advance(now), now)
	}
	snap := r.snapshot(v, now)
	var simMs int64
	if r.eng != nil {
		simMs = r.eng.Now(now)
	}
	c := r.hub.Subscribe(v, &Envelope{Type: "session.snapshot", SimMs: simMs, Payload: snap})
	return c, func() { r.hub.Unsubscribe(c) }
}

// IsLive reports whether the session has an engine in this process.
func (m *Manager) IsLive(sessionID uuid.UUID) bool {
	r := m.room(sessionID)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.eng != nil
}
