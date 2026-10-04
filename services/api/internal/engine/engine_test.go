package engine

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The worked example from the brief.
func exampleDef() Definition {
	def := baseDef(360)
	def.Reports = []Report{report("report-a", 60, 0), report("report-b", 180, 0)}
	delay := rule("feed-delay", ModeDelay, 120, 300)
	delay.DelayMs = 30 * sec
	def.Rules = []Rule{delay}
	return def
}

func TestBriefExampleTimeline(t *testing.T) {
	s := start(t, exampleDef(), 1000)
	s.to(360)

	wantLines(t, s.events,
		"T+0 EXERCISE_STARTED",
		"T+0 PHASE_STARTED",
		"T+60 REPORT_GENERATED",
		"T+60 REPORT_DELIVERED team=1",
		"T+60 REPORT_DELIVERED team=2",
		"T+120 COMMUNICATION_DEGRADED",
		"T+180 REPORT_GENERATED",
		"T+180 REPORT_DELAYED team=1",
		"T+180 REPORT_DELAYED team=2",
		"T+210 REPORT_DELIVERED team=1",
		"T+210 REPORT_DELIVERED team=2",
		"T+300 COMMUNICATION_RESTORED",
		"T+360 PHASE_COMPLETED",
		"T+360 EXERCISE_ENDED",
	)
	for i, e := range s.events {
		if e.Seq != int64(i+1) {
			t.Fatalf("event %d has seq %d", i, e.Seq)
		}
	}
	if s.e.Status() != StatusEnded {
		t.Errorf("status = %s, want ENDED", s.e.Status())
	}
}

func TestNormalDelivery(t *testing.T) {
	def := baseDef(120)
	def.Reports = []Report{report("report-a", 60, 0)}
	s := start(t, def, 1000)

	s.to(59.9)
	if n := len(s.view(alice, 59.9).Reports); n != 0 {
		t.Fatalf("report visible %d ms early", 100)
	}
	if got := s.toTrainee(alice, NReportReceived); len(got) != 0 {
		t.Fatal("trainee was notified before the report was generated")
	}

	s.to(60)
	delivered := s.only(EvReportDelivered)
	if len(delivered) != 2 {
		t.Fatalf("delivered to %d teams, want 2", len(delivered))
	}
	p := payload[deliveryPayload](t, delivered[0])
	if p.State != StateNormal || p.DelayMs != 0 || *p.DeliverAtMs != 60*sec || len(p.Applied) != 0 {
		t.Errorf("delivery record = %+v", p)
	}

	notes := s.toTrainee(alice, NReportReceived)
	if len(notes) != 1 {
		t.Fatalf("alice got %d report notifications, want 1", len(notes))
	}
	view := notes[0].Payload.(ReportView)
	if view.Source != "Synthetic Sensor Alpha" || view.SentAtMs != 60*sec || view.ReceivedAtMs != 60*sec || !strings.Contains(view.Content, "eastern route") {
		t.Errorf("report view = %+v", view)
	}
	if got := s.view(alice, 60).Reports; len(got) != 1 || got[0] != view {
		t.Errorf("snapshot view differs from the live notification: %+v", got)
	}
}

func TestChannelBaseLatencyIsNotDegradation(t *testing.T) {
	def := baseDef(120)
	def.Channels[2].BaseLatencyMs = 4 * sec // the feed is always 4 s behind
	def.Reports = []Report{report("report-a", 60, 1)}
	s := start(t, def, 1000)
	s.to(120)

	wantLines(t, s.only(EvReportGenerated, EvReportDelayed, EvReportDelivered),
		"T+60 REPORT_GENERATED team=1",
		"T+64 REPORT_DELIVERED team=1",
	)
	if p := payload[deliveryPayload](t, s.only(EvReportDelivered)[0]); p.State != StateNormal {
		t.Errorf("state = %s, want NORMAL", p.State)
	}
}

