package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
	"fogline/api/internal/repository/memrepo"
)

func intp(i int) *int { return &i }

func params(t *testing.T, p domain.RuleParams) domain.JSONB {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return domain.JSONB(raw)
}

// completeScenario is a small fictional scenario with no outstanding issues.
func completeScenario(t *testing.T) *domain.Scenario {
	t.Helper()
	team, feed := uuid.New(), uuid.New()
	return &domain.Scenario{
		Title: "Exercise Tidewatch", Summary: "Relief routing on the fictional island of Veridia.",
		Difficulty: domain.DifficultyIntermediate, TeamCount: 2, TraineeCount: 8,
		Objectives: domain.JSONB(`["Decide with incomplete information"]`),
		Channels: []domain.CommunicationChannel{
			{ID: team, Name: "Team Net", Kind: domain.ChannelTeam},
			{ID: feed, Name: "Field Feed", Kind: domain.ChannelFeed},
		},
		Phases: []domain.ScenarioPhase{
			{
				ID: uuid.New(), Title: "Assessment", DurationSec: 300,
				Reports: []domain.InformationReport{{
					ID: uuid.New(), Title: "Coastal road", SourceName: "Relay Station K-7", Body: "Passable.",
					OffsetSec: 120, Priority: domain.PriorityRoutine, SourceReliability: "A", ChannelID: &feed,
				}},
				Rules: []domain.DegradationRule{
					{ID: uuid.New(), Name: "Feed lag", Mode: domain.ModeDelay, OffsetSec: 150, DurationSec: 60,
						ChannelID: &feed, Params: params(t, domain.RuleParams{DelaySec: 30})},
					{ID: uuid.New(), Name: "Net down", Mode: domain.ModeDropout, OffsetSec: 180, DurationSec: 40,
						ChannelID: &team, TargetTeam: intp(1)},
				},
			},
			{
				ID: uuid.New(), Title: "Commitment", DurationSec: 420,
				Rules: []domain.DegradationRule{
					{ID: uuid.New(), Name: "Pass status", Mode: domain.ModeConflicting, OffsetSec: 20, ChannelID: &feed,
						Params: params(t, domain.RuleParams{
							Topic: "Inland pass", SourceA: "Relay Station K-7", ContentA: "Open.",
							SourceB: "Community Net", ContentB: "Blocked by a slide.",
						})},
					{ID: uuid.New(), Name: "Fragments", Mode: domain.ModePartial, OffsetSec: 60, DurationSec: 60,
						Params: params(t, domain.RuleParams{KeepPercent: 50})},
				},
				DecisionPoints: []domain.DecisionPoint{{
					ID: uuid.New(), Prompt: "Which route does the convoy take?", Scope: domain.ScopeTeam,
					OffsetSec: 120, TimeLimitSec: 180, RationaleRequired: true,
					EvaluationCriteria: "Sought confirmation before committing.",
					Options: []domain.DecisionOption{
						{ID: uuid.New(), Label: "Coastal road"},
						{ID: uuid.New(), Label: "Inland pass"},
					},
				}},
			},
		},
	}
}

