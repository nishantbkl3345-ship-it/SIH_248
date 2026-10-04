package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/engine"
)

// These tests run with no background loop and a hand-wound clock: the manager
// only moves when a test calls Tick, so there is no timing to get wrong.

var t0 = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) set(sec float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t0.Add(time.Duration(sec * float64(time.Second)))
}

// recStore remembers the order in which things reached it.
type recStore struct {
	mu       sync.Mutex
	events   []engine.Event
	failNext int
	progress []engine.Status
}

func (s *recStore) Append(_ context.Context, _ uuid.UUID, out engine.Output) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failNext > 0 {
		s.failNext--
		return errors.New("store unavailable")
	}
	s.events = append(s.events, out.Events...)
	return nil
}

func (s *recStore) Progress(_ context.Context, _ uuid.UUID, status engine.Status, _, _ int64, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.progress = append(s.progress, status)
	return nil
}

func (s *recStore) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func nid(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

var (
	feed  = nid("feed")
	alice = engine.Participant{ID: nid("alice"), Name: "Alice", Team: 1}
	bela  = engine.Participant{ID: nid("bela"), Name: "Bela", Team: 2}
)

func definition() engine.Definition {
	dropTeam2 := engine.Rule{ID: nid("drop"), Mode: engine.ModeDropout, StartMs: 50_000, EndMs: 70_000, Team: 2}
	return engine.Definition{
		TeamCount: 2,
		TotalMs:   120_000,
		Channels:  []engine.Channel{{ID: feed, Name: "Field Feed", Kind: engine.ChannelFeed}},
		Phases:    []engine.Phase{{ID: nid("p"), Title: "Only", EndMs: 120_000}},
		Reports: []engine.Report{
			{ID: nid("r1"), AtMs: 30_000, ChannelID: feed, Title: "first", Source: "Synthetic Sensor Alpha", Content: "alpha"},
			{ID: nid("r2"), AtMs: 60_000, ChannelID: feed, Title: "second", Source: "Synthetic Observer Bravo", Content: "bravo"},
		},
		Rules: []engine.Rule{dropTeam2},
	}
}

type fixture struct {
	t     *testing.T
	m     *Manager
	clock *fakeClock
	store *recStore
	id    uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, clock: &fakeClock{now: t0}, store: &recStore{}, id: uuid.New()}
	f.m = NewManager(Options{Store: f.store, Now: f.clock.Now})
	t.Cleanup(f.m.Shutdown)
	return f
}

func (f *fixture) start() {
	f.t.Helper()
	if err := f.m.Start(context.Background(), f.id, definition(), []engine.Participant{alice, bela}, 7, 1000); err != nil {
		f.t.Fatalf("Start: %v", err)
	}
}

func (f *fixture) tick(sec float64) {
	f.clock.set(sec)
	f.m.Tick(context.Background())
}

// drain returns the frames queued for a client, decoded, without blocking.
func drain(t *testing.T, c *Client) []Envelope {
	t.Helper()
	var out []Envelope
	for {
		select {
		case frame, open := <-c.Send():
			if !open {
				return out
			}
			var e Envelope
			if err := json.Unmarshal(frame, &e); err != nil {
				t.Fatalf("bad frame %s: %v", frame, err)
			}
			out = append(out, e)
		default:
			return out
		}
	}
}

func types(frames []Envelope) string {
	var out []string
	for _, f := range frames {
		out = append(out, f.Type)
	}
	return strings.Join(out, ",")
}

func TestTraineesReceiveOnlyWhatIsAddressedToTheirTeam(t *testing.T) {
	f := newFixture(t)
	a, _ := f.m.Connect(context.Background(), f.id, Viewer{Participant: alice})
	b, _ := f.m.Connect(context.Background(), f.id, Viewer{Participant: bela})
	inst, _ := f.m.Connect(context.Background(), f.id, Viewer{Instructor: true})

	// Before the exercise starts a client gets a snapshot saying so.
	if got := drain(t, a); len(got) != 1 || got[0].Type != "session.snapshot" {
		t.Fatalf("lobby frames = %s", types(got))
	}
	drain(t, b)
	drain(t, inst)

	f.start()
	f.tick(30)
	f.tick(60) // second report: delivered to team 1, dropped for team 2
	f.tick(61)

	aliceFrames, belaFrames, instFrames := drain(t, a), drain(t, b), drain(t, inst)

	if got := types(aliceFrames); got != "simulation.started,phase.changed,report.received,simulation.time_update,report.received,simulation.time_update,simulation.time_update" {
		t.Errorf("alice received: %s", got)
	}
	if got := types(belaFrames); got != "simulation.started,phase.changed,report.received,simulation.time_update,simulation.time_update,simulation.time_update" {
		t.Errorf("bela received: %s", got)
	}
	// Bela must have no trace of the dropped report or of the dropout itself.
	raw, _ := json.Marshal(belaFrames)
	for _, leak := range []string{"bravo", "second", "Bravo", "DROPPED", "degraded", "dropped", "appliedRules"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("team 2's stream leaks %q", leak)
		}
	}

	// The instructor sees all of it, in timeline order.
	got := types(instFrames)
	for _, want := range []string{"simulation.started", "report.generated", "report.received", "communication.degraded", "report.dropped", "simulation.time_update"} {
		if !strings.Contains(got, want) {
			t.Errorf("instructor stream has no %s: %s", want, got)
		}
	}
	if strings.Index(got, "communication.degraded") > strings.Index(got, "report.dropped") {
		t.Errorf("instructor stream is out of order: %s", got)
	}
}

