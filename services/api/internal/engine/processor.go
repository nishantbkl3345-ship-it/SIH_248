package engine

import (
	"sort"

	"github.com/google/uuid"
)

// schedule queues the whole script. Insertion order within a rank follows the
// order the instructor arranged things in, which is the last tie-break.
func (e *Engine) schedule() {
	for i, p := range e.def.Phases {
		e.sched.Schedule(task{at: p.StartMs, rank: rankPhaseStarted, index: i})
		e.sched.Schedule(task{at: p.EndMs, rank: rankPhaseCompleted, index: i})
	}
	for i, r := range e.def.Rules {
		if r.Mode == ModeConflicting {
			e.sched.Schedule(task{at: r.StartMs, rank: rankGenerate, index: -(i + 1)})
			continue
		}
		e.sched.Schedule(task{at: r.StartMs, rank: rankRuleStart, index: i})
		e.sched.Schedule(task{at: r.EndMs, rank: rankRuleEnd, index: i})
	}
	for i, r := range e.def.Reports {
		e.sched.Schedule(task{at: r.AtMs, rank: rankGenerate, index: i})
	}
	for i, d := range e.def.DecisionPoints {
		e.sched.Schedule(task{at: d.OpenMs, rank: rankDecisionOpen, index: i})
		e.sched.Schedule(task{at: d.CloseMs, rank: rankDecisionClose, index: i})
	}
	e.sched.Schedule(task{at: e.def.TotalMs, rank: rankExerciseEnd})
}

// advance runs every due task in order. Each is processed at its own
// scheduled simulation time and the wall time that corresponds to it, not the
// time of the call, so how often Advance is called has no effect on anything
// that is recorded.
func (e *Engine) advance() {
	if e.st.status != StatusRunning {
		return
	}
	callWall := e.wall
	target := e.clock.Now(callWall)
	for {
		t, ok := e.sched.PopDue(target)
		if !ok {
			break
		}
		e.now = t.at
		e.wall = e.clock.WallAt(t.at)
		e.process(t)
		if e.st.status == StatusEnded {
			return
		}
	}
	e.now = target
	e.wall = callWall
}

func (e *Engine) process(t task) {
	switch t.rank {
	case rankPhaseStarted:
		e.startPhase(t.index)
	case rankPhaseCompleted:
		e.completePhase(t.index)
	case rankRuleStart:
		e.startRule(t.index)
	case rankRuleEnd:
		e.endRule(t.index)
	case rankGenerate:
		if t.index < 0 {
			e.generateConflict(e.def.Rules[-t.index-1])
		} else {
			e.generateReport(e.def.Reports[t.index])
		}
	case rankRelease:
		e.release(t.delivery)
	case rankDecisionOpen:
		e.openDecision(e.def.DecisionPoints[t.index])
	case rankDecisionClose:
		e.closeDecision(e.def.DecisionPoints[t.index])
	case rankExerciseEnd:
		e.end("COMPLETED")
	}
}

type phasePayload struct {
	PhaseID uuid.UUID `json:"phaseId"`
	Index   int       `json:"index"`
	Title   string    `json:"title"`
	StartMs SimMs     `json:"startMs"`
	EndMs   SimMs     `json:"endMs"`
}

func (e *Engine) startPhase(i int) {
	p := e.def.Phases[i]
	e.st.phaseIndex = i
	payload := phasePayload{PhaseID: p.ID, Index: i, Title: p.Title, StartMs: p.StartMs, EndMs: p.EndMs}
	e.record(EvPhaseStarted, 0, nil, payload)
	e.tell(NPhaseChanged, Audience{AllTrainees: true}, payload)
}

func (e *Engine) completePhase(i int) {
	p := e.def.Phases[i]
	e.record(EvPhaseCompleted, 0, nil, phasePayload{PhaseID: p.ID, Index: i, Title: p.Title, StartMs: p.StartMs, EndMs: p.EndMs})
}

type rulePayload struct {
	RuleID      uuid.UUID  `json:"ruleId"`
	Name        string     `json:"name,omitempty"`
	Mode        RuleMode   `json:"mode"`
	ChannelID   *uuid.UUID `json:"channelId"`
	Team        int        `json:"team"`
	DelayMs     int64      `json:"delayMs,omitempty"`
	KeepPercent int        `json:"keepPercent,omitempty"`
	UntilMs     SimMs      `json:"untilMs,omitempty"`
	// StillActive lists degrading rules that remain in force on an
	// overlapping scope, so "restored" is never read as "all clear".
	StillActive []uuid.UUID `json:"stillActive,omitempty"`
	Reason      string      `json:"reason,omitempty"`
}

func rulePayloadOf(r Rule) rulePayload {
	return rulePayload{RuleID: r.ID, Name: r.Name, Mode: r.Mode, ChannelID: r.ChannelID, Team: r.Team, DelayMs: r.DelayMs, KeepPercent: r.KeepPercent}
}

// Communication changes go to the instructor only. Trainees are never told
// that a link is degraded; they experience it.
func (e *Engine) startRule(i int) {
	r := e.def.Rules[i]
	at := sort.SearchInts(e.st.activeRules, i)
	e.st.activeRules = append(e.st.activeRules, 0)
	copy(e.st.activeRules[at+1:], e.st.activeRules[at:])
	e.st.activeRules[at] = i

	p := rulePayloadOf(r)
	p.UntilMs = r.EndMs
	if r.Mode == ModeNormal {
		p.Reason = "FORCED_NORMAL"
		e.record(EvCommunicationRestored, r.Team, nil, p)
		return
	}
	e.record(EvCommunicationDegraded, r.Team, nil, p)
}

func (e *Engine) endRule(i int) {
	r := e.def.Rules[i]
	at := sort.SearchInts(e.st.activeRules, i)
	if at == len(e.st.activeRules) || e.st.activeRules[at] != i {
		return
	}
	e.st.activeRules = append(e.st.activeRules[:at], e.st.activeRules[at+1:]...)

	p := rulePayloadOf(r)
	for _, other := range e.activeRules() {
		if other.Mode != ModeNormal && overlaps(r, other) {
			p.StillActive = append(p.StillActive, other.ID)
		}
	}
	if r.Mode == ModeNormal {
		// The forced-clear window is over; say so only if that changes anything.
		if len(p.StillActive) > 0 {
			p.Reason = "FORCED_NORMAL_ENDED"
			e.record(EvCommunicationDegraded, r.Team, nil, p)
		}
		return
	}
	e.record(EvCommunicationRestored, r.Team, nil, p)
}

func overlaps(a, b Rule) bool {
	channels := a.ChannelID == nil || b.ChannelID == nil || *a.ChannelID == *b.ChannelID
	teams := a.Team == 0 || b.Team == 0 || a.Team == b.Team
	return channels && teams
}
