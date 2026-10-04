package engine

import (
	"strings"
	"testing"
	"time"
)

func TestClockMapsWallTimeToSimulationTime(t *testing.T) {
	c := NewClock(5000) // 1 real second = 5 simulation seconds
	if got := c.Now(wallAt(100)); got != 0 {
		t.Fatalf("a clock that has not started reads %d", got)
	}
	c.Resume(t0)
	if got := c.Now(wallAt(1)); got != 5*sec {
		t.Errorf("after 1 real second at 5x: %d ms, want 5000", got)
	}
	if got := c.Now(wallAt(12.5)); got != 62_500 {
		t.Errorf("after 12.5 real seconds at 5x: %d ms, want 62500", got)
	}
	if got := c.Now(t0.Add(-time.Hour)); got != 0 {
		t.Errorf("wall time before the anchor moved the clock to %d", got)
	}
}

func TestClockPauseResumeAndSpeedChange(t *testing.T) {
	c := NewClock(1000)
	c.Resume(t0)
	c.Pause(wallAt(10))
	if got := c.Now(wallAt(500)); got != 10*sec {
		t.Fatalf("paused clock reads %d, want 10000", got)
	}
	c.Resume(wallAt(500))
	if got := c.Now(wallAt(505)); got != 15*sec {
		t.Fatalf("after resume: %d, want 15000", got)
	}
	c.Resume(wallAt(900)) // resuming a running clock must not re-anchor it
	if got := c.Now(wallAt(505)); got != 15*sec {
		t.Fatalf("redundant resume changed the reading to %d", got)
	}

	if err := c.SetSpeed(wallAt(505), 4000); err != nil {
		t.Fatal(err)
	}
	if got := c.Now(wallAt(507)); got != 23*sec { // 15 + 2*4
		t.Fatalf("after speed change: %d, want 23000", got)
	}
	if err := c.SetSpeed(wallAt(507), 250); err != nil {
		t.Fatal(err)
	}
	if got := c.Now(wallAt(511)); got != 24*sec { // 23 + 4*0.25
		t.Fatalf("at quarter speed: %d, want 24000", got)
	}
	// WallAt inverts Now, rounding up to the first millisecond that is late enough.
	for _, sim := range []int64{24 * sec, 24*sec + 1, 30 * sec, 31_337} {
		w := c.WallAt(sim)
		if c.Now(w) < sim || c.Now(w.Add(-time.Millisecond)) >= sim {
			t.Errorf("WallAt(%d) = %v is not the first instant at which sim time reaches it", sim, w.Sub(t0))
		}
	}
	if w := c.WallAt(1); !w.Equal(wallAt(507)) {
		t.Errorf("WallAt for a time already passed = %v, want the anchor", w.Sub(t0))
	}

	for _, bad := range []int64{0, 249, 20_001, -1000} {
		if err := c.SetSpeed(wallAt(511), bad); err != ErrSpeedOutOfRange {
			t.Errorf("SetSpeed(%d) = %v, want ErrSpeedOutOfRange", bad, err)
		}
	}
	c.StopAt(99)
	if c.Running() || c.Now(wallAt(9999)) != 99 {
		t.Error("StopAt did not freeze the clock")
	}
}

func TestSchedulerTotalOrder(t *testing.T) {
	var s Scheduler
	// Inserted deliberately out of order.
	s.Schedule(task{at: 20, rank: rankGenerate, index: 1})
	s.Schedule(task{at: 10, rank: rankGenerate, index: 2})
	s.Schedule(task{at: 20, rank: rankRuleStart, index: 3})
	s.Schedule(task{at: 20, rank: rankGenerate, index: 4})
	s.Schedule(task{at: 20, rank: rankPhaseCompleted, index: 5})
	s.Schedule(task{at: 30, rank: rankExerciseEnd, index: 6})
	s.Schedule(task{at: 20, rank: rankRuleEnd, index: 7})

	if _, ok := s.PopDue(9); ok {
		t.Fatal("popped a task before it was due")
	}
	var got []int
	for {
		tk, ok := s.PopDue(25)
		if !ok {
			break
		}
		got = append(got, tk.index)
	}
	// time, then rank (completed, rule end, rule start, generate), then insertion.
	want := []int{2, 5, 7, 3, 1, 4}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if s.Len() != 1 {
		t.Fatalf("%d tasks left, want the one at t=30", s.Len())
	}
}

