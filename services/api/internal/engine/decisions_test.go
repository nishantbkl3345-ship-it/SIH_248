package engine

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

func decisionDef() Definition {
	def := baseDef(600)
	def.Reports = []Report{report("report-a", 60, 0), report("report-b", 180, 0), report("report-c", 190, 1)}
	delay := rule("feed-delay", ModeDelay, 120, 400)
	delay.ChannelID = &chFeed
	delay.DelayMs = 60 * sec
	drop := rule("team-1-blackout", ModeDropout, 185, 195)
	drop.Team = 1
	def.Rules = []Rule{delay, drop}
	def.DecisionPoints = []DecisionPoint{
		{ID: id("route"), OpenMs: 200 * sec, CloseMs: 320 * sec, Scope: ScopeTeam, Question: "Which route?", RationaleRequired: true, Options: twoOptions()},
		{ID: id("self-check"), OpenMs: 200 * sec, CloseMs: 320 * sec, Scope: ScopeIndividual, Question: "How sure are you?", Options: twoOptions()},
	}
	return def
}

func TestDecisionRecording(t *testing.T) {
	s := start(t, decisionDef(), 2000) // 2x: sim and wall response times differ
	s.to(100)                          // T+200: the points open

	opened := s.toTrainee(alice, NDecisionOpened)
	if len(opened) != 2 {
		t.Fatalf("alice was told about %d decision points, want 2", len(opened))
	}
	if raw := mustJSON(t, opened[0].Payload); strings.Contains(strings.ToLower(raw), "criteria") {
		t.Fatalf("decision point sent to trainees mentions evaluation criteria: %s", raw)
	}

	confidence := 4
	// 15 real seconds later = T+230.
	rec, out, err := s.e.SubmitDecision(wallAt(115), DecisionInput{
		ParticipantID: alice.ID, DecisionPointID: id("route"), OptionID: id("opt-b"),
		Rationale: "  Only one source so far; choosing the confirmed route.  ", Confidence: &confidence,
	})
	if err != nil {
		t.Fatalf("SubmitDecision: %v", err)
	}
	s.take(out)

	if rec.ParticipantID != alice.ID || rec.Team != 1 || rec.OptionID != id("opt-b") {
		t.Errorf("who/what: %+v", rec)
	}
	if rec.SimMs != 230*sec || !rec.Wall.Equal(wallAt(115)) {
		t.Errorf("when: sim=%d wall=%v", rec.SimMs, rec.Wall)
	}
	if rec.ResponseMs != 30*sec || rec.ResponseWallMs != 15*sec {
		t.Errorf("response time: sim=%d wall=%d, want 30000 and 15000", rec.ResponseMs, rec.ResponseWallMs)
	}
	if rec.Rationale != "Only one source so far; choosing the confirmed route." || *rec.Confidence != 4 {
		t.Errorf("rationale/confidence: %q %v", rec.Rationale, rec.Confidence)
	}

	// The information picture at T+230:
	//   report-a  delivered at T+60
	//   report-b  generated T+180, delayed 60 s -> still in transit
	//   report-c  generated T+190 for team 1 during its blackout -> dropped
	snap := rec.Snapshot
	if len(snap.Reports) != 1 || snap.Reports[0].Title != "report-a" || snap.Reports[0].State != StateNormal || snap.Reports[0].ReceivedAtMs != 60*sec {
		t.Errorf("available reports = %+v", snap.Reports)
	}
	if snap.PendingReports != 1 || snap.DroppedReports != 1 {
		t.Errorf("withheld: pending=%d dropped=%d, want 1 and 1", snap.PendingReports, snap.DroppedReports)
	}
	if len(snap.ActiveDegradation) != 1 || snap.ActiveDegradation[0].RuleID != id("feed-delay") || snap.ActiveDegradation[0].Mode != ModeDelay {
		t.Errorf("active degradation = %+v", snap.ActiveDegradation)
	}
	if snap.PhaseID != id("phase-1") || len(snap.Messages) != 0 {
		t.Errorf("phase/messages = %v %+v", snap.PhaseID, snap.Messages)
	}

	// It is on the timeline with the full record, and handed back for storage.
	submitted := s.only(EvDecisionSubmitted)
	if len(submitted) != 1 || submitted[0].Team != 1 || *submitted[0].ParticipantID != alice.ID {
		t.Fatalf("timeline entry = %+v", submitted)
	}
	if got := payload[DecisionRecord](t, submitted[0]); got.ID != rec.ID || got.Snapshot.PendingReports != 1 {
		t.Errorf("recorded payload = %+v", got)
	}
	if len(out.Decisions) != 1 || out.Decisions[0].ID != rec.ID {
		t.Errorf("output decisions = %+v", out.Decisions)
	}

	// The team is told; the other team is not, and nobody gets the snapshot.
	if len(s.toTrainee(arun, NDecisionSubmitted)) != 1 {
		t.Error("alice's team-mate was not told about the team decision")
	}
	if len(s.toTrainee(bela, NDecisionSubmitted)) != 0 {
		t.Error("the other team was told about team 1's decision")
	}
	told := mustJSON(t, s.toTrainee(arun, NDecisionSubmitted)[0].Payload)
	if strings.Contains(told, "snapshot") || strings.Contains(told, "Degradation") || strings.Contains(told, "dropped") {
		t.Errorf("the notification to the team carries ground truth: %s", told)
	}
}

