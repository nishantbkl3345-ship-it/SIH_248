package engine

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxRationaleRunes = 4000

// DecisionPointView is a decision point as trainees see it. Evaluation
// criteria are never part of it.
type DecisionPointView struct {
	ID                uuid.UUID        `json:"id"`
	Question          string           `json:"question"`
	Scope             DecisionScope    `json:"scope"`
	RationaleRequired bool             `json:"rationaleRequired"`
	OpenedAtMs        SimMs            `json:"openedAtMs"`
	ClosesAtMs        SimMs            `json:"closesAtMs"`
	Options           []DecisionOption `json:"options"`
}

func (d DecisionPoint) view() DecisionPointView {
	return DecisionPointView{
		ID: d.ID, Question: d.Question, Scope: d.Scope, RationaleRequired: d.RationaleRequired,
		OpenedAtMs: d.OpenMs, ClosesAtMs: d.CloseMs, Options: append([]DecisionOption{}, d.Options...),
	}
}

func (e *Engine) openDecision(d DecisionPoint) {
	e.st.openDecisions[d.ID] = openDecision{openedMs: e.now, openedWall: e.wall}
	e.record(EvDecisionPointOpened, 0, nil, d.view())
	e.tell(NDecisionOpened, Audience{AllTrainees: true}, d.view())
}

type decisionClosedPayload struct {
	DecisionPointID uuid.UUID `json:"decisionPointId"`
}

func (e *Engine) closeDecision(d DecisionPoint) {
	if _, open := e.st.openDecisions[d.ID]; !open {
		return
	}
	delete(e.st.openDecisions, d.ID)
	p := decisionClosedPayload{DecisionPointID: d.ID}
	e.record(EvDecisionPointClosed, 0, nil, p)
	e.tell(NDecisionClosed, Audience{AllTrainees: true}, p)
}

// DecisionInput is what a trainee supplies. Everything else in the record
// (time, team, response time, the information picture) comes from the engine.
type DecisionInput struct {
	ParticipantID   uuid.UUID
	DecisionPointID uuid.UUID
	OptionID        uuid.UUID
	Rationale       string
	Confidence      *int // optional, 1 to 5
}

// SubmittedView is what the decider's side is told about a recorded decision.
type SubmittedView struct {
	DecisionID      uuid.UUID `json:"decisionId"`
	DecisionPointID uuid.UUID `json:"decisionPointId"`
	OptionID        uuid.UUID `json:"optionId"`
	ParticipantID   uuid.UUID `json:"participantId"`
	ParticipantName string    `json:"participantName"`
	Team            int       `json:"team"`
	SimMs           SimMs     `json:"simMs"`
}

