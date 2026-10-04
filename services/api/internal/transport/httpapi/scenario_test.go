package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

type scenarioEnvelope struct {
	Scenario scenarioResponse `json:"scenario"`
}

type m = map[string]any

// tidewatch is a complete fictional scenario document as a client sends it.
func tidewatch(revision int) m {
	team, feed := uuid.NewString(), uuid.NewString()
	return m{
		"revision": revision,
		"title":    "Exercise Tidewatch", "summary": "Relief routing on the fictional island of Veridia.",
		"difficulty": "INTERMEDIATE", "teamCount": 2, "traineeCount": 8,
		"objectives": []string{"Decide with incomplete information"},
		"channels": []m{
			{"id": team, "name": "Team Net", "kind": "TEAM", "baseLatencySec": 0},
			{"id": feed, "name": "Field Feed", "kind": "FEED", "baseLatencySec": 2},
		},
		"phases": []m{
			{
				"id": uuid.NewString(), "title": "Assessment", "description": "A storm has passed.", "durationSec": 300,
				"reports": []m{{
					"id": uuid.NewString(), "title": "Coastal road", "source": "Relay Station K-7", "content": "Passable.",
					"atSec": 120, "priority": "HIGH", "reliability": "A", "targetTeam": 1, "channelId": feed,
				}},
				"rules": []m{
					{"id": uuid.NewString(), "name": "Feed lag", "mode": "DELAY", "atSec": 150, "durationSec": 60, "channelId": feed, "delaySec": 30},
					{"id": uuid.NewString(), "name": "Net down", "mode": "DROPOUT", "atSec": 180, "durationSec": 40, "channelId": team, "targetTeam": 2},
				},
				"decisionPoints": []m{},
			},
			{
				"id": uuid.NewString(), "title": "Commitment", "description": "", "durationSec": 420,
				"reports": []m{},
				"rules": []m{{
					"id": uuid.NewString(), "name": "Pass status", "mode": "CONFLICTING", "atSec": 20, "durationSec": 0, "channelId": feed,
					"conflict": m{"topic": "Inland pass", "sourceA": "Relay Station K-7", "contentA": "Open.", "sourceB": "Community Net", "contentB": "Blocked."},
				}},
				"decisionPoints": []m{{
					"id": uuid.NewString(), "question": "Which route does the convoy take?", "scope": "TEAM",
					"atSec": 120, "deadlineSec": 180, "rationaleRequired": true, "evaluationCriteria": "Sought confirmation first.",
					"options": []m{
						{"id": uuid.NewString(), "label": "Coastal road", "description": ""},
						{"id": uuid.NewString(), "label": "Inland pass", "description": ""},
					},
				}},
			},
		},
	}
}

func wantScenario(t *testing.T, rec *httptest.ResponseRecorder, status int) scenarioResponse {
	t.Helper()
	wantStatus(t, rec, status)
	return decode[scenarioEnvelope](t, rec).Scenario
}