func TestHistoryIsStoredBeforeAnyoneIsTold(t *testing.T) {
	f := newFixture(t)
	a, _ := f.m.Connect(context.Background(), f.id, Viewer{Participant: alice})
	drain(t, a)
	f.start()
	drain(t, a)
	stored := f.store.count()

	// The store goes down just as the first report is generated.
	f.store.failNext = 2
	f.tick(30)
	if got := drain(t, a); len(got) != 0 {
		t.Fatalf("alice was told %s while the record could not be written", types(got))
	}
	if f.store.count() != stored {
		t.Fatal("events were stored despite the simulated failure")
	}

	f.tick(30.5) // still down
	if got := drain(t, a); len(got) != 0 {
		t.Fatalf("alice was told %s while the record could not be written", types(got))
	}

	f.tick(31) // back up: the backlog is stored in order, then broadcast
	if f.store.count() <= stored {
		t.Fatal("backlog was not stored once the store recovered")
	}
	if got := types(drain(t, a)); !strings.HasPrefix(got, "report.received") {
		t.Fatalf("alice received %q after recovery, want the report first", got)
	}
	for i, e := range f.store.events {
		if e.Seq != int64(i+1) {
			t.Fatalf("stored events are out of sequence at %d: seq %d", i, e.Seq)
		}
	}
}

func TestConnectingMidExerciseGivesASnapshotThenTheStreamWithNoGap(t *testing.T) {
	f := newFixture(t)
	f.start()
	f.tick(45)

	// Connect at T+45: one report so far. The call itself catches the engine up.
	f.clock.set(60)
	a, leave := f.m.Connect(context.Background(), f.id, Viewer{Participant: alice})
	frames := drain(t, a)
	if len(frames) != 1 || frames[0].Type != "session.snapshot" {
		t.Fatalf("first frames = %s", types(frames))
	}
	raw, _ := json.Marshal(frames[0].Payload)
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if !snap.Live || snap.Trainee == nil || snap.Instructor != nil {
		t.Fatalf("snapshot = %+v", snap)
	}
	// Both reports are in the snapshot (the second landed at T+60, during
	// Connect), so neither may also arrive as a stream frame.
	if n := len(snap.Trainee.Reports); n != 2 {
		t.Fatalf("snapshot holds %d reports, want 2", n)
	}
	f.tick(61)
	if got := types(drain(t, a)); strings.Contains(got, "report.received") {
		t.Fatalf("a report in the snapshot was sent again: %s", got)
	}

	leave()
	f.tick(120)
	if _, open := <-a.Send(); open {
		t.Error("an unsubscribed client is still receiving")
	}
}

func TestStateReflectsOnlyTheViewersInformation(t *testing.T) {
	f := newFixture(t)
	if snap := f.m.State(context.Background(), f.id, Viewer{Participant: alice}); snap.Live {
		t.Fatal("a session that has not started reports as live")
	}
	f.start()
	f.clock.set(65)

	a := f.m.State(context.Background(), f.id, Viewer{Participant: alice})
	b := f.m.State(context.Background(), f.id, Viewer{Participant: bela})
	i := f.m.State(context.Background(), f.id, Viewer{Instructor: true})
	if len(a.Trainee.Reports) != 2 || len(b.Trainee.Reports) != 1 {
		t.Errorf("alice has %d reports and bela %d, want 2 and 1", len(a.Trainee.Reports), len(b.Trainee.Reports))
	}
	if i.Instructor == nil || i.Trainee != nil || i.Instructor.Teams[1].DroppedReports != 1 {
		t.Errorf("instructor state = %+v", i.Instructor)
	}
	if a.Instructor != nil || b.Instructor != nil {
		t.Error("a trainee was given the instructor view")
	}
}

func TestCommandsAgainstASessionThatIsNotLive(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.m.Pause(ctx, f.id); !errors.Is(err, ErrNotLive) {
		t.Errorf("Pause = %v, want ErrNotLive", err)
	}
	if _, err := f.m.SendMessage(ctx, f.id, alice.ID, feed, "x"); !errors.Is(err, ErrNotLive) {
		t.Errorf("SendMessage = %v, want ErrNotLive", err)
	}
	f.start()
	if err := f.m.Start(ctx, f.id, definition(), []engine.Participant{alice}, 7, 1000); err == nil {
		t.Error("starting a session twice succeeded")
	}
}

