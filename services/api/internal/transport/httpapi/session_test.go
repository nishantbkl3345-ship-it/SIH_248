package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

type sessionEnvelope struct {
	Session sessionResponse `json:"session"`
}

type frame struct {
	Type    string          `json:"type"`
	SimMs   int64           `json:"simMs"`
	Payload json.RawMessage `json:"payload"`
}

// stream is a WebSocket client that reads frames one at a time. Reads block
// until the server sends something; the timeout only bounds a failing test.
type stream struct {
	t    *testing.T
	conn *websocket.Conn
	seen []frame
}

func dial(t *testing.T, srv *httptest.Server, sessionID, token string) *stream {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/sessions/" + sessionID + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + token}}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	s := &stream{t: t, conn: conn}
	if first := s.read(); first.Type != "session.snapshot" {
		t.Fatalf("first frame = %s, want session.snapshot", first.Type)
	}
	return s
}

func (s *stream) read() frame {
	s.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, raw, err := s.conn.Read(ctx)
	if err != nil {
		s.t.Fatalf("read: %v (frames so far: %s)", err, s.types())
	}
	var f frame
	if err := json.Unmarshal(raw, &f); err != nil {
		s.t.Fatalf("frame %s: %v", raw, err)
	}
	s.seen = append(s.seen, f)
	return f
}

// until reads frames up to and including the first of the given type.
func (s *stream) until(typ string) frame {
	s.t.Helper()
	for {
		if f := s.read(); f.Type == typ {
			return f
		}
	}
}

func (s *stream) types() string {
	var out []string
	for _, f := range s.seen {
		if f.Type != "simulation.time_update" {
			out = append(out, f.Type)
		}
	}
	return strings.Join(out, ",")
}

func (s *stream) everything() string {
	raw, _ := json.Marshal(s.seen)
	return string(raw)
}

// exercise is a published scenario with a lobby of two trainees, one per team.
type exercise struct {
	api        *testAPI
	srv        *httptest.Server
	id         string
	url        string
	doc        m
	instructor string
	alice      string // team 1
	bela       string // team 2
}

func newExercise(t *testing.T) *exercise {
	t.Helper()
	api := newTestAPI(t)
	x := &exercise{
		api:        api,
		instructor: api.tokenFor(domain.RoleInstructor),
		alice:      api.login("alice@example.test", "Alice", domain.RoleTrainee),
		bela:       api.login("bela@example.test", "Bela", domain.RoleTrainee),
	}
	inst := withBearer(x.instructor)

	created := wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios", m{"title": "Exercise Tidewatch"}, inst), http.StatusCreated)
	x.doc = tidewatch(1)
	wantScenario(t, api.do(http.MethodPut, "/api/v1/scenarios/"+created.ID, x.doc, inst), http.StatusOK)

	// A draft cannot be run.
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/sessions", m{"scenarioId": created.ID}, inst), http.StatusConflict, "SCENARIO_NOT_PUBLISHED")
	wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios/"+created.ID+"/publish", m{"confirmFictional": true}, inst), http.StatusOK)

	rec := api.do(http.MethodPost, "/api/v1/sessions", m{"scenarioId": created.ID, "name": "Morning run", "seed": 42}, inst)
	wantStatus(t, rec, http.StatusCreated)
	sess := decode[sessionEnvelope](t, rec).Session
	if sess.Status != "LOBBY" || sess.JoinCode == nil || len(*sess.JoinCode) != 6 || *sess.Seed != 42 || len(sess.Teams) != 2 {
		t.Fatalf("created session = %+v", sess)
	}
	x.id, x.url = sess.ID, "/api/v1/sessions/"+sess.ID

	// Codes are forgiving about case and spacing.
	code := strings.ToLower((*sess.JoinCode)[:3]) + " " + (*sess.JoinCode)[3:]
	for i, token := range []string{x.alice, x.bela} {
		rec := api.do(http.MethodPost, "/api/v1/sessions/join", m{"joinCode": code}, withBearer(token))
		wantStatus(t, rec, http.StatusOK)
		joined := decode[sessionEnvelope](t, rec).Session
		if joined.MyTeam == nil || *joined.MyTeam != i+1 || joined.JoinCode != nil || joined.Seed != nil {
			t.Fatalf("trainee %d joined as %+v", i, joined)
		}
	}

	x.srv = httptest.NewServer(api.router)
	t.Cleanup(x.srv.Close)
	return x
}

