package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"fogline/api/internal/domain"
)

const (
	SeverityError   = "error"   // blocks publishing
	SeverityWarning = "warning" // worth a look, does not block
)

// Issue is one thing standing between a draft and a publishable scenario.
// Drafts are allowed to be incomplete, so issues never block saving.
type Issue struct {
	Severity string
	// Path locates the field, e.g. "phases[0].reports[2].channelId".
	Path    string
	Message string
	// PhaseID and EntityID let a client jump to the offending item.
	PhaseID  *uuid.UUID
	EntityID *uuid.UUID
}

func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Objectives decodes a scenario's learning objectives.
func Objectives(s *domain.Scenario) []string {
	var out []string
	if len(s.Objectives) > 0 {
		_ = json.Unmarshal(s.Objectives, &out)
	}
	return out
}

// RuleParams decodes a rule's mode-specific parameters.
func RuleParams(r *domain.DegradationRule) domain.RuleParams {
	var p domain.RuleParams
	if len(r.Params) > 0 {
		_ = json.Unmarshal(r.Params, &p)
	}
	return p
}

// ReviewScenario reports everything that would stop the scenario running as
// a sound exercise. It is pure: same scenario in, same issues out.
func ReviewScenario(s *domain.Scenario) []Issue {
	v := &reviewer{scenario: s}

	if blank(s.Title) {
		v.err("title", "Give the scenario a name.")
	}
	if blank(s.Summary) {
		v.err("summary", "Describe the scenario so trainees and other instructors know what it covers.")
	}
	objectives := Objectives(s)
	if len(objectives) == 0 {
		v.err("objectives", "Add at least one learning objective.")
	}
	for i, o := range objectives {
		if blank(o) {
			v.err(fmt.Sprintf("objectives[%d]", i), "Learning objective is empty.")
		}
	}
	if s.TraineeCount < s.TeamCount {
		v.err("traineeCount", "There must be at least one trainee per team.")
	}

	if len(s.Channels) == 0 {
		v.err("channels", "Add at least one communication channel.")
	}
	names := map[string]bool{}
	for i, c := range s.Channels {
		path := fmt.Sprintf("channels[%d].name", i)
		key := strings.ToLower(strings.TrimSpace(c.Name))
		switch {
		case key == "":
			v.err(path, "Channel needs a name.")
		case names[key]:
			v.err(path, fmt.Sprintf("Two channels are both called %q.", strings.TrimSpace(c.Name)))
		}
		names[key] = true
	}

	if len(s.Phases) == 0 {
		v.err("phases", "Add at least one phase.")
	}

	var reports, decisions, degradations int
	for i := range s.Phases {
		p := &s.Phases[i]
		v.phase, v.prefix = p, fmt.Sprintf("phases[%d]", i)

		if blank(p.Title) {
			v.phaseErr("title", "Phase needs a title.")
		}
		if p.DurationSec <= 0 {
			v.phaseErr("durationSec", "Phase needs a duration.")
		}

		for j := range p.Reports {
			reports++
			v.report(j, &p.Reports[j])
		}
		for j := range p.Rules {
			if p.Rules[j].Mode != domain.ModeNormal {
				degradations++
			}
			v.rule(j, &p.Rules[j])
		}
		for j := range p.DecisionPoints {
			decisions++
			v.decision(j, &p.DecisionPoints[j])
		}
	}
	v.phase, v.prefix = nil, ""

	if len(s.Phases) > 0 {
		if reports == 0 {
			v.err("phases", "Add at least one information report: trainees need something to act on.")
		}
		if decisions == 0 {
			v.err("phases", "Add at least one decision point: trainees need something to decide.")
		}
		if degradations == 0 {
			v.warn("phases", "No degradation is configured, so the exercise would run with perfect communications.")
		}
	}
	return v.issues
}

type reviewer struct {
	scenario *domain.Scenario
	phase    *domain.ScenarioPhase
	prefix   string
	issues   []Issue
}

func (v *reviewer) add(severity, path, msg string, entity *uuid.UUID) {
	issue := Issue{Severity: severity, Path: path, Message: msg, EntityID: entity}
	if v.phase != nil {
		id := v.phase.ID
		issue.PhaseID = &id
	}
	v.issues = append(v.issues, issue)
}

func (v *reviewer) err(path, msg string)  { v.add(SeverityError, path, msg, nil) }
func (v *reviewer) warn(path, msg string) { v.add(SeverityWarning, path, msg, nil) }

func (v *reviewer) phaseErr(field, msg string) {
	id := v.phase.ID
	v.add(SeverityError, v.prefix+"."+field, msg, &id)
}