func TestLifecycleIsPersisted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.start()
	f.clock.set(10)
	if err := f.m.Pause(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	f.tick(500) // paused: nothing may advance
	paused := f.store.count()
	f.tick(900)
	if f.store.count() != paused {
		t.Fatal("events were recorded while paused")
	}
	if err := f.m.Resume(ctx, f.id); err != nil {
		t.Fatal(err)
	}
	if err := f.m.SetSpeed(ctx, f.id, 5000); err != nil {
		t.Fatal(err)
	}
	f.tick(900 + 22) // 22 real seconds at 5x from T+10 = T+120: the end
	f.tick(900 + 60)

	last := f.store.events[len(f.store.events)-1]
	if last.Type != engine.EvExerciseEnded || last.SimMs != 120_000 {
		t.Fatalf("last stored event = %s at %d", last.Type, last.SimMs)
	}
	got := f.store.progress
	if got[0] != engine.StatusRunning || got[len(got)-1] != engine.StatusEnded {
		t.Errorf("progress writes = %v", got)
	}
	seenPaused := false
	for _, s := range got {
		seenPaused = seenPaused || s == engine.StatusPaused
	}
	if !seenPaused {
		t.Errorf("the paused state was never persisted: %v", got)
	}
}

func TestSlowSubscriberIsDroppedNotWaitedFor(t *testing.T) {
	h := NewHub()
	slow := h.Subscribe(Viewer{Instructor: true}, nil)
	ok := h.Subscribe(Viewer{Instructor: true}, nil)

	note := []engine.Notification{{Type: "x", Audience: engine.Audience{Instructor: true}, Payload: 1}}
	for i := 0; i < sendBuffer+10; i++ {
		h.Broadcast(note)
		<-ok.Send() // this one keeps up
	}
	if h.Count() != 1 {
		t.Fatalf("%d subscribers left, want only the one that kept up", h.Count())
	}
	n := 0
	for range slow.Send() { // closed after its buffer, so this terminates
		n++
	}
	if n != sendBuffer {
		t.Errorf("slow subscriber got %d frames before being dropped, want %d", n, sendBuffer)
	}
}

func TestRosterAtStartOverridesTheTeamAClientConnectedWith(t *testing.T) {
	f := newFixture(t)
	// Bela connected while still listed on team 1; the instructor then moved
	// her to team 2 before starting.
	stale := bela
	stale.Team = 1
	b, _ := f.m.Connect(context.Background(), f.id, Viewer{Participant: stale})
	stranger, _ := f.m.Connect(context.Background(), f.id, Viewer{Participant: engine.Participant{ID: nid("gone"), Team: 1}})
	drain(t, b)
	drain(t, stranger)

	f.start()
	f.tick(60)
	if got := types(drain(t, b)); strings.Count(got, "report.received") != 1 {
		t.Errorf("bela should get team 2's single report, got: %s", got)
	}
	if _, open := <-stranger.Send(); open {
		// one buffered frame may still be readable; the channel must then close
		if _, open := <-stranger.Send(); open {
			t.Error("a subscriber who is not on the roster stayed connected")
		}
	}
}

func TestEntitlement(t *testing.T) {
	inst, a1 := Viewer{Instructor: true}, Viewer{Participant: alice}
	cases := []struct {
		name     string
		viewer   Viewer
		audience engine.Audience
		want     bool
	}{
		{"instructor gets instructor notes", inst, engine.Audience{Instructor: true}, true},
		{"instructor does not get trainee notes", inst, engine.Audience{AllTrainees: true}, false},
		{"trainee does not get instructor notes", a1, engine.Audience{Instructor: true}, false},
		{"trainee does not get instructor notes that also name their team", a1, engine.Audience{Instructor: true, Teams: []int{1}}, false},
		{"clock updates go to both", a1, engine.Audience{Instructor: true, AllTrainees: true}, true},
		{"own team", a1, engine.Audience{Teams: []int{1}}, true},
		{"other team", a1, engine.Audience{Teams: []int{2}}, false},
		{"addressed personally", a1, engine.Audience{Participants: []uuid.UUID{alice.ID}}, true},
		{"excluded as the sender", a1, engine.Audience{Teams: []int{1}, Except: alice.ID}, false},
		{"empty audience reaches nobody", a1, engine.Audience{}, false},
	}
	for _, tc := range cases {
		if got := entitled(tc.viewer, tc.audience); got != tc.want {
			t.Errorf("%s: entitled = %v, want %v", tc.name, got, tc.want)
		}
	}
}