func TestDelayedDelivery(t *testing.T) {
	s := start(t, exampleDef(), 1000)

	s.to(209.999)
	// The report exists and is on its way, but no trainee can know that.
	if got := s.view(alice, 209.999).Reports; len(got) != 1 {
		t.Fatalf("alice holds %d reports before the delayed one lands, want 1", len(got))
	}
	if got := s.toTrainee(alice, NReportReceived); len(got) != 1 {
		t.Fatalf("alice was sent %d reports, want only the first", len(got))
	}
	for _, n := range s.notes {
		if !n.Audience.Instructor && strings.Contains(mustJSON(t, n.Payload), "report-b") {
			t.Fatalf("a trainee notification mentions the delayed report before it arrived: %s", n.Type)
		}
	}
	delayed := payload[deliveryPayload](t, s.only(EvReportDelayed)[0])
	if delayed.State != StateDelayed || delayed.DelayMs != 30*sec || *delayed.DeliverAtMs != 210*sec || delayed.Applied[0] != id("feed-delay") {
		t.Errorf("delay record = %+v", delayed)
	}

	s.to(210)
	got := s.view(alice, 210).Reports
	if len(got) != 2 {
		t.Fatalf("alice holds %d reports, want 2", len(got))
	}
	// The trainee can see it was sent 30 s before it arrived.
	if got[1].SentAtMs != 180*sec || got[1].ReceivedAtMs != 210*sec {
		t.Errorf("sent/received = %d/%d, want 180000/210000", got[1].SentAtMs, got[1].ReceivedAtMs)
	}
}

func TestDroppedDelivery(t *testing.T) {
	def := baseDef(300)
	def.Reports = []Report{report("report-a", 60, 0), report("report-b", 180, 0)}
	drop := rule("blackout", ModeDropout, 150, 200)
	drop.Team = 1
	def.Rules = []Rule{drop}
	s := start(t, def, 1000)
	s.to(300)

	wantLines(t, s.only(EvReportGenerated, EvReportDelivered, EvReportDropped),
		"T+60 REPORT_GENERATED",
		"T+60 REPORT_DELIVERED team=1",
		"T+60 REPORT_DELIVERED team=2",
		"T+180 REPORT_GENERATED",
		"T+180 REPORT_DROPPED team=1",
		"T+180 REPORT_DELIVERED team=2",
	)
	dropped := payload[deliveryPayload](t, s.only(EvReportDropped)[0])
	if dropped.State != StateDropped || dropped.DeliverAtMs != nil || dropped.Content != "" || dropped.Applied[0] != id("blackout") {
		t.Errorf("drop record = %+v", dropped)
	}

	// Team 1 never learns report B existed, live or on reconnect.
	if got := s.view(alice, 300).Reports; len(got) != 1 || got[0].Title != "report-a" {
		t.Errorf("alice's reports = %+v", got)
	}
	if got := s.toTrainee(alice, NReportReceived); len(got) != 1 {
		t.Errorf("alice was sent %d reports, want 1", len(got))
	}
	if got := s.view(bela, 300).Reports; len(got) != 2 {
		t.Errorf("bela holds %d reports, want 2", len(got))
	}
	// The instructor's summary shows the loss.
	teams := s.e.InstructorView(wallAt(300)).Teams
	if teams[0].DroppedReports != 1 || teams[0].ReportsHeld != 1 || teams[1].DroppedReports != 0 || teams[1].ReportsHeld != 2 {
		t.Errorf("team status = %+v", teams)
	}
}

func TestPartialDelivery(t *testing.T) {
	def := baseDef(120)
	def.Reports = []Report{report("report-a", 60, 0)}
	partial := rule("fragments", ModePartial, 30, 90)
	partial.KeepPercent = 50
	def.Rules = []Rule{partial}
	s := start(t, def, 1000)
	s.to(120)

	original := def.Reports[0].Content
	generated := payload[generatedPayload](t, s.only(EvReportGenerated)[0])
	if generated.Content != original {
		t.Errorf("the record of what was sent is not the original: %q", generated.Content)
	}
	rec := payload[deliveryPayload](t, s.only(EvReportDelivered)[0])
	if rec.State != StatePartial || rec.Content == original || !strings.Contains(rec.Content, Redacted) {
		t.Errorf("partial record = %+v", rec)
	}
	got := s.view(alice, 120).Reports[0]
	if got.Content != rec.Content {
		t.Error("the trainee saw something other than what was recorded as delivered")
	}
}