func TestReviewAcceptsCompleteScenario(t *testing.T) {
	if issues := ReviewScenario(completeScenario(t)); len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestReviewFindsProblems(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(*domain.Scenario)
		severity string
		path     string
	}{
		{"no title", func(s *domain.Scenario) { s.Title = "  " }, SeverityError, "title"},
		{"no summary", func(s *domain.Scenario) { s.Summary = "" }, SeverityError, "summary"},
		{"no objectives", func(s *domain.Scenario) { s.Objectives = domain.JSONB(`[]`) }, SeverityError, "objectives"},
		{"blank objective", func(s *domain.Scenario) { s.Objectives = domain.JSONB(`["ok"," "]`) }, SeverityError, "objectives[1]"},
		{"fewer trainees than teams", func(s *domain.Scenario) { s.TraineeCount = 1 }, SeverityError, "traineeCount"},
		{"no channels", func(s *domain.Scenario) { s.Channels = nil }, SeverityError, "channels"},
		{"duplicate channel name", func(s *domain.Scenario) { s.Channels[1].Name = " team net " }, SeverityError, "channels[1].name"},
		{"no phases", func(s *domain.Scenario) { s.Phases = nil }, SeverityError, "phases"},
		{"untitled phase", func(s *domain.Scenario) { s.Phases[0].Title = "" }, SeverityError, "phases[0].title"},
		{"zero-length phase", func(s *domain.Scenario) { s.Phases[1].DurationSec = 0 }, SeverityError, "phases[1].durationSec"},

		{"report without source", func(s *domain.Scenario) { s.Phases[0].Reports[0].SourceName = "" }, SeverityError, "phases[0].reports[0].source"},
		{"report without content", func(s *domain.Scenario) { s.Phases[0].Reports[0].Body = "" }, SeverityError, "phases[0].reports[0].content"},
		{"report without channel", func(s *domain.Scenario) { s.Phases[0].Reports[0].ChannelID = nil }, SeverityError, "phases[0].reports[0].channelId"},
		{"report after phase end", func(s *domain.Scenario) { s.Phases[0].Reports[0].OffsetSec = 301 }, SeverityError, "phases[0].reports[0].atSec"},
		{"report to missing team", func(s *domain.Scenario) { s.Phases[0].Reports[0].TargetTeam = intp(3) }, SeverityError, "phases[0].reports[0].targetTeam"},

		{"delay without amount", func(s *domain.Scenario) { s.Phases[0].Rules[0].Params = nil }, SeverityError, "phases[0].rules[0].delaySec"},
		{"dropout without duration", func(s *domain.Scenario) { s.Phases[0].Rules[1].DurationSec = 0 }, SeverityError, "phases[0].rules[1].durationSec"},
		{"rule overruns phase", func(s *domain.Scenario) { s.Phases[0].Rules[1].DurationSec = 500 }, SeverityWarning, "phases[0].rules[1].durationSec"},
		{"partial keeps everything", func(s *domain.Scenario) {
			s.Phases[1].Rules[1].Params = domain.JSONB(`{"keepPercent":100}`)
		}, SeverityError, "phases[1].rules[1].keepPercent"},
		{"conflict with identical content", func(s *domain.Scenario) {
			s.Phases[1].Rules[0].Params = domain.JSONB(`{"sourceA":"A","contentA":"Open.","sourceB":"B","contentB":" open. "}`)
		}, SeverityError, "phases[1].rules[0].conflict.contentB"},
		{"conflict missing a source", func(s *domain.Scenario) {
			s.Phases[1].Rules[0].Params = domain.JSONB(`{"sourceA":"A","contentA":"Open.","contentB":"Shut."}`)
		}, SeverityError, "phases[1].rules[0].conflict.sourceB"},
		{"conflict from one source", func(s *domain.Scenario) {
			s.Phases[1].Rules[0].Params = domain.JSONB(`{"sourceA":"A","contentA":"Open.","sourceB":"a","contentB":"Shut."}`)
		}, SeverityWarning, "phases[1].rules[0].conflict.sourceB"},

		{"decision without question", func(s *domain.Scenario) { s.Phases[1].DecisionPoints[0].Prompt = "" }, SeverityError, "phases[1].decisionPoints[0].question"},
		{"decision with one option", func(s *domain.Scenario) {
			d := &s.Phases[1].DecisionPoints[0]
			d.Options = d.Options[:1]
		}, SeverityError, "phases[1].decisionPoints[0].options"},
		{"decision with duplicate options", func(s *domain.Scenario) {
			s.Phases[1].DecisionPoints[0].Options[1].Label = "coastal ROAD"
		}, SeverityError, "phases[1].decisionPoints[0].options[1].label"},
		{"decision without deadline", func(s *domain.Scenario) { s.Phases[1].DecisionPoints[0].TimeLimitSec = 0 }, SeverityError, "phases[1].decisionPoints[0].deadlineSec"},
		{"deadline after phase", func(s *domain.Scenario) { s.Phases[1].DecisionPoints[0].TimeLimitSec = 400 }, SeverityWarning, "phases[1].decisionPoints[0].deadlineSec"},
		{"decision without criteria", func(s *domain.Scenario) { s.Phases[1].DecisionPoints[0].EvaluationCriteria = "" }, SeverityWarning, "phases[1].decisionPoints[0].evaluationCriteria"},

		{"nothing to decide", func(s *domain.Scenario) { s.Phases[1].DecisionPoints = nil }, SeverityError, "phases"},
		{"nothing to read", func(s *domain.Scenario) { s.Phases[0].Reports = nil }, SeverityError, "phases"},
		{"no degradation", func(s *domain.Scenario) {
			s.Phases[0].Rules, s.Phases[1].Rules = nil, nil
		}, SeverityWarning, "phases"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := completeScenario(t)
			tc.mutate(s)
			issues := ReviewScenario(s)
			for _, i := range issues {
				if i.Path == tc.path && i.Severity == tc.severity {
					return
				}
			}
			t.Fatalf("no %s at %q; got %+v", tc.severity, tc.path, issues)
		})
	}
}