// tick moves the wall clock and lets the runtime catch up.
func (x *exercise) tick(d time.Duration) {
	x.api.clock.advance(d)
	x.api.live.Tick(context.Background())
}

func (x *exercise) ids() (feed, team string, point m) {
	channels := x.doc["channels"].([]m)
	point = x.doc["phases"].([]m)[1]["decisionPoints"].([]m)[0]
	return channels[1]["id"].(string), channels[0]["id"].(string), point
}

// The scenario (see tidewatch): feed has 2 s latency.
//
//	T+120  report "Coastal road" for team 1
//	T+150  feed delayed 30 s (until T+210)
//	T+180  team 2 loses the team net (until T+220)
//	T+320  two conflicting reports for everyone
//	T+420  team decision opens, closes T+600
//	T+720  end
func TestExerciseOverHTTPAndWebSocket(t *testing.T) {
	x := newExercise(t)
	api := x.api
	inst, alice, bela := withBearer(x.instructor), withBearer(x.alice), withBearer(x.bela)
	feed, teamNet, point := x.ids()
	options := point["options"].([]m)

	aliceWS := dial(t, x.srv, x.id, x.alice)
	belaWS := dial(t, x.srv, x.id, x.bela)
	instWS := dial(t, x.srv, x.id, x.instructor)

	// Nothing can be sent before the exercise starts.
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/messages", m{"channelId": teamNet, "body": "hello"}, alice), http.StatusConflict, "SESSION_NOT_LIVE")

	rec := api.do(http.MethodPost, x.url+"/start", nil, inst)
	wantStatus(t, rec, http.StatusOK)
	if s := decode[sessionEnvelope](t, rec).Session; s.Status != "RUNNING" || s.StartedAt == nil {
		t.Fatalf("after start: %+v", s)
	}
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/start", nil, inst), http.StatusConflict, "ALREADY_STARTED")
	aliceWS.until("phase.changed")
	belaWS.until("phase.changed")

	// --- T+122: the first report reaches team 1 only.
	x.tick(122 * time.Second)
	got := aliceWS.until("report.received")
	var report struct {
		Title, Source, Content string
		SentAtMs, ReceivedAtMs int64
	}
	if err := json.Unmarshal(got.Payload, &report); err != nil {
		t.Fatal(err)
	}
	if report.Title != "Coastal road" || report.Source != "Relay Station K-7" || report.SentAtMs != 120_000 || report.ReceivedAtMs != 122_000 {
		t.Errorf("alice's report = %+v", report)
	}

	// --- T+200: team 2's net is down. Bela's message is accepted and lost.
	x.tick(78 * time.Second)
	rec = api.do(http.MethodPost, x.url+"/messages", m{"channelId": teamNet, "body": "Team 2 checking in"}, bela)
	wantStatus(t, rec, http.StatusCreated)
	if body := rec.Body.String(); strings.Contains(body, "DROPPED") || strings.Contains(body, "state") {
		t.Errorf("the sender was told what became of the message: %s", body)
	}
	// Alice's goes through to her own team.
	wantStatus(t, api.do(http.MethodPost, x.url+"/messages", m{"channelId": teamNet, "body": "Team 1 here"}, alice), http.StatusCreated)
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/messages", m{"channelId": feed, "body": "x"}, alice), http.StatusConflict, "CHANNEL_NOT_WRITABLE")

	// --- Pause and resume: instructor only, and time stands still.
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/pause", nil, alice), http.StatusForbidden, "FORBIDDEN")
	wantStatus(t, api.do(http.MethodPost, x.url+"/pause", nil, inst), http.StatusOK)
	aliceWS.until("simulation.paused")
	x.tick(time.Hour)
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/messages", m{"channelId": teamNet, "body": "x"}, alice), http.StatusConflict, "EXERCISE_PAUSED")
	rec = api.do(http.MethodPost, x.url+"/resume", nil, inst)
	wantStatus(t, rec, http.StatusOK)
	aliceWS.until("simulation.resumed")

	// --- Speed: 1 real second = 5 simulated. 24 s takes T+200 to T+320.
	wantErrorCode(t, api.do(http.MethodPatch, x.url+"/speed", m{"speed": 500}, inst), http.StatusUnprocessableEntity, "SPEED_OUT_OF_RANGE")
	wantErrorCode(t, api.do(http.MethodPatch, x.url+"/speed", m{"speed": 5}, bela), http.StatusForbidden, "FORBIDDEN")
	rec = api.do(http.MethodPatch, x.url+"/speed", m{"speed": 5}, inst)
	wantStatus(t, rec, http.StatusOK)
	if s := decode[sessionEnvelope](t, rec).Session; s.Speed != 5 {
		t.Errorf("speed = %v, want 5", s.Speed)
	}
	x.tick(24*time.Second + 400*time.Millisecond) // T+322: conflicting reports land (2 s latency)

	// Both teams get two ordinary reports from two sources.
	for _, ws := range []*stream{aliceWS, belaWS} {
		first, second := ws.until("report.received"), ws.until("report.received")
		pair := string(first.Payload) + string(second.Payload)
		if !strings.Contains(pair, "Relay Station K-7") || !strings.Contains(pair, "Community Net") ||
			!strings.Contains(pair, "Open.") || !strings.Contains(pair, "Blocked.") {
			t.Errorf("conflicting reports as received: %s", pair)
		}
	}

	// --- T+420: the decision point opens for everyone.
	x.tick(20 * time.Second) // 100 simulated seconds: now T+422
	opened := aliceWS.until("decision.opened")
	belaWS.until("decision.opened")
	if strings.Contains(strings.ToLower(string(opened.Payload)), "criteria") || strings.Contains(string(opened.Payload), "Sought confirmation") {
		t.Errorf("evaluation criteria were sent to a trainee: %s", opened.Payload)
	}

	// --- T+472: Bela decides, 52 simulated seconds (10.4 real ones) after it opened. The request tries to dictate time, team and
	// what she knew; none of it is used.
	x.tick(10 * time.Second)
	rec = api.do(http.MethodPost, x.url+"/decisions", m{
		"decisionPointId": point["id"], "optionId": options[1]["id"], "rationale": "Two sources disagree; holding to the confirmed road.",
		"confidence": 3,
		"simMs":      1, "wallTime": "1999-01-01T00:00:00Z", "team": 1, "responseMs": 1, "participantId": uuid.NewString(),
		"snapshot": m{"reports": []m{}},
	}, bela)
	wantStatus(t, rec, http.StatusCreated)
	var receipt struct {
		Decision struct {
			SimMs, ResponseMs int64
			WallTime          time.Time
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Decision.SimMs != 472_000 || receipt.Decision.ResponseMs != 52_000 || !receipt.Decision.WallTime.Equal(api.clock.Now()) {
		t.Errorf("decision times came from somewhere other than the server clock: %+v", receipt.Decision)
	}
	if strings.Contains(rec.Body.String(), "snapshot") || strings.Contains(rec.Body.String(), "Degradation") {
		t.Errorf("the receipt carries ground truth: %s", rec.Body.String())
	}
	belaWS.until("decision.submitted")

	wantErrorCode(t, api.do(http.MethodPost, x.url+"/decisions", m{"decisionPointId": point["id"], "optionId": options[0]["id"], "rationale": "again"}, bela), http.StatusConflict, "ALREADY_DECIDED")
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/decisions", m{"decisionPointId": point["id"], "optionId": options[0]["id"], "rationale": " "}, alice), http.StatusUnprocessableEntity, "RATIONALE_REQUIRED")
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/decisions", m{"decisionPointId": point["id"], "optionId": uuid.NewString(), "rationale": "r"}, alice), http.StatusUnprocessableEntity, "UNKNOWN_OPTION")
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/decisions", m{"decisionPointId": point["id"], "optionId": options[0]["id"], "rationale": "r"}, inst), http.StatusForbidden, "FORBIDDEN")

	// --- T+600: deadline. Alice is too late.
	x.tick(26 * time.Second)
	aliceWS.until("decision.closed")
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/decisions", m{"decisionPointId": point["id"], "optionId": options[0]["id"], "rationale": "late"}, alice), http.StatusConflict, "DECISION_NOT_OPEN")

	// --- T+720: the end.
	x.tick(24 * time.Second)
	aliceWS.until("simulation.completed")
	belaWS.until("simulation.completed")
	instWS.until("simulation.completed")
	rec = api.do(http.MethodGet, x.url, nil, inst)
	if s := decode[sessionEnvelope](t, rec).Session; s.Status != "ENDED" || s.EndedAt == nil || s.SimMs != 720_000 {
		t.Errorf("after the end: %+v", s)
	}
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/pause", nil, inst), http.StatusConflict, "NOT_RUNNING")

	// ------------------------------------------------ what each side saw

	if got, want := aliceWS.types(), "session.snapshot,simulation.started,phase.changed,report.received,"+
		"simulation.paused,simulation.resumed,phase.changed,report.received,report.received,"+
		"decision.opened,decision.closed,simulation.completed"; got != want {
		t.Errorf("alice's stream:\n got %s\nwant %s", got, want)
	}
	// Bela: no first report (team 1 only), no message from Alice (team net),
	// and her own decision.
	if got, want := belaWS.types(), "session.snapshot,simulation.started,phase.changed,"+
		"simulation.paused,simulation.resumed,phase.changed,report.received,report.received,"+
		"decision.opened,decision.submitted,decision.closed,simulation.completed"; got != want {
		t.Errorf("bela's stream:\n got %s\nwant %s", got, want)
	}

	for name, ws := range map[string]*stream{"alice": aliceWS, "bela": belaWS} {
		all := ws.everything()
		for _, leak := range []string{"DROPPED", "DELAYED", "CONTRADICTORY", "conflictGroup", "appliedRules", "communication.", "report.delayed", "report.dropped", "report.generated", "message.dropped", "activeDegradation", "Sought confirmation"} {
			if strings.Contains(all, leak) {
				t.Errorf("%s's stream leaks %q", name, leak)
			}
		}
	}
	// "Passable." is the body of the report addressed to team 1 only.
	if !strings.Contains(aliceWS.everything(), "Passable.") {
		t.Error("team 1 did not receive its own report")
	}
	if strings.Contains(belaWS.everything(), "Passable.") || strings.Contains(belaWS.everything(), `"sentAtMs":120000`) {
		t.Error("team 2 received team 1's report")
	}
	if strings.Contains(aliceWS.everything(), "Team 2 checking in") || strings.Contains(aliceWS.everything(), "holding to the confirmed road") {
		t.Error("team 1 received team 2's message or decision")
	}

	// The instructor's stream has the ground truth.
	instAll := instWS.types()
	for _, want := range []string{"report.generated", "report.received", "communication.degraded", "communication.restored", "message.sent", "message.dropped", "message.received", "decision.opened", "decision.submitted", "simulation.speed_changed", "phase.completed"} {
		if !strings.Contains(instAll, want) {
			t.Errorf("instructor stream has no %s", want)
		}
	}
	if !strings.Contains(instWS.everything(), "CONTRADICTORY") || !strings.Contains(instWS.everything(), "activeDegradation") {
		t.Error("instructor stream lacks delivery states or the decision snapshot")
	}

	// ---------------------------------------------------- the stored record

	wantErrorCode(t, api.do(http.MethodGet, x.url+"/timeline", nil, alice), http.StatusForbidden, "FORBIDDEN")
	rec = api.do(http.MethodGet, x.url+"/timeline", nil, inst)
	wantStatus(t, rec, http.StatusOK)
	timeline := decode[struct{ Events []timelineEventDTO }](t, rec).Events
	for i, e := range timeline {
		if e.Seq != int64(i+1) {
			t.Fatalf("timeline seq %d at position %d", e.Seq, i)
		}
		if i > 0 && e.SimMs < timeline[i-1].SimMs {
			t.Fatalf("timeline goes backwards at seq %d", e.Seq)
		}
	}
	count := map[string]int{}
	for _, e := range timeline {
		count[e.Type]++
	}
	for typ, want := range map[string]int{
		"EXERCISE_STARTED": 1, "PHASE_STARTED": 2, "PHASE_COMPLETED": 2, "REPORT_GENERATED": 3, "REPORT_DELIVERED": 5,
		"COMMUNICATION_DEGRADED": 2, "COMMUNICATION_RESTORED": 2, "MESSAGE_SENT": 2, "MESSAGE_DROPPED": 1, "MESSAGE_DELIVERED": 1,
		"EXERCISE_PAUSED": 1, "EXERCISE_RESUMED": 1, "SPEED_CHANGED": 1, "DECISION_POINT_OPENED": 1, "DECISION_SUBMITTED": 1,
		"DECISION_POINT_CLOSED": 1, "EXERCISE_ENDED": 1,
	} {
		if count[typ] != want {
			t.Errorf("timeline has %d %s, want %d", count[typ], typ, want)
		}
	}
	paged := decode[struct{ Events []timelineEventDTO }](t, api.do(http.MethodGet, x.url+"/timeline?afterSeq=5&limit=3", nil, inst)).Events
	if len(paged) != 3 || paged[0].Seq != 6 {
		t.Errorf("paging: %+v", paged)
	}

	if len(api.sessions.Decisions) != 1 || len(api.sessions.Messages) != 2 {
		t.Fatalf("stored %d decisions and %d messages, want 1 and 2", len(api.sessions.Decisions), len(api.sessions.Messages))
	}
	d := api.sessions.Decisions[0]
	var snap struct {
		Reports           []struct{ State string }
		ActiveDegradation []any
	}
	if err := json.Unmarshal(d.Snapshot, &snap); err != nil {
		t.Fatal(err)
	}
	if d.TeamNo != 2 || d.SimMs != 472_000 || d.ResponseMs != 52_000 || d.ResponseWallMs != 10_400 || *d.Confidence != 3 || len(snap.Reports) != 2 || snap.Reports[0].State != "CONTRADICTORY" {
		t.Errorf("stored decision = %+v snapshot = %+v", d, snap)
	}
}

func TestStateEndpointIsRoleFiltered(t *testing.T) {
	x := newExercise(t)
	api := x.api
	inst, alice, bela := withBearer(x.instructor), withBearer(x.alice), withBearer(x.bela)

	// In the lobby there is nothing to see yet.
	rec := api.do(http.MethodGet, x.url+"/state", nil, alice)
	wantStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Body.String(), `"live":false`) {
		t.Errorf("lobby state: %s", rec.Body.String())
	}

	wantStatus(t, api.do(http.MethodPost, x.url+"/start", nil, inst), http.StatusOK)
	x.api.clock.advance(125 * time.Second) // no Tick: the request itself must catch up

	a := api.do(http.MethodGet, x.url+"/state", nil, alice).Body.String()
	b := api.do(http.MethodGet, x.url+"/state", nil, bela).Body.String()
	i := api.do(http.MethodGet, x.url+"/state", nil, inst).Body.String()

	if !strings.Contains(a, "Passable.") || strings.Contains(b, "Passable.") {
		t.Error("the report for team 1 is not visible to exactly team 1")
	}
	for name, body := range map[string]string{"alice": a, "bela": b} {
		if strings.Contains(body, `"instructor"`) || strings.Contains(body, "joinCode") || strings.Contains(body, "seed") || strings.Contains(body, "activeRules") {
			t.Errorf("%s's state includes instructor-only data: %s", name, body)
		}
	}
	// A trainee sees their own team's roster, not the other team's.
	if !strings.Contains(a, "Alice") || strings.Contains(a, "Bela") {
		t.Errorf("alice's roster: %s", a)
	}
	if !strings.Contains(i, `"instructor"`) || !strings.Contains(i, "joinCode") || !strings.Contains(i, "Bela") || strings.Contains(i, `"trainee"`) {
		t.Errorf("instructor state: %s", i)
	}
}

func TestSessionAccessControl(t *testing.T) {
	x := newExercise(t)
	api := x.api
	outsider := withBearer(api.login("carol@example.test", "Carol", domain.RoleTrainee))
	otherInstructor := withBearer(api.login("other@example.test", "Other", domain.RoleInstructor))
	admin := withBearer(api.tokenFor(domain.RoleAdmin))

	for _, path := range []string{"", "/state", "/timeline"} {
		wantStatus(t, api.do(http.MethodGet, x.url+path, nil), http.StatusUnauthorized)
	}
	// Someone who has not joined, and an instructor who does not run it,
	// cannot tell the session exists.
	wantErrorCode(t, api.do(http.MethodGet, x.url, nil, outsider), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodGet, x.url+"/state", nil, outsider), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodGet, x.url+"/state", nil, otherInstructor), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodPost, x.url+"/start", nil, otherInstructor), http.StatusNotFound, "NOT_FOUND")
	wantStatus(t, api.do(http.MethodGet, x.url, nil, admin), http.StatusOK)

	// Role gates.
	wantStatus(t, api.do(http.MethodPost, "/api/v1/sessions", m{"scenarioId": uuid.NewString()}, withBearer(x.alice)), http.StatusForbidden)
	wantStatus(t, api.do(http.MethodPost, "/api/v1/sessions/join", m{"joinCode": "ABCDEF"}, withBearer(x.instructor)), http.StatusForbidden)
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/sessions/join", m{"joinCode": "ZZZZZZ"}, outsider), http.StatusNotFound, "NOT_FOUND")
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/sessions", m{"scenarioId": uuid.NewString()}, otherInstructor), http.StatusNotFound, "NOT_FOUND")

	// Lists are scoped to the caller.
	list := func(opt reqOpt) int {
		return len(decode[struct{ Sessions []sessionResponse }](t, api.do(http.MethodGet, "/api/v1/sessions", nil, opt)).Sessions)
	}
	if list(withBearer(x.instructor)) != 1 || list(withBearer(x.alice)) != 1 || list(outsider) != 0 || list(otherInstructor) != 0 || list(admin) != 1 {
		t.Error("session lists are not scoped to the caller")
	}

	// The WebSocket is refused before the upgrade for the same people.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(x.srv.URL, "http") + x.url + "/ws"
	for name, hdr := range map[string]http.Header{
		"anonymous": {},
		"outsider":  {"Authorization": {"Bearer " + api.login("dave@example.test", "Dave", domain.RoleTrainee)}},
	} {
		_, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: hdr})
		if err == nil {
			t.Fatalf("%s opened a stream", name)
		}
		want := map[string]int{"anonymous": http.StatusUnauthorized, "outsider": http.StatusNotFound}[name]
		if resp == nil || resp.StatusCode != want {
			t.Errorf("%s: handshake status = %v, want %d", name, resp, want)
		}
	}
	// A page on another origin cannot open a stream using the user's credentials.
	_, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{
		"Authorization": {"Bearer " + x.alice}, "Origin": {"https://evil.example"},
	}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin handshake: err=%v resp=%v, want 403", err, resp)
	}
}