func TestCommunicationRestoration(t *testing.T) {
	def := baseDef(400)
	def.Reports = []Report{report("during", 150, 1), report("at-restore", 200, 1), report("after", 250, 1)}
	drop := rule("blackout", ModeDropout, 100, 200)
	def.Rules = []Rule{drop}
	s := start(t, def, 1000)
	s.to(400)

	wantLines(t, s.only(EvCommunicationDegraded, EvCommunicationRestored, EvReportDropped, EvReportDelivered),
		"T+100 COMMUNICATION_DEGRADED",
		"T+150 REPORT_DROPPED team=1",
		"T+200 COMMUNICATION_RESTORED",
		// The window is [start, end): a report at the instant of restoration gets through.
		"T+200 REPORT_DELIVERED team=1",
		"T+250 REPORT_DELIVERED team=1",
	)
	restored := payload[rulePayload](t, s.only(EvCommunicationRestored)[0])
	if restored.RuleID != id("blackout") || len(restored.StillActive) != 0 {
		t.Errorf("restore record = %+v", restored)
	}
	if v := s.e.InstructorView(wallAt(400)); len(v.ActiveRules) != 0 {
		t.Errorf("rules still active after restoration: %+v", v.ActiveRules)
	}
}

func TestRestorationReportsWhatIsStillDegraded(t *testing.T) {
	def := baseDef(400)
	delay := rule("lag", ModeDelay, 50, 300)
	delay.DelayMs = 10 * sec
	drop := rule("blackout", ModeDropout, 100, 200)
	forced := rule("clear-window", ModeNormal, 220, 260)
	def.Rules = []Rule{delay, drop, forced}
	def.Reports = []Report{report("in-clear-window", 240, 1), report("after-clear-window", 270, 1)}
	s := start(t, def, 1000)
	s.to(400)

	wantLines(t, s.only(EvCommunicationDegraded, EvCommunicationRestored, EvReportDelayed, EvReportDelivered),
		"T+50 COMMUNICATION_DEGRADED",
		"T+100 COMMUNICATION_DEGRADED",
		"T+200 COMMUNICATION_RESTORED", // blackout over, but the lag remains
		"T+220 COMMUNICATION_RESTORED", // forced clear
		"T+240 REPORT_DELIVERED team=1",
		"T+260 COMMUNICATION_DEGRADED", // forced window over: the lag applies again
		"T+270 REPORT_DELAYED team=1",
		"T+280 REPORT_DELIVERED team=1",
		"T+300 COMMUNICATION_RESTORED",
	)
	events := s.only(EvCommunicationRestored, EvCommunicationDegraded)
	if p := payload[rulePayload](t, events[2]); len(p.StillActive) != 1 || p.StillActive[0] != id("lag") {
		t.Errorf("blackout restore should list the lag as still active: %+v", p)
	}
	if p := payload[rulePayload](t, events[4]); p.Reason != "FORCED_NORMAL_ENDED" {
		t.Errorf("reason = %q", p.Reason)
	}
}

func conflictDef() Definition {
	def := baseDef(200)
	c := rule("pass-status", ModeConflicting, 100, 100)
	c.ChannelID = &chFeed
	c.Conflict = &Conflict{
		Topic:   "Northern crossing",
		SourceA: "Synthetic Sensor Alpha", ContentA: "Crossing is open.",
		SourceB: "Synthetic Observer Bravo", ContentB: "Crossing is blocked.",
	}
	def.Rules = []Rule{c}
	return def
}