// The acceptance path: create, build, save, reopen, edit, publish.
func TestScenarioBuilderFlow(t *testing.T) {
	api := newTestAPI(t)
	instructor := withBearer(api.tokenFor(domain.RoleInstructor))

	created := wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios", m{"title": "Exercise Tidewatch"}, instructor), http.StatusCreated)
	if created.Status != "DRAFT" || created.Revision != 1 || len(created.Phases) != 1 || len(created.Channels) != 3 || len(created.Issues) == 0 {
		t.Fatalf("created = %+v", created)
	}
	url := "/api/v1/scenarios/" + created.ID

	// Publishing an unfinished draft lists what is missing.
	e := wantErrorCode(t, api.do(http.MethodPost, url+"/publish", m{"confirmFictional": true}, instructor), http.StatusUnprocessableEntity, "NOT_READY")
	if len(e.Details) == 0 {
		t.Error("NOT_READY carries no details")
	}

	doc := tidewatch(1)
	saved := wantScenario(t, api.do(http.MethodPut, url, doc, instructor), http.StatusOK)
	if saved.Revision != 2 || saved.EstDurationSec != 720 || len(saved.Issues) != 0 {
		t.Fatalf("saved: revision=%d duration=%d issues=%+v", saved.Revision, saved.EstDurationSec, saved.Issues)
	}

	reopened := wantScenario(t, api.do(http.MethodGet, url, nil, instructor), http.StatusOK)
	if len(reopened.Phases) != 2 {
		t.Fatalf("reopened phases = %d", len(reopened.Phases))
	}
	report := reopened.Phases[0].Reports[0]
	if report.Source != "Relay Station K-7" || report.AtSec != 120 || report.Priority != "HIGH" ||
		report.TargetTeam == nil || *report.TargetTeam != 1 || report.ChannelID == nil {
		t.Errorf("report did not round-trip: %+v", report)
	}
	delay := reopened.Phases[0].Rules[0]
	if delay.Mode != "DELAY" || delay.DelaySec != 30 || delay.DurationSec != 60 || delay.Conflict != nil {
		t.Errorf("delay rule did not round-trip: %+v", delay)
	}
	conflict := reopened.Phases[1].Rules[0]
	if conflict.Conflict == nil || conflict.Conflict.SourceB != "Community Net" || conflict.Conflict.ContentB != "Blocked." {
		t.Errorf("conflict rule did not round-trip: %+v", conflict)
	}
	point := reopened.Phases[1].DecisionPoints[0]
	if point.Question == "" || point.DeadlineSec != 180 || !point.RationaleRequired || len(point.Options) != 2 || point.Options[1].Label != "Inland pass" {
		t.Errorf("decision point did not round-trip: %+v", point)
	}

	// Edit: rename a phase and blank the question. It saves, with an issue.
	doc["revision"] = 2
	phases := doc["phases"].([]m)
	phases[0]["title"] = "Initial assessment"
	phases[1]["decisionPoints"].([]m)[0]["question"] = ""
	edited := wantScenario(t, api.do(http.MethodPut, url, doc, instructor), http.StatusOK)
	if edited.Revision != 3 || edited.Phases[0].Title != "Initial assessment" {
		t.Fatalf("edited = %+v", edited)
	}
	if len(edited.Issues) != 1 || edited.Issues[0].Path != "phases[1].decisionPoints[0].question" || edited.Issues[0].EntityID == nil {
		t.Fatalf("issues = %+v", edited.Issues)
	}
	wantErrorCode(t, api.do(http.MethodPost, url+"/publish", m{"confirmFictional": true}, instructor), http.StatusUnprocessableEntity, "NOT_READY")

	// A second tab still on revision 2 cannot overwrite revision 3.
	doc["revision"] = 2
	wantErrorCode(t, api.do(http.MethodPut, url, doc, instructor), http.StatusConflict, "REVISION_CONFLICT")

	doc["revision"] = 3
	phases[1]["decisionPoints"].([]m)[0]["question"] = "Which route does the convoy take?"
	wantScenario(t, api.do(http.MethodPut, url, doc, instructor), http.StatusOK)

	wantErrorCode(t, api.do(http.MethodPost, url+"/publish", m{}, instructor), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	published := wantScenario(t, api.do(http.MethodPost, url+"/publish", m{"confirmFictional": true}, instructor), http.StatusOK)
	if published.Status != "PUBLISHED" || published.PublishedAt == nil {
		t.Fatalf("published = %+v", published)
	}

	doc["revision"] = published.Revision
	wantErrorCode(t, api.do(http.MethodPut, url, doc, instructor), http.StatusConflict, "NOT_DRAFT")

	list := decode[struct{ Scenarios []scenarioSummary }](t, api.do(http.MethodGet, "/api/v1/scenarios", nil, instructor))
	if len(list.Scenarios) != 1 || list.Scenarios[0].Status != "PUBLISHED" || list.Scenarios[0].PhaseCount != 2 || list.Scenarios[0].EstDurationSec != 720 {
		t.Fatalf("list = %+v", list.Scenarios)
	}

	reverted := wantScenario(t, api.do(http.MethodPost, url+"/unpublish", nil, instructor), http.StatusOK)
	if reverted.Status != "DRAFT" {
		t.Fatalf("reverted = %+v", reverted)
	}
	wantStatus(t, api.do(http.MethodDelete, url, nil, instructor), http.StatusNoContent)
	wantErrorCode(t, api.do(http.MethodGet, url, nil, instructor), http.StatusNotFound, "NOT_FOUND")
}

func TestScenarioAccessControl(t *testing.T) {
	api := newTestAPI(t)
	trainee := withBearer(api.tokenFor(domain.RoleTrainee))
	instructor := withBearer(api.tokenFor(domain.RoleInstructor))
	admin := withBearer(api.tokenFor(domain.RoleAdmin))

	created := wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios", m{"title": "S"}, instructor), http.StatusCreated)
	url := "/api/v1/scenarios/" + created.ID

	for _, rt := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/scenarios", nil},
		{http.MethodPost, "/api/v1/scenarios", m{"title": "X"}},
		{http.MethodGet, url, nil},
		{http.MethodPut, url, tidewatch(1)},
		{http.MethodDelete, url, nil},
		{http.MethodPost, url + "/publish", m{"confirmFictional": true}},
		{http.MethodPost, url + "/unpublish", nil},
	} {
		wantStatus(t, api.do(rt.method, rt.path, rt.body), http.StatusUnauthorized)
		wantStatus(t, api.do(rt.method, rt.path, rt.body, trainee), http.StatusForbidden)
	}

	// A second instructor cannot see or touch the first one's scenario.
	if _, err := api.users.Create(t.Context(), "second@example.test", "password-123", "Second", domain.RoleInstructor); err != nil {
		t.Fatal(err)
	}
	res, err := api.auth.Login(t.Context(), "second@example.test", "password-123")
	if err != nil {
		t.Fatal(err)
	}
	second := withBearer(res.AccessToken)
	wantErrorCode(t, api.do(http.MethodGet, url, nil, second), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodPut, url, tidewatch(1), second), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodDelete, url, nil, second), http.StatusNotFound, "NOT_FOUND")
	if list := decode[struct{ Scenarios []scenarioSummary }](t, api.do(http.MethodGet, "/api/v1/scenarios", nil, second)); len(list.Scenarios) != 0 {
		t.Errorf("second instructor sees %d scenarios", len(list.Scenarios))
	}

	wantScenario(t, api.do(http.MethodGet, url, nil, admin), http.StatusOK)
	wantErrorCode(t, api.do(http.MethodGet, "/api/v1/scenarios/not-a-uuid", nil, admin), http.StatusNotFound, "NOT_FOUND")
}

