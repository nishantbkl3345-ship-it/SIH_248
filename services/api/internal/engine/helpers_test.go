package engine

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Everything in these tests is driven by explicit wall times relative to t0;
// nothing reads the real clock, so there is nothing to make them flaky.
var t0 = time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)

func wallAt(sec float64) time.Time { return t0.Add(time.Duration(sec * float64(time.Second))) }

// id gives a stable, readable id for a name.
func id(name string) uuid.UUID { return uuid.NewSHA1(uuid.Nil, []byte(name)) }

func idp(name string) *uuid.UUID { v := id(name); return &v }

const sec = int64(1000)

var (
	chFeed  = id("feed")
	chTeam  = id("team-net")
	chCross = id("coord-net")
)

func baseDef(totalSec int64) Definition {
	return Definition{
		TeamCount: 2,
		TotalMs:   totalSec * sec,
		Channels: []Channel{
			{ID: chTeam, Name: "Team Net", Kind: ChannelTeam},
			{ID: chCross, Name: "Coordination Net", Kind: ChannelCrossTeam},
			{ID: chFeed, Name: "Field Feed", Kind: ChannelFeed},
		},
		Phases: []Phase{{ID: id("phase-1"), Title: "Assessment", StartMs: 0, EndMs: totalSec * sec}},
	}
}

func report(name string, atSec int64, team int) Report {
	return Report{
		ID: id(name), AtMs: atSec * sec, ChannelID: chFeed, Team: team, Title: name,
		Source: "Synthetic Sensor Alpha", Reliability: "B", Priority: "ROUTINE",
		Content: "Observation for " + name + ": the eastern route is reported clear at this time.",
	}
}

func rule(name string, mode RuleMode, fromSec, toSec int64) Rule {
	return Rule{ID: id(name), Name: name, Mode: mode, StartMs: fromSec * sec, EndMs: toSec * sec}
}

var (
	alice = Participant{ID: id("alice"), Name: "Alice", Team: 1}
	arun  = Participant{ID: id("arun"), Name: "Arun", Team: 1}
	bela  = Participant{ID: id("bela"), Name: "Bela", Team: 2}
)

func roster() []Participant { return []Participant{alice, arun, bela} }

// sim wraps an engine and keeps everything it has ever emitted.
type sim struct {
	t      *testing.T
	e      *Engine
	events []Event
	notes  []Notification
}

func start(t *testing.T, def Definition, speedMilli int64) *sim {
	t.Helper()
	e, err := New(def, roster(), 42, speedMilli)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s := &sim{t: t, e: e}
	out, err := e.Start(t0)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.take(out)
	return s
}

func (s *sim) take(out Output) {
	s.events = append(s.events, out.Events...)
	s.notes = append(s.notes, out.Notifications...)
}

// to advances the exercise to the given wall second.
func (s *sim) to(wallSec float64) { s.take(s.e.Advance(wallAt(wallSec))) }

// lines renders events as "T+<seconds> TYPE[ team]" for compact assertions.
func lines(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		l := fmt.Sprintf("T+%g %s", float64(e.SimMs)/1000, e.Type)
		if e.Team != 0 {
			l += fmt.Sprintf(" team=%d", e.Team)
		}
		out[i] = l
	}
	return out
}

func (s *sim) only(types ...EventType) []Event {
	var out []Event
	for _, e := range s.events {
		for _, typ := range types {
			if e.Type == typ {
				out = append(out, e)
			}
		}
	}
	return out
}

func wantLines(t *testing.T, got []Event, want ...string) {
	t.Helper()
	g := lines(got)
	if strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Fatalf("timeline mismatch\n got:\n  %s\nwant:\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}
}

func payload[T any](t *testing.T, e Event) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(e.Payload, &v); err != nil {
		t.Fatalf("payload of %s: %v", e.Type, err)
	}
	return v
}

// toTrainee returns the notifications a participant would receive.
func (s *sim) toTrainee(p Participant, typ string) []Notification {
	var out []Notification
	for _, n := range s.notes {
		if !n.Audience.Instructor && n.Type == typ && n.Audience.Includes(p) {
			out = append(out, n)
		}
	}
	return out
}

func (s *sim) view(p Participant, wallSec float64) TraineeView {
	s.t.Helper()
	v, err := s.e.TraineeView(wallAt(wallSec), p.ID)
	if err != nil {
		s.t.Fatalf("TraineeView: %v", err)
	}
	return v
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *Error
	if err == nil {
		t.Fatalf("expected error %s, got nil", code)
	}
	if !asError(err, &e) || e.Code != code {
		t.Fatalf("error = %v, want code %s", err, code)
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