func TestConflictingReports(t *testing.T) {
	s := start(t, conflictDef(), 1000)
	s.to(200)

	wantLines(t, s.only(EvReportGenerated, EvReportDelivered),
		"T+100 REPORT_GENERATED",
		"T+100 REPORT_DELIVERED team=1",
		"T+100 REPORT_DELIVERED team=2",
		"T+100 REPORT_GENERATED",
		"T+100 REPORT_DELIVERED team=1",
		"T+100 REPORT_DELIVERED team=2",
	)

	// The trainee sees two ordinary reports from two sources.
	got := s.view(alice, 200).Reports
	if len(got) != 2 {
		t.Fatalf("alice holds %d reports, want 2", len(got))
	}
	if got[0].Source != "Synthetic Sensor Alpha" || got[0].Content != "Crossing is open." ||
		got[1].Source != "Synthetic Observer Bravo" || got[1].Content != "Crossing is blocked." {
		t.Errorf("reports = %+v", got)
	}
	// Nothing the trainee receives says the reports conflict or which is right.
	for _, n := range s.notes {
		if n.Audience.Instructor {
			continue
		}
		raw := strings.ToLower(mustJSON(t, n.Payload))
		for _, leak := range []string{"contradict", "conflict", "state", "truth", "correct"} {
			if strings.Contains(raw, leak) {
				t.Fatalf("trainee notification %s leaks %q: %s", n.Type, leak, raw)
			}
		}
	}
	if raw := strings.ToLower(mustJSON(t, s.view(alice, 200))); strings.Contains(raw, "contradict") || strings.Contains(raw, "conflict") {
		t.Fatalf("trainee view leaks the conflict: %s", raw)
	}

	// The record links both and marks them contradictory, with neither favoured.
	var groups []uuid.UUID
	for _, e := range s.only(EvReportDelivered) {
		p := payload[deliveryPayload](t, e)
		if p.State != StateContradictory || p.Conflict == nil {
			t.Fatalf("delivery record = %+v", p)
		}
		groups = append(groups, *p.Conflict)
	}
	for _, g := range groups {
		if g != groups[0] {
			t.Fatal("the two reports are not in the same conflict group")
		}
	}
	a := payload[generatedPayload](t, s.only(EvReportGenerated)[0])
	b := payload[generatedPayload](t, s.only(EvReportGenerated)[1])
	if a.ItemID == b.ItemID || a.Content != "Crossing is open." || b.Content != "Crossing is blocked." {
		t.Errorf("both sides must be recorded as distinct items: %+v / %+v", a, b)
	}
}

func TestConflictingReportsAreSubjectToOtherRules(t *testing.T) {
	def := conflictDef()
	drop := rule("blackout", ModeDropout, 90, 110)
	drop.Team = 2
	delay := rule("lag", ModeDelay, 90, 110)
	delay.Team = 1
	delay.DelayMs = 20 * sec
	def.Rules = append(def.Rules, drop, delay)
	s := start(t, def, 1000)
	s.to(200)

	wantLines(t, s.only(EvReportDelayed, EvReportDropped, EvReportDelivered),
		"T+100 REPORT_DELAYED team=1",
		"T+100 REPORT_DROPPED team=2",
		"T+100 REPORT_DELAYED team=1",
		"T+100 REPORT_DROPPED team=2",
		"T+120 REPORT_DELIVERED team=1",
		"T+120 REPORT_DELIVERED team=1",
	)
	if p := payload[deliveryPayload](t, s.only(EvReportDelivered)[0]); p.State != StateContradictory || p.DelayMs != 20*sec {
		t.Errorf("delayed conflicting report = %+v", p)
	}
	if p := payload[deliveryPayload](t, s.only(EvReportDropped)[0]); p.State != StateDropped {
		t.Errorf("dropped conflicting report = %+v", p)
	}
}