func TestReviewIssuesPointAtTheirEntity(t *testing.T) {
	s := completeScenario(t)
	s.Phases[0].Reports[0].Body = ""
	issues := ReviewScenario(s)
	if len(issues) != 1 {
		t.Fatalf("issues = %+v", issues)
	}
	i := issues[0]
	if i.PhaseID == nil || *i.PhaseID != s.Phases[0].ID || i.EntityID == nil || *i.EntityID != s.Phases[0].Reports[0].ID {
		t.Errorf("issue does not reference its phase and report: %+v", i)
	}
}

func TestNormalizeDerivesTimingOrderAndKeys(t *testing.T) {
	s := completeScenario(t)
	s.ID = uuid.New()
	s.Channels = append(s.Channels, domain.CommunicationChannel{ID: uuid.New(), Name: "Team Net"}, domain.CommunicationChannel{ID: uuid.New()})
	normalize(s)

	if s.EstDurationSec != 720 {
		t.Errorf("EstDurationSec = %d, want 720", s.EstDurationSec)
	}
	if s.Phases[0].Ord != 1 || s.Phases[1].Ord != 2 || s.Phases[1].StartsAtSim != 300 {
		t.Errorf("phase ordering/timing wrong: %+v", s.Phases)
	}
	d := s.Phases[1].DecisionPoints[0]
	if d.PhaseID != s.Phases[1].ID || d.ScenarioID != s.ID || d.Options[1].Ord != 2 || d.Options[1].DecisionPointID != d.ID {
		t.Errorf("decision point parents not set: %+v", d)
	}
	keys := []string{}
	for _, c := range s.Channels {
		keys = append(keys, c.Key)
	}
	if got := strings.Join(keys, ","); got != "team-net,field-feed,team-net-2,channel" {
		t.Errorf("channel keys = %s", got)
	}
}

type scenarioFixture struct {
	svc        *ScenarioService
	repo       *memrepo.ScenarioRepository
	instructor *domain.User
	other      *domain.User
	admin      *domain.User
}

func newScenarioFixture() scenarioFixture {
	repo := memrepo.NewScenarioRepository()
	return scenarioFixture{
		svc:        NewScenarioService(repo),
		repo:       repo,
		instructor: &domain.User{ID: uuid.New(), Role: domain.RoleInstructor},
		other:      &domain.User{ID: uuid.New(), Role: domain.RoleInstructor},
		admin:      &domain.User{ID: uuid.New(), Role: domain.RoleAdmin},
	}
}

