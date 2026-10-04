package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/service"
)

// The scenario is exchanged as one document. Times inside a phase (atSec) are
// seconds from the start of that phase. Limits here bound what can be stored;
// whether a draft is complete enough to run is reported separately as issues.

type channelDTO struct {
	ID             string `json:"id" binding:"required,uuid"`
	Name           string `json:"name" binding:"max=60"`
	Kind           string `json:"kind" binding:"required,oneof=TEAM CROSS_TEAM BROADCAST FEED"`
	BaseLatencySec int    `json:"baseLatencySec" binding:"min=0,max=600"`
}

type reportDTO struct {
	ID          string  `json:"id" binding:"required,uuid"`
	Title       string  `json:"title" binding:"max=120"`
	Source      string  `json:"source" binding:"max=120"`
	Content     string  `json:"content" binding:"max=4000"`
	AtSec       int     `json:"atSec" binding:"min=0,max=86400"`
	Priority    string  `json:"priority" binding:"required,oneof=LOW ROUTINE HIGH CRITICAL"`
	Reliability string  `json:"reliability" binding:"required,oneof=A B C D E"`
	TargetTeam  *int    `json:"targetTeam" binding:"omitempty,min=1,max=8"`
	ChannelID   *string `json:"channelId" binding:"omitempty,uuid"`
}

type conflictDTO struct {
	Topic    string `json:"topic" binding:"max=120"`
	SourceA  string `json:"sourceA" binding:"max=120"`
	ContentA string `json:"contentA" binding:"max=2000"`
	SourceB  string `json:"sourceB" binding:"max=120"`
	ContentB string `json:"contentB" binding:"max=2000"`
}

type ruleDTO struct {
	ID          string       `json:"id" binding:"required,uuid"`
	Name        string       `json:"name" binding:"max=120"`
	Mode        string       `json:"mode" binding:"required,oneof=NORMAL DELAY DROPOUT PARTIAL CONFLICTING"`
	AtSec       int          `json:"atSec" binding:"min=0,max=86400"`
	DurationSec int          `json:"durationSec" binding:"min=0,max=86400"`
	ChannelID   *string      `json:"channelId" binding:"omitempty,uuid"`
	TargetTeam  *int         `json:"targetTeam" binding:"omitempty,min=1,max=8"`
	DelaySec    int          `json:"delaySec" binding:"min=0,max=3600"`
	KeepPercent int          `json:"keepPercent" binding:"min=0,max=100"`
	Conflict    *conflictDTO `json:"conflict"`
}

type optionDTO struct {
	ID          string `json:"id" binding:"required,uuid"`
	Label       string `json:"label" binding:"max=200"`
	Description string `json:"description" binding:"max=1000"`
}

type decisionPointDTO struct {
	ID                 string      `json:"id" binding:"required,uuid"`
	Question           string      `json:"question" binding:"max=1000"`
	Scope              string      `json:"scope" binding:"required,oneof=INDIVIDUAL TEAM"`
	AtSec              int         `json:"atSec" binding:"min=0,max=86400"`
	DeadlineSec        int         `json:"deadlineSec" binding:"min=0,max=86400"`
	RationaleRequired  bool        `json:"rationaleRequired"`
	EvaluationCriteria string      `json:"evaluationCriteria" binding:"max=4000"`
	Options            []optionDTO `json:"options" binding:"max=8,dive"`
}

type phaseDTO struct {
	ID             string             `json:"id" binding:"required,uuid"`
	Title          string             `json:"title" binding:"max=120"`
	Description    string             `json:"description" binding:"max=4000"`
	DurationSec    int                `json:"durationSec" binding:"min=0,max=86400"`
	Reports        []reportDTO        `json:"reports" binding:"max=100,dive"`
	Rules          []ruleDTO          `json:"rules" binding:"max=50,dive"`
	DecisionPoints []decisionPointDTO `json:"decisionPoints" binding:"max=30,dive"`
}