func TestDegrade(t *testing.T) {
	const content = "one two three four five six seven eight nine ten"
	item := Intent{ItemID: id("item"), ChannelID: chFeed, Team: 1, Content: content}

	delay30 := Rule{ID: id("delay30"), Mode: ModeDelay, DelayMs: 30 * sec}
	delay15feed := Rule{ID: id("delay15"), Mode: ModeDelay, DelayMs: 15 * sec, ChannelID: &chFeed}
	dropTeam1 := Rule{ID: id("drop1"), Mode: ModeDropout, Team: 1}
	dropTeam2 := Rule{ID: id("drop2"), Mode: ModeDropout, Team: 2}
	dropOtherChannel := Rule{ID: id("dropx"), Mode: ModeDropout, ChannelID: &chTeam}
	partial50 := Rule{ID: id("partial50"), Mode: ModePartial, KeepPercent: 50}
	partial20 := Rule{ID: id("partial20"), Mode: ModePartial, KeepPercent: 20}
	normal := Rule{ID: id("normal"), Mode: ModeNormal}

	cases := []struct {
		name    string
		rules   []Rule
		state   DeliveryState
		delayMs int64
		applied []string
	}{
		{"no rules", nil, StateNormal, 0, nil},
		{"rule for another team does not apply", []Rule{dropTeam2}, StateNormal, 0, nil},
		{"rule for another channel does not apply", []Rule{dropOtherChannel}, StateNormal, 0, nil},
		{"delay", []Rule{delay30}, StateDelayed, 30 * sec, []string{"delay30"}},
		{"delays add up", []Rule{delay30, delay15feed}, StateDelayed, 45 * sec, []string{"delay30", "delay15"}},
		{"dropout", []Rule{dropTeam1}, StateDropped, 0, []string{"drop1"}},
		{"dropout beats delay and partial", []Rule{delay30, partial50, dropTeam1}, StateDropped, 0, []string{"drop1"}},
		{"partial", []Rule{partial50}, StatePartial, 0, []string{"partial50"}},
		{"partial and delay combine", []Rule{delay30, partial50}, StatePartial, 30 * sec, []string{"partial50", "delay30"}},
		{"strictest partial wins", []Rule{partial50, partial20}, StatePartial, 0, []string{"partial20"}},
		{"forced normal overrides everything", []Rule{delay30, dropTeam1, partial50, normal}, StateNormal, 0, []string{"normal"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := Degrade(42, item, tc.rules)
			if out.State != tc.state || out.DelayMs != tc.delayMs {
				t.Fatalf("state=%s delay=%d, want %s %d", out.State, out.DelayMs, tc.state, tc.delayMs)
			}
			if len(out.Applied) != len(tc.applied) {
				t.Fatalf("applied = %v, want %v", out.Applied, tc.applied)
			}
			for i, name := range tc.applied {
				if out.Applied[i] != id(name) {
					t.Fatalf("applied[%d] is not %s", i, name)
				}
			}
			switch tc.state {
			case StateDropped:
				if out.Content != "" {
					t.Errorf("dropped item still carries content %q", out.Content)
				}
			case StateNormal, StateDelayed:
				if out.Content != content {
					t.Errorf("content changed: %q", out.Content)
				}
			}
		})
	}
}