func TestScenarioLifecycle(t *testing.T) {
	f := newScenarioFixture()
	ctx := context.Background()

	created, issues, err := f.svc.Create(ctx, f.instructor, "  Exercise Tidewatch ")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "Exercise Tidewatch" || created.Status != domain.ScenarioDraft || created.Revision != 1 {
		t.Fatalf("created = %+v", created)
	}
	if len(created.Channels) != 3 || len(created.Phases) != 1 || !HasErrors(issues) {
		t.Fatalf("a new draft should have default channels, one phase and outstanding issues")
	}

	// A draft cannot be published until it is complete.
	_, _, err = f.svc.Publish(ctx, f.instructor, created.ID, true)
	wantAppErr(t, err, http.StatusUnprocessableEntity, "NOT_READY")

	saved, issues, err := f.svc.Save(ctx, f.instructor, created.ID, 1, completeScenario(t))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if saved.Revision != 2 || saved.EstDurationSec != 720 || len(issues) != 0 {
		t.Fatalf("saved revision=%d duration=%d issues=%+v", saved.Revision, saved.EstDurationSec, issues)
	}

	// A save based on a stale revision is refused rather than overwriting.
	_, _, err = f.svc.Save(ctx, f.instructor, created.ID, 1, completeScenario(t))
	wantAppErr(t, err, http.StatusConflict, "REVISION_CONFLICT")

	reopened, _, err := f.svc.Get(ctx, f.instructor, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(reopened.Phases) != 2 || len(reopened.Phases[1].DecisionPoints[0].Options) != 2 || reopened.OwnerID != f.instructor.ID {
		t.Fatalf("reopened scenario lost content: %+v", reopened)
	}

	_, _, err = f.svc.Publish(ctx, f.instructor, created.ID, false)
	wantAppErr(t, err, http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	published, _, err := f.svc.Publish(ctx, f.instructor, created.ID, true)
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if published.Status != domain.ScenarioPublished || published.PublishedAt == nil {
		t.Fatalf("published = %+v", published)
	}

	// Published scenarios are frozen.
	_, _, err = f.svc.Save(ctx, f.instructor, created.ID, published.Revision, completeScenario(t))
	wantAppErr(t, err, http.StatusConflict, "NOT_DRAFT")
	_, _, err = f.svc.Publish(ctx, f.instructor, created.ID, true)
	wantAppErr(t, err, http.StatusConflict, "NOT_DRAFT")

	// Once a session uses it, it can be neither reverted nor deleted.
	f.repo.Sessions[created.ID] = 1
	_, _, err = f.svc.Unpublish(ctx, f.instructor, created.ID)
	wantAppErr(t, err, http.StatusConflict, "SCENARIO_IN_USE")
	wantAppErr(t, f.svc.Delete(ctx, f.instructor, created.ID), http.StatusConflict, "SCENARIO_IN_USE")
	f.repo.Sessions[created.ID] = 0

	draft, _, err := f.svc.Unpublish(ctx, f.instructor, created.ID)
	if err != nil || draft.Status != domain.ScenarioDraft || draft.PublishedAt != nil {
		t.Fatalf("Unpublish: %+v, %v", draft, err)
	}
	if _, _, err := f.svc.Save(ctx, f.instructor, created.ID, draft.Revision, completeScenario(t)); err != nil {
		t.Fatalf("Save after unpublish: %v", err)
	}

	if err := f.svc.Delete(ctx, f.instructor, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, _, err = f.svc.Get(ctx, f.instructor, created.ID)
	wantAppErr(t, err, http.StatusNotFound, "NOT_FOUND")
}

func TestScenarioOwnership(t *testing.T) {
	f := newScenarioFixture()
	ctx := context.Background()
	mine, _, _ := f.svc.Create(ctx, f.instructor, "Mine")
	theirs, _, _ := f.svc.Create(ctx, f.other, "Theirs")

	// Another instructor's scenario looks like it does not exist.
	_, _, err := f.svc.Get(ctx, f.instructor, theirs.ID)
	wantAppErr(t, err, http.StatusNotFound, "NOT_FOUND")
	_, _, err = f.svc.Save(ctx, f.instructor, theirs.ID, 1, completeScenario(t))
	wantAppErr(t, err, http.StatusNotFound, "NOT_FOUND")
	wantAppErr(t, f.svc.Delete(ctx, f.instructor, theirs.ID), http.StatusNotFound, "NOT_FOUND")

	list, err := f.svc.List(ctx, f.instructor)
	if err != nil || len(list) != 1 || list[0].ID != mine.ID {
		t.Fatalf("instructor list = %+v, %v", list, err)
	}
	// An admin sees and can open everything; saving keeps the original owner.
	if list, _ := f.svc.List(ctx, f.admin); len(list) != 2 {
		t.Fatalf("admin list len = %d, want 2", len(list))
	}
	saved, _, err := f.svc.Save(ctx, f.admin, theirs.ID, 1, completeScenario(t))
	if err != nil || saved.OwnerID != f.other.ID {
		t.Fatalf("admin save: owner=%v err=%v", saved, err)
	}
}

func TestSaveRejectsIncoherentDocuments(t *testing.T) {
	f := newScenarioFixture()
	ctx := context.Background()
	created, _, _ := f.svc.Create(ctx, f.instructor, "S")

	dup := completeScenario(t)
	dup.Phases[1].ID = dup.Phases[0].ID
	_, _, err := f.svc.Save(ctx, f.instructor, created.ID, 1, dup)
	wantAppErr(t, err, http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	dangling := completeScenario(t)
	ghost := uuid.New()
	dangling.Phases[0].Reports[0].ChannelID = &ghost
	_, _, err = f.svc.Save(ctx, f.instructor, created.ID, 1, dangling)
	wantAppErr(t, err, http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	// Rejected saves leave the stored draft and its revision untouched.
	got, _, _ := f.svc.Get(ctx, f.instructor, created.ID)
	if got.Revision != 1 || len(got.Phases) != 1 {
		t.Fatalf("draft was modified by a rejected save: %+v", got)
	}

	// An incomplete draft, by contrast, saves fine and reports what is missing.
	incomplete := completeScenario(t)
	incomplete.Phases[1].DecisionPoints[0].Prompt = ""
	_, issues, err := f.svc.Save(ctx, f.instructor, created.ID, 1, incomplete)
	if err != nil || !HasErrors(issues) {
		t.Fatalf("incomplete draft: issues=%+v err=%v", issues, err)
	}
}
