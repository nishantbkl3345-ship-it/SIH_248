package engine

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

type ChannelView struct {
	ID       uuid.UUID   `json:"id"`
	Name     string      `json:"name"`
	Kind     ChannelKind `json:"kind"`
	Writable bool        `json:"writable"`
}

type PhaseView struct {
	Index int    `json:"index"`
	Count int    `json:"count"`
	Title string `json:"title"`
}

type OwnDecision struct {
	DecisionPointID uuid.UUID `json:"decisionPointId"`
	OptionID        uuid.UUID `json:"optionId"`
	ParticipantID   uuid.UUID `json:"participantId"`
	Rationale       string    `json:"rationale"`
	SimMs           SimMs     `json:"simMs"`
}

// TraineeView is everything one trainee is entitled to see right now, and
// nothing else. It is what a client gets when it connects or reconnects, so
// a reload can never reveal more than the live stream did.
type TraineeView struct {
	Clock         clockPayload        `json:"clock"`
	Phase         *PhaseView          `json:"phase"`
	Team          int                 `json:"team"`
	Channels      []ChannelView       `json:"channels"`
	Reports       []ReportView        `json:"reports"`
	Messages      []MessageView       `json:"messages"`
	OpenDecisions []DecisionPointView `json:"openDecisions"`
	Decisions     []OwnDecision       `json:"decisions"`
}

func (e *Engine) phaseView() *PhaseView {
	if e.st.phaseIndex < 0 {
		return nil
	}
	return &PhaseView{Index: e.st.phaseIndex, Count: len(e.def.Phases), Title: e.def.Phases[e.st.phaseIndex].Title}
}

func (e *Engine) clockAt(wall time.Time) clockPayload {
	p := e.clockPayload()
	p.SimMs = e.clock.Now(wall)
	return p
}

// TraineeView builds the view for one participant. It does not advance the
// exercise; the caller advances first if it wants the latest state.
func (e *Engine) TraineeView(wall time.Time, participantID uuid.UUID) (TraineeView, error) {
	p, ok := e.byID[participantID]
	if !ok {
		return TraineeView{}, reject("NOT_A_PARTICIPANT", "you are not a participant in this exercise")
	}
	v := TraineeView{
		Clock:         e.clockAt(wall),
		Phase:         e.phaseView(),
		Team:          p.Team,
		Channels:      []ChannelView{},
		Reports:       []ReportView{},
		Messages:      []MessageView{},
		OpenDecisions: []DecisionPointView{},
		Decisions:     []OwnDecision{},
	}
	for _, c := range e.def.Channels {
		v.Channels = append(v.Channels, ChannelView{ID: c.ID, Name: c.Name, Kind: c.Kind, Writable: c.Kind == ChannelTeam || c.Kind == ChannelCrossTeam})
	}
	for _, d := range e.st.inbox[p.Team] {
		switch {
		case !d.Message:
			v.Reports = append(v.Reports, d.reportView())
		case d.SenderID != p.ID:
			v.Messages = append(v.Messages, e.messageView(d))
		}
	}
	for _, d := range e.st.sent[p.ID] {
		v.Messages = append(v.Messages, e.messageView(d))
	}
	sort.SliceStable(v.Messages, func(i, j int) bool { return v.Messages[i].ReceivedAtMs < v.Messages[j].ReceivedAtMs })

	for _, d := range e.def.DecisionPoints {
		if _, open := e.st.openDecisions[d.ID]; open {
			v.OpenDecisions = append(v.OpenDecisions, d.view())
		}
	}
	for _, d := range e.st.decisions {
		point := e.decisionPoint(d.DecisionPointID)
		mine := d.ParticipantID == p.ID || (point != nil && point.Scope == ScopeTeam && d.Team == p.Team)
		if mine {
			v.Decisions = append(v.Decisions, OwnDecision{
				DecisionPointID: d.DecisionPointID, OptionID: d.OptionID, ParticipantID: d.ParticipantID,
				Rationale: d.Rationale, SimMs: d.SimMs,
			})
		}
	}
	return v, nil
}

func (e *Engine) decisionPoint(id uuid.UUID) *DecisionPoint {
	for i := range e.def.DecisionPoints {
		if e.def.DecisionPoints[i].ID == id {
			return &e.def.DecisionPoints[i]
		}
	}
	return nil
}

type TeamStatus struct {
	Team           int `json:"team"`
	ReportsHeld    int `json:"reportsHeld"`
	MessagesHeld   int `json:"messagesHeld"`
	PendingReports int `json:"pendingReports"`
	DroppedReports int `json:"droppedReports"`
}

// InstructorView is the ground-truth summary for the monitoring screen. The
// detail behind it is the timeline.
type InstructorView struct {
	Clock         clockPayload        `json:"clock"`
	Phase         *PhaseView          `json:"phase"`
	ActiveRules   []rulePayload       `json:"activeRules"`
	OpenDecisions []DecisionPointView `json:"openDecisions"`
	Decisions     []DecisionRecord    `json:"decisions"`
	Teams         []TeamStatus        `json:"teams"`
	LastSeq       int64               `json:"lastSeq"`
}

func (e *Engine) InstructorView(wall time.Time) InstructorView {
	v := InstructorView{
		Clock:         e.clockAt(wall),
		Phase:         e.phaseView(),
		ActiveRules:   []rulePayload{},
		OpenDecisions: []DecisionPointView{},
		Decisions:     append([]DecisionRecord{}, e.st.decisions...),
		Teams:         []TeamStatus{},
		LastSeq:       int64(e.rec.Len()),
	}
	for _, r := range e.activeRules() {
		p := rulePayloadOf(r)
		p.UntilMs = r.EndMs
		v.ActiveRules = append(v.ActiveRules, p)
	}
	for _, d := range e.def.DecisionPoints {
		if _, open := e.st.openDecisions[d.ID]; open {
			v.OpenDecisions = append(v.OpenDecisions, d.view())
		}
	}
	for _, team := range e.teams(0) {
		ts := TeamStatus{Team: team, PendingReports: e.st.pending[team], DroppedReports: e.st.dropped[team]}
		for _, d := range e.st.inbox[team] {
			if d.Message {
				ts.MessagesHeld++
			} else {
				ts.ReportsHeld++
			}
		}
		v.Teams = append(v.Teams, ts)
	}
	return v
}