func TestDegradePartialIsDeterministicAndOnlyRemoves(t *testing.T) {
	words := strings.Fields(strings.Repeat("alpha bravo charlie delta echo foxtrot golf hotel india juliet ", 20))
	item := Intent{ItemID: id("item"), ChannelID: chFeed, Team: 1, Content: strings.Join(words, " ")}
	rule := Rule{ID: id("partial"), Mode: ModePartial, KeepPercent: 40}

	first := Degrade(42, item, []Rule{rule})
	for i := 0; i < 5; i++ {
		if again := Degrade(42, item, []Rule{rule}); again.Content != first.Content {
			t.Fatal("the same inputs produced different redactions")
		}
	}

	got := strings.Fields(first.Content)
	if len(got) != len(words) {
		t.Fatalf("word count changed from %d to %d", len(words), len(got))
	}
	kept := 0
	for i, w := range got {
		switch w {
		case words[i]:
			kept++
		case Redacted:
		default:
			t.Fatalf("word %d became %q: redaction must only remove, never alter", i, w)
		}
	}
	// 200 words at 40%: allow a generous band around 80.
	if kept < 55 || kept > 105 {
		t.Errorf("kept %d of %d words at 40%%", kept, len(words))
	}

	if other := Degrade(43, item, []Rule{rule}); other.Content == first.Content {
		t.Error("a different seed produced the identical redaction")
	}
	team2 := item
	team2.Team = 2
	if other := Degrade(42, team2, []Rule{rule}); other.Content == first.Content {
		t.Error("two teams lost exactly the same words")
	}
}

func TestDegradeMessageRuleAppliesAtEitherEnd(t *testing.T) {
	dropTeam1 := []Rule{{ID: id("drop1"), Mode: ModeDropout, Team: 1}}
	msg := Intent{ItemID: id("m"), ChannelID: chCross, Content: "hello"}

	msg.SenderTeam, msg.Team = 1, 2 // sent from the cut-off team
	if out := Degrade(1, msg, dropTeam1); out.State != StateDropped {
		t.Errorf("message from the cut-off team: %s, want DROPPED", out.State)
	}
	msg.SenderTeam, msg.Team = 2, 1 // sent to the cut-off team
	if out := Degrade(1, msg, dropTeam1); out.State != StateDropped {
		t.Errorf("message to the cut-off team: %s, want DROPPED", out.State)
	}
	msg.SenderTeam, msg.Team = 2, 2 // neither end affected
	if out := Degrade(1, msg, dropTeam1); out.State != StateNormal {
		t.Errorf("message between unaffected teams: %s, want NORMAL", out.State)
	}
}

func TestRecorderIsAppendOnlyAndHandsOutCopies(t *testing.T) {
	var r Recorder
	pid := id("p")
	first := r.append(1000, t0, EvPhaseStarted, 0, &pid, map[string]string{"title": "Assessment"})
	r.append(2000, t0, EvReportGenerated, 1, nil, map[string]int{"n": 1})

	// Scribbling on what the recorder returned must not change the record.
	first.Payload[2] = 'X'
	*first.ParticipantID = id("someone-else")
	read := r.Since(0)
	read[1].Payload[2] = 'X'
	read[0].Type = "TAMPERED"

	got := r.Since(0)
	if len(got) != 2 || got[0].Seq != 1 || got[1].Seq != 2 {
		t.Fatalf("sequence numbers are not consecutive from 1: %+v", got)
	}
	if string(got[0].Payload) != `{"title":"Assessment"}` || string(got[1].Payload) != `{"n":1}` {
		t.Errorf("payload was modified through a returned copy: %s %s", got[0].Payload, got[1].Payload)
	}
	if got[0].Type != EvPhaseStarted || *got[0].ParticipantID != pid {
		t.Errorf("event was modified through a returned copy: %+v", got[0])
	}
	if len(r.Since(1)) != 1 || r.Since(2) != nil || r.Since(-5)[0].Seq != 1 {
		t.Error("Since returns the wrong window")
	}
}