// SubmitDecision records a decision if the point is open for this trainee.
// A team-scope point takes one decision per team; an individual one takes one
// per trainee. Decisions cannot be changed once recorded.
func (e *Engine) SubmitDecision(wall time.Time, in DecisionInput) (DecisionRecord, Output, error) {
	e.begin(wall)
	e.advance() // a deadline that has passed closes the point before we look
	p, rejected := e.participant(in.ParticipantID)
	if rejected != nil {
		return DecisionRecord{}, e.finish(), rejected
	}

	var point *DecisionPoint
	for i := range e.def.DecisionPoints {
		if e.def.DecisionPoints[i].ID == in.DecisionPointID {
			point = &e.def.DecisionPoints[i]
		}
	}
	if point == nil {
		return DecisionRecord{}, e.finish(), reject("UNKNOWN_DECISION_POINT", "that decision point does not exist in this exercise")
	}
	opened, open := e.st.openDecisions[point.ID]
	if !open {
		return DecisionRecord{}, e.finish(), reject("DECISION_NOT_OPEN", "this decision point is not open")
	}
	valid := false
	for _, o := range point.Options {
		valid = valid || o.ID == in.OptionID
	}
	if !valid {
		return DecisionRecord{}, e.finish(), reject("UNKNOWN_OPTION", "that is not one of the available options")
	}
	rationale := strings.TrimSpace(in.Rationale)
	if point.RationaleRequired && rationale == "" {
		return DecisionRecord{}, e.finish(), reject("RATIONALE_REQUIRED", "explain your reasoning before submitting")
	}
	if utf8.RuneCountInString(rationale) > MaxRationaleRunes {
		return DecisionRecord{}, e.finish(), reject("RATIONALE_TOO_LONG", "rationale must be at most %d characters", MaxRationaleRunes)
	}
	if in.Confidence != nil && (*in.Confidence < 1 || *in.Confidence > 5) {
		return DecisionRecord{}, e.finish(), reject("INVALID_CONFIDENCE", "confidence must be between 1 and 5")
	}

	key := decisionKey{point: point.ID, participant: p.ID}
	audience := Audience{Participants: []uuid.UUID{p.ID}}
	if point.Scope == ScopeTeam {
		key = decisionKey{point: point.ID, team: p.Team}
		audience = Audience{Teams: []int{p.Team}}
	}
	if e.st.decided[key] {
		who := "you have"
		if point.Scope == ScopeTeam {
			who = "your team has"
		}
		return DecisionRecord{}, e.finish(), reject("ALREADY_DECIDED", "%s already submitted a decision here", who)
	}
	e.st.decided[key] = true

	rec := DecisionRecord{
		ID:              e.newID("decision"),
		DecisionPointID: point.ID,
		OptionID:        in.OptionID,
		ParticipantID:   p.ID,
		Team:            p.Team,
		Rationale:       rationale,
		Confidence:      in.Confidence,
		SimMs:           e.now,
		Wall:            wall,
		ResponseMs:      e.now - opened.openedMs,
		ResponseWallMs:  wall.Sub(opened.openedWall).Milliseconds(),
		Snapshot:        e.snapshot(p),
	}
	e.st.decisions = append(e.st.decisions, rec)
	e.out.Decisions = append(e.out.Decisions, rec)

	pid := p.ID
	e.record(EvDecisionSubmitted, p.Team, &pid, rec)
	e.tell(NDecisionSubmitted, audience, SubmittedView{
		DecisionID: rec.ID, DecisionPointID: point.ID, OptionID: in.OptionID,
		ParticipantID: p.ID, ParticipantName: p.Name, Team: p.Team, SimMs: e.now,
	})
	return rec, e.finish(), nil
}

// snapshot captures what a trainee had in front of them, and what they did
// not, at this instant.
func (e *Engine) snapshot(p Participant) DecisionSnapshot {
	snap := DecisionSnapshot{
		Reports:           []AvailableItem{},
		Messages:          []AvailableItem{},
		ActiveDegradation: []ActiveRule{},
		PendingReports:    e.st.pending[p.Team],
		DroppedReports:    e.st.dropped[p.Team],
	}
	if e.st.phaseIndex >= 0 {
		snap.PhaseID = e.def.Phases[e.st.phaseIndex].ID
	}
	for _, d := range e.st.inbox[p.Team] {
		item := AvailableItem{DeliveryID: d.ID, ItemID: d.ItemID, Source: d.Source, Title: d.Title, State: d.State, ReceivedAtMs: d.DeliveredMs}
		switch {
		case !d.Message:
			snap.Reports = append(snap.Reports, item)
		case d.SenderID != p.ID:
			snap.Messages = append(snap.Messages, item)
		}
	}
	for _, d := range e.st.sent[p.ID] {
		snap.Messages = append(snap.Messages, AvailableItem{DeliveryID: d.ID, ItemID: d.ItemID, Source: d.Source, State: StateNormal, ReceivedAtMs: d.DeliveredMs})
	}
	for _, r := range e.activeRules() {
		if r.Team == 0 || r.Team == p.Team {
			snap.ActiveDegradation = append(snap.ActiveDegradation, ActiveRule{RuleID: r.ID, Mode: r.Mode, ChannelID: r.ChannelID, Team: r.Team})
		}
	}
	return snap
}