func TestLobby(t *testing.T) {
	x := newExercise(t)
	api := x.api
	inst := withBearer(x.instructor)

	// Joining twice is harmless.
	sess := decode[sessionEnvelope](t, api.do(http.MethodGet, x.url, nil, inst)).Session
	rec := api.do(http.MethodPost, "/api/v1/sessions/join", m{"joinCode": *sess.JoinCode}, withBearer(x.alice))
	wantStatus(t, rec, http.StatusOK)
	sess = decode[sessionEnvelope](t, api.do(http.MethodGet, x.url, nil, inst)).Session
	if len(sess.Participants) != 2 {
		t.Fatalf("%d participants after a repeat join, want 2", len(sess.Participants))
	}

	// The instructor moves Bela to team 1.
	var bela participantDTO
	for _, p := range sess.Participants {
		if p.DisplayName == "Bela" {
			bela = p
		}
	}
	wantErrorCode(t, api.do(http.MethodPatch, x.url+"/participants/"+bela.ID, m{"team": 1}, withBearer(x.bela)), http.StatusForbidden, "FORBIDDEN")
	wantErrorCode(t, api.do(http.MethodPatch, x.url+"/participants/"+bela.ID, m{"team": 3}, inst), http.StatusUnprocessableEntity, "VALIDATION_FAILED")
	wantErrorCode(t, api.do(http.MethodPatch, x.url+"/participants/"+uuid.NewString(), m{"team": 1}, inst), http.StatusNotFound, "NOT_FOUND")
	rec = api.do(http.MethodPatch, x.url+"/participants/"+bela.ID, m{"team": 1}, inst)
	wantStatus(t, rec, http.StatusOK)
	for _, p := range decode[sessionEnvelope](t, rec).Session.Participants {
		if p.Team != 1 {
			t.Errorf("%s is on team %d", p.DisplayName, p.Team)
		}
	}

	// A third trainee is balanced onto the empty team.
	carol := api.login("carol@example.test", "Carol", domain.RoleTrainee)
	rec = api.do(http.MethodPost, "/api/v1/sessions/join", m{"joinCode": *sess.JoinCode}, withBearer(carol))
	if got := decode[sessionEnvelope](t, rec).Session; *got.MyTeam != 2 {
		t.Errorf("carol was put on team %d, want the empty team 2", *got.MyTeam)
	}

	// Once it starts, the lobby is closed and teams are fixed.
	wantStatus(t, api.do(http.MethodPost, x.url+"/start", nil, inst), http.StatusOK)
	late := api.login("late@example.test", "Late", domain.RoleTrainee)
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/sessions/join", m{"joinCode": *sess.JoinCode}, withBearer(late)), http.StatusConflict, "SESSION_NOT_JOINABLE")
	wantErrorCode(t, api.do(http.MethodPatch, x.url+"/participants/"+bela.ID, m{"team": 2}, inst), http.StatusConflict, "SESSION_STARTED")
}

func TestStartNeedsAtLeastOneTrainee(t *testing.T) {
	api := newTestAPI(t)
	inst := withBearer(api.tokenFor(domain.RoleInstructor))
	created := wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios", m{"title": "S"}, inst), http.StatusCreated)
	wantScenario(t, api.do(http.MethodPut, "/api/v1/scenarios/"+created.ID, tidewatch(1), inst), http.StatusOK)
	wantScenario(t, api.do(http.MethodPost, "/api/v1/scenarios/"+created.ID+"/publish", m{"confirmFictional": true}, inst), http.StatusOK)
	sess := decode[sessionEnvelope](t, api.do(http.MethodPost, "/api/v1/sessions", m{"scenarioId": created.ID}, inst)).Session
	if sess.Name != "Exercise Tidewatch" {
		t.Errorf("default session name = %q, want the scenario title", sess.Name)
	}
	wantErrorCode(t, api.do(http.MethodPost, "/api/v1/sessions/"+sess.ID+"/start", nil, inst), http.StatusConflict, "NO_PARTICIPANTS")
}