type scenarioContent struct {
	Title        string       `json:"title" binding:"max=120"`
	Summary      string       `json:"summary" binding:"max=2000"`
	Difficulty   string       `json:"difficulty" binding:"required,oneof=BASIC INTERMEDIATE ADVANCED"`
	TeamCount    int          `json:"teamCount" binding:"min=1,max=8"`
	TraineeCount int          `json:"traineeCount" binding:"min=1,max=60"`
	Objectives   []string     `json:"objectives" binding:"max=12,dive,max=300"`
	Channels     []channelDTO `json:"channels" binding:"max=12,dive"`
	Phases       []phaseDTO   `json:"phases" binding:"max=20,dive"`
}

type saveScenarioRequest struct {
	// Revision is the one the client last received.
	Revision int `json:"revision" binding:"required,min=1"`
	scenarioContent
}

type createScenarioRequest struct {
	Title string `json:"title" binding:"required,min=1,max=120"`
}

type publishScenarioRequest struct {
	ConfirmFictional bool `json:"confirmFictional"`
}

type issueDTO struct {
	Severity string  `json:"severity"`
	Path     string  `json:"path"`
	Message  string  `json:"message"`
	PhaseID  *string `json:"phaseId"`
	EntityID *string `json:"entityId"`
}

type scenarioResponse struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Revision       int        `json:"revision"`
	EstDurationSec int        `json:"estDurationSec"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	PublishedAt    *time.Time `json:"publishedAt"`
	scenarioContent
	Issues []issueDTO `json:"issues"`
}

type scenarioSummary struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Summary        string     `json:"summary"`
	Difficulty     string     `json:"difficulty"`
	Status         string     `json:"status"`
	TeamCount      int        `json:"teamCount"`
	TraineeCount   int        `json:"traineeCount"`
	PhaseCount     int        `json:"phaseCount"`
	EstDurationSec int        `json:"estDurationSec"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	PublishedAt    *time.Time `json:"publishedAt"`
}

// ------------------------------------------------------------------ mapping

func optUUID(s *string) *uuid.UUID {
	if s == nil {
		return nil
	}
	id := uuid.MustParse(*s) // format already checked by binding
	return &id
}

func optString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	s := id.String()
	return &s
}