func TestPauseAndResume(t *testing.T) {
	s := start(t, exampleDef(), 1000)
	s.to(190) // report B is in transit, due at T+210

	out, err := s.e.Pause(wallAt(190))
	if err != nil {
		t.Fatal(err)
	}
	s.take(out)
	before := len(s.events)

	// An hour passes on the wall. Nothing happens in the exercise.
	s.to(190 + 3600)
	if len(s.events) != before {
		t.Fatalf("%d events were processed while paused", len(s.events)-before)
	}
	if now := s.e.Now(wallAt(190 + 3600)); now != 190*sec {
		t.Fatalf("simulation time moved to %d while paused", now)
	}
	if _, _, err := s.e.SendMessage(wallAt(200), alice.ID, chTeam, "anyone there?"); err == nil {
		t.Fatal("a message was accepted while paused")
	} else {
		wantCode(t, err, "EXERCISE_PAUSED")
	}
	if _, err := s.e.Pause(wallAt(200)); err == nil {
		t.Fatal("pausing twice succeeded")
	}

	out, err = s.e.Resume(wallAt(190 + 3600))
	if err != nil {
		t.Fatal(err)
	}
	s.take(out)
	if _, err := s.e.Resume(wallAt(190 + 3600)); err == nil {
		t.Fatal("resuming twice succeeded")
	}

	// 19.9 simulated seconds later the report is still in transit; at 20 it lands.
	s.to(190 + 3600 + 19.9)
	if n := len(s.only(EvReportDelivered)); n != 2 {
		t.Fatalf("delayed report arrived early after resume (%d deliveries)", n)
	}
	s.to(190 + 3600 + 20)
	s.to(190 + 3600 + 170)

	wantLines(t, s.only(EvExercisePaused, EvExerciseResumed, EvReportDelayed, EvReportDelivered, EvCommunicationRestored, EvExerciseEnded),
		"T+60 REPORT_DELIVERED team=1",
		"T+60 REPORT_DELIVERED team=2",
		"T+180 REPORT_DELAYED team=1",
		"T+180 REPORT_DELAYED team=2",
		"T+190 EXERCISE_PAUSED",
		"T+190 EXERCISE_RESUMED",
		"T+210 REPORT_DELIVERED team=1",
		"T+210 REPORT_DELIVERED team=2",
		"T+300 COMMUNICATION_RESTORED",
		"T+360 EXERCISE_ENDED",
	)
	if len(s.toTrainee(bela, NSimulationPaused)) != 1 || len(s.toTrainee(bela, NSimulationResumed)) != 1 {
		t.Error("trainees were not told about the pause and resume")
	}
}

func TestSimulationSpeed(t *testing.T) {
	s := start(t, exampleDef(), 5000) // 1 real second = 5 simulation seconds

	s.to(11.9) // T+59.5
	if n := len(s.only(EvReportDelivered)); n != 0 {
		t.Fatal("report A arrived before T+60")
	}
	s.to(12) // T+60
	if n := len(s.only(EvReportDelivered)); n != 2 {
		t.Fatalf("at 12 real seconds (T+60) there are %d deliveries, want 2", n)
	}

	// Slow to real time at T+100 (20 real seconds in).
	out, err := s.e.SetSpeed(wallAt(20), 1000)
	if err != nil {
		t.Fatal(err)
	}
	s.take(out)
	if now := s.e.Now(wallAt(30)); now != 110*sec {
		t.Fatalf("10 s after slowing to 1x at T+100, sim time is %d, want 110000", now)
	}
	s.to(39.9) // T+119.9
	if n := len(s.only(EvCommunicationDegraded)); n != 0 {
		t.Fatal("the delay started early after a speed change")
	}
	s.to(40) // T+120
	if n := len(s.only(EvCommunicationDegraded)); n != 1 {
		t.Fatal("the delay did not start at T+120 after a speed change")
	}

	if _, err := s.e.SetSpeed(wallAt(40), 100_000); err == nil {
		t.Fatal("an absurd speed was accepted")
	}
	changed := s.only(EvSpeedChanged)
	if len(changed) != 1 || changed[0].SimMs != 100*sec {
		t.Errorf("speed change record = %+v", changed)
	}
}

func TestEndingEarlyAndCommandsAfterTheEnd(t *testing.T) {
	s := start(t, exampleDef(), 1000)
	s.to(190) // report B still in transit
	out, err := s.e.End(wallAt(190))
	if err != nil {
		t.Fatal(err)
	}
	s.take(out)
	s.to(10_000)

	last := s.events[len(s.events)-1]
	if last.Type != EvExerciseEnded || last.SimMs != 190*sec {
		t.Fatalf("last event = %s at %d", last.Type, last.SimMs)
	}
	if n := len(s.only(EvReportDelivered)); n != 2 {
		t.Errorf("an item was delivered after the exercise ended (%d deliveries)", n)
	}
	if now := s.e.Now(wallAt(10_000)); now != 190*sec {
		t.Errorf("clock kept running after the end: %d", now)
	}

	_, _, err = s.e.SendMessage(wallAt(200), alice.ID, chTeam, "hello")
	wantCode(t, err, "EXERCISE_ENDED")
	_, err = s.e.Pause(wallAt(200))
	wantCode(t, err, "NOT_RUNNING")
	_, err = s.e.End(wallAt(200))
	wantCode(t, err, "NOT_RUNNING")
	_, err = s.e.Start(wallAt(200))
	wantCode(t, err, "ALREADY_STARTED")
}

