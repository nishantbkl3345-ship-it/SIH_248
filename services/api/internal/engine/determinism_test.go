package engine

import (
	"encoding/json"
	"testing"
)

// busyDef exercises every feature at once, with several things on the same
// instant, so that any ordering that depended on chance would show up.
func busyDef() Definition {
	def := baseDef(600)
	def.Phases = []Phase{
		{ID: id("p1"), Title: "One", StartMs: 0, EndMs: 300 * sec},
		{ID: id("p2"), Title: "Two", StartMs: 300 * sec, EndMs: 600 * sec},
	}
	def.Channels[2].BaseLatencyMs = 1500
	for i, at := range []int64{10, 60, 60, 120, 180, 180, 180, 240, 300, 300, 360, 420, 480, 540} {
		def.Reports = append(def.Reports, report("r"+string(rune('a'+i)), at, i%3))
	}
	delay := rule("lag", ModeDelay, 100, 260)
	delay.DelayMs = 25 * sec
	drop := rule("blackout", ModeDropout, 170, 190)
	drop.Team = 2
	partial := rule("fragments", ModePartial, 290, 380)
	partial.KeepPercent = 45
	clear := rule("clear", ModeNormal, 350, 370)
	conflict := rule("conflict", ModeConflicting, 300, 300)
	conflict.ChannelID = &chFeed
	conflict.Conflict = &Conflict{Topic: "Crossing", SourceA: "Synthetic Sensor Alpha", ContentA: "Open.", SourceB: "Synthetic Observer Bravo", ContentB: "Blocked."}
	def.Rules = []Rule{delay, drop, partial, clear, conflict}
	def.DecisionPoints = []DecisionPoint{
		{ID: id("d1"), OpenMs: 200 * sec, CloseMs: 300 * sec, Scope: ScopeTeam, Question: "Route?", RationaleRequired: true, Options: twoOptions()},
		{ID: id("d2"), OpenMs: 400 * sec, CloseMs: 600 * sec, Scope: ScopeIndividual, Question: "Sure?", Options: twoOptions()},
	}
	return def
}

// script is a fixed set of trainee and instructor actions at fixed wall times.
func script(t *testing.T, e *Engine, collect func(Output)) []func() (float64, func()) {
	t.Helper()
	must := func(out Output, err error) {
		if err != nil {
			t.Fatalf("scripted action failed: %v", err)
		}
		collect(out)
	}
	at := func(sec float64, run func()) func() (float64, func()) {
		return func() (float64, func()) { return sec, run }
	}
	return []func() (float64, func()){
		at(55, func() { _, out, err := e.SendMessage(wallAt(55), alice.ID, chTeam, "status?"); must(out, err) }),
		at(175, func() {
			_, out, err := e.SendMessage(wallAt(175), bela.ID, chCross, "anyone on the net?")
			must(out, err)
		}),
		at(210, func() {
			_, out, err := e.SubmitDecision(wallAt(210), DecisionInput{ParticipantID: arun.ID, DecisionPointID: id("d1"), OptionID: id("opt-a"), Rationale: "confirmed route"})
			must(out, err)
		}),
		at(250, func() { must(e.Pause(wallAt(250))) }),
		at(900, func() { must(e.Resume(wallAt(900))) }),
		// From here wall time is 650 s ahead of simulation time, then 4x.
		at(950, func() { must(e.SetSpeed(wallAt(950), 4000)) }), // T+300
		at(980, func() { // T+420
			_, out, err := e.SubmitDecision(wallAt(980), DecisionInput{ParticipantID: bela.ID, DecisionPointID: id("d2"), OptionID: id("opt-b")})
			must(out, err)
		}),
	}
}

// run plays the script, calling Advance every tick seconds of wall time in
// between, and returns the full timeline.
func run(t *testing.T, seed int64, tick float64) []Event {
	t.Helper()
	e, err := New(busyDef(), roster(), seed, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	collect := func(out Output) { events = append(events, out.Events...) }
	out, err := e.Start(t0)
	if err != nil {
		t.Fatal(err)
	}
	collect(out)

	const endWall = 1100.0
	now := 0.0
	advanceTo := func(target float64) {
		for tick > 0 && now+tick < target {
			now += tick
			collect(e.Advance(wallAt(now)))
		}
		now = target
	}
	for _, step := range script(t, e, collect) {
		at, action := step()
		advanceTo(at)
		action()
	}
	advanceTo(endWall)
	collect(e.Advance(wallAt(endWall)))

	if e.Status() != StatusEnded {
		t.Fatalf("exercise did not finish: %s", e.Status())
	}
	if full := e.Timeline(0); len(full) != len(events) {
		t.Fatalf("collected %d events but the timeline holds %d", len(events), len(full))
	}
	return events
}

func asJSON(events []Event) string {
	raw, _ := json.Marshal(events)
	return string(raw)
}

func TestSameInputsProduceTheIdenticalTimeline(t *testing.T) {
	first := asJSON(run(t, 42, 0.25))
	for i := 0; i < 5; i++ {
		if again := asJSON(run(t, 42, 0.25)); again != first {
			t.Fatalf("run %d differs from the first run with identical inputs", i+2)
		}
	}
}

func TestTimelineDoesNotDependOnHowOftenTheEngineIsPolled(t *testing.T) {
	reference := run(t, 42, 0.25)
	if len(reference) < 60 {
		t.Fatalf("the busy scenario produced only %d events; it is not exercising much", len(reference))
	}
	// The whole record is compared, wall-clock stamps and real-time response
	// times included: scheduled events are stamped with the moment they were
	// due, not the moment the engine was next polled.
	want := asJSON(reference)
	// From 27 times a second to never at all between actions.
	for _, tick := range []float64{0.037, 1, 7.3, 61, 0} {
		got := run(t, 42, tick)
		if asJSON(got) == want {
			continue
		}
		for i := range got {
			if i >= len(reference) || asJSON(got[i:i+1]) != asJSON(reference[i:i+1]) {
				t.Errorf("polling every %g s diverges at event %d:\n got %s\nwant %s", tick, i, asJSON(got[i:i+1]), asJSON(reference[min(i, len(reference)-1):min(i, len(reference)-1)+1]))
				break
			}
		}
	}
}

func TestSeedChangesOnlyWhatIsSeeded(t *testing.T) {
	a, b := run(t, 42, 5), run(t, 43, 5)
	if len(a) != len(b) {
		t.Fatalf("a different seed changed the number of events: %d vs %d", len(a), len(b))
	}
	differs := false
	for i := range a {
		if a[i].Type != b[i].Type || a[i].SimMs != b[i].SimMs || a[i].Team != b[i].Team {
			t.Fatalf("a different seed changed the structure of the timeline at %d: %s vs %s", i, a[i].Type, b[i].Type)
		}
		if string(a[i].Payload) != string(b[i].Payload) {
			differs = true
		}
	}
	if !differs {
		t.Error("the seed has no effect at all")
	}
}

func TestTimelineIsStrictlyOrdered(t *testing.T) {
	events := run(t, 42, 3)
	for i, e := range events {
		if e.Seq != int64(i+1) {
			t.Fatalf("seq %d at position %d", e.Seq, i)
		}
		if i > 0 && e.SimMs < events[i-1].SimMs {
			t.Fatalf("simulation time goes backwards at seq %d: %d after %d", e.Seq, e.SimMs, events[i-1].SimMs)
		}
		if i > 0 && e.Wall.Before(events[i-1].Wall) {
			t.Fatalf("wall time goes backwards at seq %d", e.Seq)
		}
	}
}