func TestDecisionSnapshotIncludesCommunications(t *testing.T) {
	s := start(t, decisionDef(), 1000)
	s.to(205)
	for _, m := range []struct {
		from Participant
		ch   uuid.UUID
		body string
	}{
		{arun, chTeam, "K-7 says clear"},    // reaches alice
		{bela, chCross, "we are holding"},   // reaches alice
		{alice, chTeam, "copy, checking B"}, // alice's own
		{bela, chTeam, "team 2 only"},       // never reaches alice
	} {
		_, out, err := s.e.SendMessage(wallAt(205), m.from.ID, m.ch, m.body)
		if err != nil {
			t.Fatalf("SendMessage: %v", err)
		}
		s.take(out)
	}

	rec, _, err := s.e.SubmitDecision(wallAt(210), DecisionInput{ParticipantID: alice.ID, DecisionPointID: id("route"), OptionID: id("opt-a"), Rationale: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, m := range rec.Snapshot.Messages {
		sources = append(sources, m.Source)
	}
	if got := strings.Join(sources, ","); got != "Arun,Bela,Alice" {
		t.Errorf("communications available to alice came from %q, want Arun,Bela,Alice", got)
	}
}

func TestDecisionRules(t *testing.T) {
	submit := func(s *sim, wallSec float64, p Participant, point, option, rationale string) error {
		_, out, err := s.e.SubmitDecision(wallAt(wallSec), DecisionInput{
			ParticipantID: p.ID, DecisionPointID: id(point), OptionID: id(option), Rationale: rationale,
		})
		s.take(out)
		return err
	}

	t.Run("not open yet", func(t *testing.T) {
		s := start(t, decisionDef(), 1000)
		wantCode(t, submit(s, 199.9, alice, "route", "opt-a", "r"), "DECISION_NOT_OPEN")
	})

	t.Run("deadline is enforced by the server clock", func(t *testing.T) {
		s := start(t, decisionDef(), 1000)
		if err := submit(s, 319.999, alice, "route", "opt-a", "just in time"); err != nil {
			t.Fatalf("a decision 1 ms before the deadline was refused: %v", err)
		}
		// No Advance was called: the submit itself must notice the deadline.
		wantCode(t, submit(s, 320, bela, "route", "opt-a", "too late"), "DECISION_NOT_OPEN")
		if n := len(s.only(EvDecisionPointClosed)); n != 2 {
			t.Errorf("%d points closed, want 2", n)
		}
	})

	t.Run("validation", func(t *testing.T) {
		s := start(t, decisionDef(), 1000)
		s.to(200)
		wantCode(t, submit(s, 201, alice, "route", "opt-a", "   "), "RATIONALE_REQUIRED")
		wantCode(t, submit(s, 201, alice, "route", "no-such-option", "r"), "UNKNOWN_OPTION")
		wantCode(t, submit(s, 201, alice, "no-such-point", "opt-a", "r"), "UNKNOWN_DECISION_POINT")
		wantCode(t, submit(s, 201, Participant{ID: id("stranger")}, "route", "opt-a", "r"), "NOT_A_PARTICIPANT")
		bad := 9
		_, _, err := s.e.SubmitDecision(wallAt(201), DecisionInput{ParticipantID: alice.ID, DecisionPointID: id("route"), OptionID: id("opt-a"), Rationale: "r", Confidence: &bad})
		wantCode(t, err, "INVALID_CONFIDENCE")
		// Rationale is optional where the point does not require it.
		if err := submit(s, 201, alice, "self-check", "opt-a", ""); err != nil {
			t.Errorf("optional rationale was required: %v", err)
		}
		// None of the rejected attempts were recorded.
		if n := len(s.only(EvDecisionSubmitted)); n != 1 {
			t.Errorf("%d decisions on the timeline, want 1", n)
		}
	})

	t.Run("one decision per team for team scope", func(t *testing.T) {
		s := start(t, decisionDef(), 1000)
		s.to(200)
		if err := submit(s, 201, alice, "route", "opt-a", "r"); err != nil {
			t.Fatal(err)
		}
		wantCode(t, submit(s, 202, arun, "route", "opt-b", "r"), "ALREADY_DECIDED")
		wantCode(t, submit(s, 203, alice, "route", "opt-b", "changed my mind"), "ALREADY_DECIDED")
		if err := submit(s, 204, bela, "route", "opt-b", "r"); err != nil {
			t.Errorf("the other team could not decide: %v", err)
		}
	})

	t.Run("one decision per trainee for individual scope", func(t *testing.T) {
		s := start(t, decisionDef(), 1000)
		s.to(200)
		if err := submit(s, 201, alice, "self-check", "opt-a", ""); err != nil {
			t.Fatal(err)
		}
		if err := submit(s, 202, arun, "self-check", "opt-b", ""); err != nil {
			t.Errorf("a team-mate could not make their own individual decision: %v", err)
		}
		wantCode(t, submit(s, 203, alice, "self-check", "opt-b", ""), "ALREADY_DECIDED")
		// Individual decisions are private to their author.
		if len(s.toTrainee(arun, NDecisionSubmitted)) != 1 || len(s.toTrainee(alice, NDecisionSubmitted)) != 1 {
			t.Error("individual decisions were shared with team-mates")
		}
		mine := s.view(alice, 204).Decisions
		if len(mine) != 1 || mine[0].ParticipantID != alice.ID {
			t.Errorf("alice's view of decisions = %+v", mine)
		}
	})
}

func TestMessages(t *testing.T) {
	def := baseDef(600)
	delay := rule("coord-lag", ModeDelay, 100, 200)
	delay.ChannelID = &chCross
	delay.DelayMs = 20 * sec
	drop := rule("team-1-net-down", ModeDropout, 300, 400)
	drop.Team = 1
	def.Rules = []Rule{delay, drop}
	s := start(t, def, 1000)

	send := func(wallSec float64, from Participant, ch uuid.UUID, body string) (MessageView, error) {
		view, out, err := s.e.SendMessage(wallAt(wallSec), from.ID, ch, body)
		s.take(out)
		return view, err
	}

	// Normal: team channel reaches the team-mate, not the sender or the other team.
	sent, err := send(50, alice, chTeam, "  status?  ")
	if err != nil {
		t.Fatal(err)
	}
	if sent.Body != "status?" || sent.SenderName != "Alice" || sent.SentAtMs != 50*sec {
		t.Errorf("sender's copy = %+v", sent)
	}
	if len(s.toTrainee(arun, NMessageReceived)) != 1 || len(s.toTrainee(alice, NMessageReceived)) != 0 || len(s.toTrainee(bela, NMessageReceived)) != 0 {
		t.Error("team message went to the wrong people")
	}

	// Delayed: cross-team message during the lag arrives 20 s later for everyone.
	if _, err := send(150, bela, chCross, "holding at the junction"); err != nil {
		t.Fatal(err)
	}
	s.to(169.9)
	if len(s.toTrainee(alice, NMessageReceived)) != 0 {
		t.Fatal("delayed message arrived early")
	}
	s.to(170)
	if got := s.toTrainee(alice, NMessageReceived); len(got) != 1 || got[0].Payload.(MessageView).SentAtMs != 150*sec || got[0].Payload.(MessageView).ReceivedAtMs != 170*sec {
		t.Fatalf("delayed message = %+v", got)
	}

	// Dropped: team 1 is cut off, in both directions.
	if _, err := send(350, alice, chCross, "can anyone hear us?"); err != nil {
		t.Fatalf("the sender must not be told the message was lost: %v", err)
	}
	if _, err := send(351, bela, chCross, "team 1, respond"); err != nil {
		t.Fatal(err)
	}
	// What can and cannot be sent.
	_, err = send(360, alice, chFeed, "x")
	wantCode(t, err, "CHANNEL_NOT_WRITABLE")
	_, err = send(360, alice, id("nowhere"), "x")
	wantCode(t, err, "UNKNOWN_CHANNEL")
	_, err = send(360, alice, chTeam, "   ")
	wantCode(t, err, "INVALID_MESSAGE")
	_, err = send(360, alice, chTeam, strings.Repeat("x", MaxMessageRunes+1))
	wantCode(t, err, "INVALID_MESSAGE")
	_, err = send(360, Participant{ID: id("stranger")}, chTeam, "x")
	wantCode(t, err, "NOT_A_PARTICIPANT")

	s.to(600)

	wantLines(t, s.only(EvMessageSent, EvMessageDelivered, EvMessageDelayed, EvMessageDropped),
		"T+50 MESSAGE_SENT team=1",
		"T+50 MESSAGE_DELIVERED team=1",
		"T+150 MESSAGE_SENT team=2",
		"T+150 MESSAGE_DELAYED team=1",
		"T+150 MESSAGE_DELAYED team=2",
		"T+170 MESSAGE_DELIVERED team=1",
		"T+170 MESSAGE_DELIVERED team=2",
		"T+350 MESSAGE_SENT team=1",
		"T+350 MESSAGE_DROPPED team=1",
		"T+350 MESSAGE_DROPPED team=2",
		"T+351 MESSAGE_SENT team=2",
		"T+351 MESSAGE_DROPPED team=1",
		"T+351 MESSAGE_DELIVERED team=2",
	)

	bodies := func(p Participant) string {
		var out []string
		for _, m := range s.view(p, 600).Messages {
			out = append(out, m.Body)
		}
		return strings.Join(out, " | ")
	}
	// Alice sees her own messages (including the lost one, which looks sent) and Bela's first.
	if got := bodies(alice); got != "status? | holding at the junction | can anyone hear us?" {
		t.Errorf("alice's messages: %s", got)
	}
	if got := bodies(arun); got != "status? | holding at the junction" {
		t.Errorf("arun's messages: %s", got)
	}
	if got := bodies(bela); got != "holding at the junction | team 1, respond" {
		t.Errorf("bela's messages: %s", got)
	}

}

func TestFromScenario(t *testing.T) {
	feed, team := id("feed"), 2
	sc := &domain.Scenario{
		TeamCount: 3,
		Channels:  []domain.CommunicationChannel{{ID: feed, Name: "Field Feed", Kind: domain.ChannelFeed, BaseLatencySec: 2}},
		Phases: []domain.ScenarioPhase{
			{ID: id("p1"), Title: "One", DurationSec: 300,
				Reports: []domain.InformationReport{{
					ID: id("r1"), Title: "R1", SourceName: "Synthetic Sensor Alpha", SourceReliability: "B",
					Body: "Clear.", OffsetSec: 120, Priority: domain.PriorityHigh, ChannelID: &feed, TargetTeam: &team,
				}},
				Rules: []domain.DegradationRule{
					{ID: id("overrun"), Mode: domain.ModeDelay, OffsetSec: 250, DurationSec: 600, Params: domain.JSONB(`{"delaySec":30}`)},
				},
			},
			{ID: id("p2"), Title: "Two", DurationSec: 200,
				Rules: []domain.DegradationRule{
					{ID: id("conflict"), Mode: domain.ModeConflicting, OffsetSec: 10, ChannelID: &feed,
						Params: domain.JSONB(`{"topic":"Crossing","sourceA":"A","contentA":"Open","sourceB":"B","contentB":"Shut"}`)},
					{ID: id("partial"), Mode: domain.ModePartial, OffsetSec: 20, DurationSec: 30, Params: domain.JSONB(`{"keepPercent":40}`)},
				},
				DecisionPoints: []domain.DecisionPoint{{
					ID: id("d1"), Prompt: "Go?", Scope: domain.ScopeTeam, OffsetSec: 100, TimeLimitSec: 500, RationaleRequired: true,
					Options: []domain.DecisionOption{{ID: id("o1"), Label: "Yes"}, {ID: id("o2"), Label: "No"}},
				}},
			},
		},
	}
	def := FromScenario(sc)

	if def.TeamCount != 3 || def.TotalMs != 500*sec || def.Channels[0].BaseLatencyMs != 2*sec {
		t.Errorf("basics: %+v", def)
	}
	if def.Phases[1].StartMs != 300*sec || def.Phases[1].EndMs != 500*sec {
		t.Errorf("phase 2 = %+v", def.Phases[1])
	}
	if r := def.Reports[0]; r.AtMs != 120*sec || r.Team != 2 || r.ChannelID != feed || r.Priority != "HIGH" {
		t.Errorf("report = %+v", r)
	}
	// A window that overruns its phase is cut off at the phase boundary.
	if r := def.Rules[0]; r.StartMs != 250*sec || r.EndMs != 300*sec || r.DelayMs != 30*sec || r.Team != 0 {
		t.Errorf("overrunning rule = %+v", r)
	}
	if r := def.Rules[1]; r.StartMs != 310*sec || r.EndMs != r.StartMs || r.Conflict == nil || r.Conflict.SourceB != "B" {
		t.Errorf("conflict rule = %+v", r)
	}
	if r := def.Rules[2]; r.StartMs != 320*sec || r.EndMs != 350*sec || r.KeepPercent != 40 {
		t.Errorf("partial rule = %+v", r)
	}
	// A deadline cannot outlive the exercise.
	if d := def.DecisionPoints[0]; d.OpenMs != 400*sec || d.CloseMs != 500*sec || len(d.Options) != 2 || !d.RationaleRequired {
		t.Errorf("decision point = %+v", d)
	}

	// And the result runs.
	e, err := New(def, []Participant{{ID: id("x"), Name: "X", Team: 2}}, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Start(t0); err != nil {
		t.Fatal(err)
	}
	e.Advance(wallAt(500))
	if e.Status() != StatusEnded {
		t.Errorf("status = %s", e.Status())
	}
}