// toScenario converts a validated request body into the domain aggregate.
func (c scenarioContent) toScenario() *domain.Scenario {
	objectives, _ := json.Marshal(append([]string{}, c.Objectives...))
	s := &domain.Scenario{
		Title:        c.Title,
		Summary:      c.Summary,
		Difficulty:   domain.Difficulty(c.Difficulty),
		TeamCount:    c.TeamCount,
		TraineeCount: c.TraineeCount,
		Objectives:   domain.JSONB(objectives),
	}
	for _, ch := range c.Channels {
		s.Channels = append(s.Channels, domain.CommunicationChannel{
			ID: uuid.MustParse(ch.ID), Name: ch.Name,
			Kind: domain.ChannelKind(ch.Kind), BaseLatencySec: ch.BaseLatencySec,
		})
	}
	for _, p := range c.Phases {
		phase := domain.ScenarioPhase{
			ID: uuid.MustParse(p.ID), Title: p.Title,
			Description: p.Description, DurationSec: p.DurationSec,
		}
		for _, r := range p.Reports {
			phase.Reports = append(phase.Reports, domain.InformationReport{
				ID: uuid.MustParse(r.ID), Title: r.Title, SourceName: r.Source, Body: r.Content,
				OffsetSec: r.AtSec, Priority: domain.ReportPriority(r.Priority),
				SourceReliability: r.Reliability, TargetTeam: r.TargetTeam, ChannelID: optUUID(r.ChannelID),
			})
		}
		for _, r := range p.Rules {
			// Only the parameters that mean something for the mode are kept.
			var params domain.RuleParams
			switch domain.DegradationMode(r.Mode) {
			case domain.ModeDelay:
				params.DelaySec = r.DelaySec
			case domain.ModePartial:
				params.KeepPercent = r.KeepPercent
			case domain.ModeConflicting:
				if r.Conflict != nil {
					params.Topic = r.Conflict.Topic
					params.SourceA, params.ContentA = r.Conflict.SourceA, r.Conflict.ContentA
					params.SourceB, params.ContentB = r.Conflict.SourceB, r.Conflict.ContentB
				}
			}
			raw, _ := json.Marshal(params)
			phase.Rules = append(phase.Rules, domain.DegradationRule{
				ID: uuid.MustParse(r.ID), Name: r.Name, Mode: domain.DegradationMode(r.Mode),
				OffsetSec: r.AtSec, DurationSec: r.DurationSec,
				ChannelID: optUUID(r.ChannelID), TargetTeam: r.TargetTeam, Params: domain.JSONB(raw),
			})
		}
		for _, d := range p.DecisionPoints {
			point := domain.DecisionPoint{
				ID: uuid.MustParse(d.ID), Prompt: d.Question, Scope: domain.DecisionScope(d.Scope),
				OffsetSec: d.AtSec, TimeLimitSec: d.DeadlineSec,
				RationaleRequired: d.RationaleRequired, EvaluationCriteria: d.EvaluationCriteria,
			}
			for _, o := range d.Options {
				point.Options = append(point.Options, domain.DecisionOption{
					ID: uuid.MustParse(o.ID), Label: o.Label, Description: o.Description,
				})
			}
			phase.DecisionPoints = append(phase.DecisionPoints, point)
		}
		s.Phases = append(s.Phases, phase)
	}
	return s
}

func toScenarioResponse(s *domain.Scenario, issues []service.Issue) scenarioResponse {
	out := scenarioResponse{
		ID: s.ID.String(), Status: string(s.Status), Revision: s.Revision,
		EstDurationSec: s.EstDurationSec, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		PublishedAt: s.PublishedAt,
		scenarioContent: scenarioContent{
			Title: s.Title, Summary: s.Summary, Difficulty: string(s.Difficulty),
			TeamCount: s.TeamCount, TraineeCount: s.TraineeCount,
			Objectives: append([]string{}, service.Objectives(s)...),
			Channels:   []channelDTO{},
			Phases:     []phaseDTO{},
		},
		Issues: []issueDTO{},
	}
	for _, c := range s.Channels {
		out.Channels = append(out.Channels, channelDTO{
			ID: c.ID.String(), Name: c.Name, Kind: string(c.Kind), BaseLatencySec: c.BaseLatencySec,
		})
	}
	for i := range s.Phases {
		p := &s.Phases[i]
		phase := phaseDTO{
			ID: p.ID.String(), Title: p.Title, Description: p.Description, DurationSec: p.DurationSec,
			Reports: []reportDTO{}, Rules: []ruleDTO{}, DecisionPoints: []decisionPointDTO{},
		}
		for _, r := range p.Reports {
			phase.Reports = append(phase.Reports, reportDTO{
				ID: r.ID.String(), Title: r.Title, Source: r.SourceName, Content: r.Body,
				AtSec: r.OffsetSec, Priority: string(r.Priority), Reliability: r.SourceReliability,
				TargetTeam: r.TargetTeam, ChannelID: optString(r.ChannelID),
			})
		}
		for j := range p.Rules {
			r := &p.Rules[j]
			params := service.RuleParams(r)
			rule := ruleDTO{
				ID: r.ID.String(), Name: r.Name, Mode: string(r.Mode),
				AtSec: r.OffsetSec, DurationSec: r.DurationSec,
				ChannelID: optString(r.ChannelID), TargetTeam: r.TargetTeam,
				DelaySec: params.DelaySec, KeepPercent: params.KeepPercent,
			}
			if r.Mode == domain.ModeConflicting {
				rule.Conflict = &conflictDTO{
					Topic: params.Topic, SourceA: params.SourceA, ContentA: params.ContentA,
					SourceB: params.SourceB, ContentB: params.ContentB,
				}
			}
			phase.Rules = append(phase.Rules, rule)
		}
		for _, d := range p.DecisionPoints {
			point := decisionPointDTO{
				ID: d.ID.String(), Question: d.Prompt, Scope: string(d.Scope),
				AtSec: d.OffsetSec, DeadlineSec: d.TimeLimitSec,
				RationaleRequired: d.RationaleRequired, EvaluationCriteria: d.EvaluationCriteria,
				Options: []optionDTO{},
			}
			for _, o := range d.Options {
				point.Options = append(point.Options, optionDTO{ID: o.ID.String(), Label: o.Label, Description: o.Description})
			}
			phase.DecisionPoints = append(phase.DecisionPoints, point)
		}
		out.Phases = append(out.Phases, phase)
	}
	for _, i := range issues {
		out.Issues = append(out.Issues, issueDTO{
			Severity: i.Severity, Path: i.Path, Message: i.Message,
			PhaseID: optString(i.PhaseID), EntityID: optString(i.EntityID),
		})
	}
	return out
}