func TestEventOrderingAtTheSameInstant(t *testing.T) {
	def := baseDef(200)
	def.Phases = []Phase{
		{ID: id("p1"), Title: "One", StartMs: 0, EndMs: 100 * sec},
		{ID: id("p2"), Title: "Two", StartMs: 100 * sec, EndMs: 200 * sec},
	}
	ending := rule("ends-at-100", ModeDropout, 50, 100)
	starting := rule("starts-at-100", ModeDelay, 100, 150)
	starting.DelayMs = 5 * sec
	def.Rules = []Rule{starting, ending} // deliberately not in time order
	def.Reports = []Report{report("second", 100, 1), report("first", 100, 1)}
	def.Reports[0].AtMs, def.Reports[1].AtMs = 100*sec, 100*sec
	def.DecisionPoints = []DecisionPoint{
		{ID: id("closes-at-100"), OpenMs: 40 * sec, CloseMs: 100 * sec, Scope: ScopeTeam, Options: twoOptions()},
		{ID: id("opens-at-100"), OpenMs: 100 * sec, CloseMs: 150 * sec, Scope: ScopeTeam, Options: twoOptions()},
	}
	s := start(t, def, 1000)
	s.to(99)
	before := len(s.events)
	s.to(100)

	wantLines(t, s.events[before:],
		"T+100 PHASE_COMPLETED",
		"T+100 PHASE_STARTED",
		"T+100 COMMUNICATION_RESTORED",
		"T+100 COMMUNICATION_DEGRADED",
		"T+100 DECISION_POINT_CLOSED",
		// Both reports meet the new rule, not the one that just ended, and
		// keep the order they were authored in.
		"T+100 REPORT_GENERATED team=1",
		"T+100 REPORT_DELAYED team=1",
		"T+100 REPORT_GENERATED team=1",
		"T+100 REPORT_DELAYED team=1",
		"T+100 DECISION_POINT_OPENED",
	)
	generated := s.only(EvReportGenerated)
	if payload[generatedPayload](t, generated[0]).Title != "second" || payload[generatedPayload](t, generated[1]).Title != "first" {
		t.Error("reports at the same instant did not keep their authored order")
	}
}

func TestManySimultaneousEventsAreAllProcessedInOneStep(t *testing.T) {
	def := baseDef(100)
	for i := 0; i < 40; i++ {
		def.Reports = append(def.Reports, report("r"+string(rune('A'+i%26))+string(rune('a'+i/26)), 50, 0))
	}
	s := start(t, def, 1000)
	s.to(50)

	if n := len(s.only(EvReportGenerated)); n != 40 {
		t.Fatalf("%d of 40 simultaneous reports were generated", n)
	}
	delivered := s.only(EvReportDelivered)
	if len(delivered) != 80 {
		t.Fatalf("%d deliveries, want 80", len(delivered))
	}
	seen := map[uuid.UUID]bool{}
	for i, e := range delivered {
		if e.SimMs != 50*sec {
			t.Fatalf("delivery %d recorded at %d", i, e.SimMs)
		}
		p := payload[deliveryPayload](t, e)
		if seen[p.DeliveryID] {
			t.Fatal("two deliveries share an id")
		}
		seen[p.DeliveryID] = true
	}
	got := s.view(alice, 50).Reports
	for i := range got {
		if got[i].Title != def.Reports[i].Title {
			t.Fatalf("report %d arrived out of authored order", i)
		}
	}
}