func TestScenarioRequestValidation(t *testing.T) {
	api := newTestAPI(t)
	instructor := withBearer(api.tokenFor(domain.RoleInstructor))
	created := wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios", m{"title": "S"}, instructor), http.StatusCreated)
	url := "/api/v1/scenarios/" + created.ID

	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/scenarios", m{"title": ""}, instructor), http.StatusUnprocessableEntity, "VALIDATION_FAILED")

	cases := map[string]struct {
		mutate func(m)
		path   string
	}{
		"missing revision":   {func(d m) { delete(d, "revision") }, "revision"},
		"unknown difficulty": {func(d m) { d["difficulty"] = "IMPOSSIBLE" }, "difficulty"},
		"too many teams":     {func(d m) { d["teamCount"] = 9 }, "teamCount"},
		"bad channel id":     {func(d m) { d["channels"].([]m)[0]["id"] = "nope" }, "channels[0].id"},
		"negative duration":  {func(d m) { d["phases"].([]m)[1]["durationSec"] = -5 }, "phases[1].durationSec"},
		"unknown priority":   {func(d m) { d["phases"].([]m)[0]["reports"].([]m)[0]["priority"] = "FLASH" }, "phases[0].reports[0].priority"},
		"unknown mode":       {func(d m) { d["phases"].([]m)[0]["rules"].([]m)[1]["mode"] = "JAM" }, "phases[0].rules[1].mode"},
		"team out of range":  {func(d m) { d["phases"].([]m)[0]["reports"].([]m)[0]["targetTeam"] = 0 }, "phases[0].reports[0].targetTeam"},
		"option without id": {func(d m) {
			delete(d["phases"].([]m)[1]["decisionPoints"].([]m)[0]["options"].([]m)[1], "id")
		}, "phases[1].decisionPoints[0].options[1].id"},
		"dangling channel": {func(d m) {
			d["phases"].([]m)[0]["reports"].([]m)[0]["channelId"] = uuid.NewString()
		}, "phases[0].reports[0].channelId"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			doc := tidewatch(1)
			tc.mutate(doc)
			e := wantErrorCode(t, api.do(http.MethodPut, url, doc, instructor), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
			if e.Details[tc.path] == "" {
				t.Errorf("no detail at %q: %v", tc.path, e.Details)
			}
		})
	}

	// None of the rejected saves changed the draft.
	if got := wantScenario(t, api.do(http.MethodGet, url, nil, instructor), http.StatusOK); got.Revision != 1 {
		t.Errorf("revision = %d after rejected saves, want 1", got.Revision)
	}
}