// item returns reporters bound to one entity inside the current phase.
func (v *reviewer) item(collection string, index int, id uuid.UUID) (errf, warnf func(field, msg string)) {
	base := fmt.Sprintf("%s.%s[%d].", v.prefix, collection, index)
	errf = func(field, msg string) { v.add(SeverityError, base+field, msg, &id) }
	warnf = func(field, msg string) { v.add(SeverityWarning, base+field, msg, &id) }
	return
}

func (v *reviewer) checkTeam(team *int, fail func(field, msg string)) {
	if team != nil && *team > v.scenario.TeamCount {
		fail("targetTeam", fmt.Sprintf("Targets team %d, but the scenario has %d.", *team, v.scenario.TeamCount))
	}
}

func (v *reviewer) checkStart(offset int, fail func(field, msg string)) bool {
	if v.phase.DurationSec > 0 && offset > v.phase.DurationSec {
		fail("atSec", "Starts after the phase has ended.")
		return false
	}
	return true
}

func (v *reviewer) report(i int, r *domain.InformationReport) {
	errf, _ := v.item("reports", i, r.ID)
	if blank(r.Title) {
		errf("title", "Report needs a title.")
	}
	if blank(r.SourceName) {
		errf("source", "Report needs a source.")
	}
	if blank(r.Body) {
		errf("content", "Report has no content.")
	}
	if r.ChannelID == nil {
		errf("channelId", "Choose the channel this report is delivered on.")
	}
	v.checkTeam(r.TargetTeam, errf)
	v.checkStart(r.OffsetSec, errf)
}

func (v *reviewer) rule(i int, r *domain.DegradationRule) {
	errf, warnf := v.item("rules", i, r.ID)
	params := RuleParams(r)

	v.checkTeam(r.TargetTeam, errf)
	startOK := v.checkStart(r.OffsetSec, errf)

	if r.Mode == domain.ModeConflicting {
		// A conflict is an instant, not a window.
		if blank(params.SourceA) {
			errf("conflict.sourceA", "Name the first source.")
		}
		if blank(params.SourceB) {
			errf("conflict.sourceB", "Name the second source.")
		}
		if blank(params.ContentA) {
			errf("conflict.contentA", "Write what the first source reports.")
		}
		if blank(params.ContentB) {
			errf("conflict.contentB", "Write what the second source reports.")
		}
		if !blank(params.ContentA) && strings.EqualFold(strings.TrimSpace(params.ContentA), strings.TrimSpace(params.ContentB)) {
			errf("conflict.contentB", "Both sources say the same thing, so there is no conflict.")
		}
		if !blank(params.SourceA) && strings.EqualFold(strings.TrimSpace(params.SourceA), strings.TrimSpace(params.SourceB)) {
			warnf("conflict.sourceB", "Both reports come from the same source.")
		}
		if r.ChannelID == nil {
			errf("channelId", "Choose the channel the conflicting reports arrive on.")
		}
		return
	}

	if r.DurationSec <= 0 {
		errf("durationSec", "Set how long this lasts.")
	} else if startOK && v.phase.DurationSec > 0 && r.OffsetSec+r.DurationSec > v.phase.DurationSec {
		warnf("durationSec", "Runs past the end of the phase; it will be cut off at the phase boundary.")
	}
	switch r.Mode {
	case domain.ModeDelay:
		if params.DelaySec <= 0 {
			errf("delaySec", "Set how long information is delayed.")
		}
	case domain.ModePartial:
		if params.KeepPercent < 1 || params.KeepPercent > 99 {
			errf("keepPercent", "Set the share of content that gets through, between 1% and 99%.")
		}
	}
}

func (v *reviewer) decision(i int, d *domain.DecisionPoint) {
	errf, warnf := v.item("decisionPoints", i, d.ID)
	if blank(d.Prompt) {
		errf("question", "Decision point needs a question.")
	}
	if len(d.Options) < 2 {
		errf("options", "Offer at least two options.")
	}
	seen := map[string]bool{}
	for j, o := range d.Options {
		key := strings.ToLower(strings.TrimSpace(o.Label))
		switch {
		case key == "":
			errf(fmt.Sprintf("options[%d].label", j), "Option needs a label.")
		case seen[key]:
			errf(fmt.Sprintf("options[%d].label", j), "Two options have the same label.")
		}
		seen[key] = true
	}
	startOK := v.checkStart(d.OffsetSec, errf)
	if d.TimeLimitSec <= 0 {
		errf("deadlineSec", "Set a decision deadline.")
	} else if startOK && v.phase.DurationSec > 0 && d.OffsetSec+d.TimeLimitSec > v.phase.DurationSec {
		warnf("deadlineSec", "Deadline falls after the end of the phase.")
	}
	if blank(d.EvaluationCriteria) {
		warnf("evaluationCriteria", "No evaluation criteria: the review will have nothing to assess this decision against.")
	}
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