func TestMultipleTeamsSeeOnlyTheirOwnInformation(t *testing.T) {
	def := baseDef(300)
	def.Reports = []Report{report("for-everyone", 30, 0), report("for-team-1", 60, 1), report("for-team-2", 90, 2)}
	partial := rule("fragments-team-2", ModePartial, 0, 300)
	partial.Team = 2
	partial.KeepPercent = 30
	def.Rules = []Rule{partial}
	s := start(t, def, 1000)
	s.to(300)

	titles := func(v TraineeView) string {
		var out []string
		for _, r := range v.Reports {
			out = append(out, r.Title)
		}
		return strings.Join(out, ",")
	}
	if got := titles(s.view(alice, 300)); got != "for-everyone,for-team-1" {
		t.Errorf("team 1 sees %q", got)
	}
	if got := titles(s.view(arun, 300)); got != "for-everyone,for-team-1" {
		t.Errorf("alice's team-mate sees %q", got)
	}
	if got := titles(s.view(bela, 300)); got != "for-everyone,for-team-2" {
		t.Errorf("team 2 sees %q", got)
	}

	// Same report, different experience: team 1 gets it whole, team 2 in fragments.
	whole, fragments := s.view(alice, 300).Reports[0].Content, s.view(bela, 300).Reports[0].Content
	if whole != def.Reports[0].Content || fragments == whole || !strings.Contains(fragments, Redacted) {
		t.Errorf("team 1: %q\nteam 2: %q", whole, fragments)
	}

	// No notification addressed to a trainee carries another team's item, and
	// nothing instructor-only is addressed to trainees at all.
	for _, n := range s.notes {
		if n.Audience.Instructor {
			continue
		}
		raw := mustJSON(t, n.Payload)
		if n.Audience.Includes(alice) && strings.Contains(raw, "for-team-2") {
			t.Fatalf("team 1 was sent team 2's report via %s", n.Type)
		}
		if n.Audience.Includes(bela) && strings.Contains(raw, "for-team-1") {
			t.Fatalf("team 2 was sent team 1's report via %s", n.Type)
		}
		switch n.Type {
		case NSimulationStarted, NSimulationPaused, NSimulationResumed, NSimulationCompleted, NSimulationTime,
			NReportReceived, NDecisionOpened, NDecisionClosed, NDecisionSubmitted, NMessageReceived, NPhaseChanged:
		default:
			t.Fatalf("instructor-only notification %q was addressed to trainees", n.Type)
		}
	}
}

func TestInstructorStreamMirrorsTheTimeline(t *testing.T) {
	s := start(t, exampleDef(), 1000)
	s.to(360)

	var mirrored []Event
	for _, n := range s.notes {
		if n.Audience.Instructor {
			if n.Audience.AllTrainees || len(n.Audience.Teams) > 0 || len(n.Audience.Participants) > 0 {
				t.Fatalf("a notification is addressed to both the instructor and trainees: %s", n.Type)
			}
			mirrored = append(mirrored, n.Payload.(Event))
		}
	}
	if len(mirrored) != len(s.events) {
		t.Fatalf("instructor received %d events, timeline has %d", len(mirrored), len(s.events))
	}
	for i := range mirrored {
		if mirrored[i].Seq != s.events[i].Seq || mirrored[i].Type != s.events[i].Type {
			t.Fatalf("instructor stream diverges from the timeline at %d", i)
		}
	}
	// Names the brief asks for, as the instructor sees them.
	names := map[string]bool{}
	for _, n := range s.notes {
		if n.Audience.Instructor {
			names[n.Type] = true
		}
	}
	for _, want := range []string{"simulation.started", "phase.changed", "report.received", "report.delayed", "communication.degraded", "communication.restored", "simulation.completed"} {
		if !names[want] {
			t.Errorf("instructor stream has no %q", want)
		}
	}
}

func twoOptions() []DecisionOption {
	return []DecisionOption{{ID: id("opt-a"), Label: "Route A"}, {ID: id("opt-b"), Label: "Route B"}}
}

func TestNewRejectsBadInput(t *testing.T) {
	if _, err := New(baseDef(100), roster(), 1, 0); err != ErrSpeedOutOfRange {
		t.Errorf("zero speed: %v", err)
	}
	if _, err := New(Definition{TeamCount: 2}, roster(), 1, 1000); err == nil {
		t.Error("a definition with no phases was accepted")
	}
	stranger := []Participant{{ID: id("x"), Name: "X", Team: 3}}
	if _, err := New(baseDef(100), stranger, 1, 1000); err == nil {
		t.Error("a participant on a team that does not exist was accepted")
	}
}