// ----------------------------------------------------------------- handlers

type scenarioHandler struct {
	scenarios *service.ScenarioService
}

func (h *scenarioHandler) id(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperr.NotFound("Scenario not found."))
		return uuid.Nil, false
	}
	return id, true
}

func (h *scenarioHandler) respond(c *gin.Context, status int, s *domain.Scenario, issues []service.Issue, err error) {
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(status, gin.H{"scenario": toScenarioResponse(s, issues)})
}

func (h *scenarioHandler) list(c *gin.Context) {
	actor, _ := currentUser(c)
	scenarios, err := h.scenarios.List(c.Request.Context(), actor)
	if err != nil {
		_ = c.Error(err)
		return
	}
	out := make([]scenarioSummary, len(scenarios))
	for i, s := range scenarios {
		out[i] = scenarioSummary{
			ID: s.ID.String(), Title: s.Title, Summary: s.Summary,
			Difficulty: string(s.Difficulty), Status: string(s.Status),
			TeamCount: s.TeamCount, TraineeCount: s.TraineeCount, PhaseCount: len(s.Phases),
			EstDurationSec: s.EstDurationSec, UpdatedAt: s.UpdatedAt, PublishedAt: s.PublishedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{"scenarios": out})
}

func (h *scenarioHandler) create(c *gin.Context) {
	var req createScenarioRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	s, issues, err := h.scenarios.Create(c.Request.Context(), actor, req.Title)
	h.respond(c, http.StatusCreated, s, issues, err)
}

func (h *scenarioHandler) get(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	actor, _ := currentUser(c)
	s, issues, err := h.scenarios.Get(c.Request.Context(), actor, id)
	h.respond(c, http.StatusOK, s, issues, err)
}

func (h *scenarioHandler) save(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req saveScenarioRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	s, issues, err := h.scenarios.Save(c.Request.Context(), actor, id, req.Revision, req.toScenario())
	h.respond(c, http.StatusOK, s, issues, err)
}

func (h *scenarioHandler) publish(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req publishScenarioRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	s, issues, err := h.scenarios.Publish(c.Request.Context(), actor, id, req.ConfirmFictional)
	h.respond(c, http.StatusOK, s, issues, err)
}

func (h *scenarioHandler) unpublish(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	actor, _ := currentUser(c)
	s, issues, err := h.scenarios.Unpublish(c.Request.Context(), actor, id)
	h.respond(c, http.StatusOK, s, issues, err)
}

func (h *scenarioHandler) delete(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	actor, _ := currentUser(c)
	if err := h.scenarios.Delete(c.Request.Context(), actor, id); err != nil {
		_ = c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
