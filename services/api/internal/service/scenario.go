package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/repository"
)

// ScenarioService is the authoring use-case. A scenario is edited as a whole
// document while it is a DRAFT and becomes read-only once PUBLISHED.
type ScenarioService struct {
	repo repository.ScenarioRepository
	now  func() time.Time
}

func NewScenarioService(repo repository.ScenarioRepository) *ScenarioService {
	return &ScenarioService{repo: repo, now: time.Now}
}

var errScenarioNotFound = apperr.NotFound("Scenario not found.")

// Create starts a draft with sensible defaults so the builder never opens on
// an empty page.
func (s *ScenarioService) Create(ctx context.Context, actor *domain.User, title string) (*domain.Scenario, []Issue, error) {
	id := uuid.New()
	phaseID := uuid.New()
	sc := &domain.Scenario{
		ID:           id,
		OwnerID:      actor.ID,
		Title:        strings.TrimSpace(title),
		Difficulty:   domain.DifficultyIntermediate,
		Status:       domain.ScenarioDraft,
		TeamCount:    2,
		TraineeCount: 8,
		Revision:     1,
		Objectives:   domain.JSONB(`[]`),
		Channels: []domain.CommunicationChannel{
			{ID: uuid.New(), Name: "Team Net", Kind: domain.ChannelTeam},
			{ID: uuid.New(), Name: "Coordination Net", Kind: domain.ChannelCrossTeam},
			{ID: uuid.New(), Name: "Field Feed", Kind: domain.ChannelFeed},
		},
		Phases: []domain.ScenarioPhase{
			{ID: phaseID, Title: "Phase 1", DurationSec: 300},
		},
	}
	normalize(sc)
	if err := s.repo.Create(ctx, sc); err != nil {
		return nil, nil, apperr.Internal(err)
	}
	return sc, ReviewScenario(sc), nil
}

// List returns the scenarios the actor may see: their own, or everyone's for
// an admin.
func (s *ScenarioService) List(ctx context.Context, actor *domain.User) ([]domain.Scenario, error) {
	var owner *uuid.UUID
	if actor.Role != domain.RoleAdmin {
		owner = &actor.ID
	}
	out, err := s.repo.List(ctx, owner)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	return out, nil
}

func (s *ScenarioService) Get(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.Scenario, []Issue, error) {
	sc, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, nil, err
	}
	return sc, ReviewScenario(sc), nil
}

// Save replaces a draft's content. Incomplete drafts are accepted; what is
// still missing comes back as issues. expectedRevision must be the revision
// the caller last read.
func (s *ScenarioService) Save(ctx context.Context, actor *domain.User, id uuid.UUID, expectedRevision int, draft *domain.Scenario) (*domain.Scenario, []Issue, error) {
	cur, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, nil, err
	}
	if cur.Status != domain.ScenarioDraft {
		return nil, nil, errNotDraft
	}

	draft.ID = cur.ID
	draft.OwnerID = cur.OwnerID
	draft.Status = cur.Status
	draft.CreatedAt = cur.CreatedAt
	draft.PublishedAt = cur.PublishedAt

	if details := checkStructure(draft); len(details) > 0 {
		return nil, nil, apperr.Validation(details)
	}
	normalize(draft)

	switch err := s.repo.Replace(ctx, draft, expectedRevision); {
	case err == nil:
	case errors.Is(err, domain.ErrRevisionConflict):
		return nil, nil, apperr.Conflict("REVISION_CONFLICT",
			"This scenario was changed somewhere else. Reload to get the latest version.")
	case errors.Is(err, domain.ErrNotDraft):
		return nil, nil, errNotDraft
	case errors.Is(err, domain.ErrIDConflict):
		return nil, nil, apperr.Conflict("ID_CONFLICT", "An item id is already in use. Reload and try again.")
	case errors.Is(err, domain.ErrNotFound):
		return nil, nil, errScenarioNotFound
	default:
		return nil, nil, apperr.Internal(err)
	}
	return draft, ReviewScenario(draft), nil
}

var errNotDraft = apperr.Conflict("NOT_DRAFT", "A published scenario cannot be edited. Revert it to a draft first.")

// Publish freezes a draft once it has no outstanding errors.
// confirmFictional is the instructor's statement that the content is fictional.
func (s *ScenarioService) Publish(ctx context.Context, actor *domain.User, id uuid.UUID, confirmFictional bool) (*domain.Scenario, []Issue, error) {
	sc, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, nil, err
	}
	if sc.Status != domain.ScenarioDraft {
		return nil, nil, apperr.Conflict("NOT_DRAFT", "Only a draft can be published.")
	}
	if !confirmFictional {
		return nil, nil, apperr.Validation(map[string]string{
			"confirmFictional": "Confirm that every place, entity and event in this scenario is fictional.",
		})
	}
	if issues := ReviewScenario(sc); HasErrors(issues) {
		details := map[string]string{}
		for _, i := range issues {
			if i.Severity == SeverityError {
				if _, dup := details[i.Path]; !dup {
					details[i.Path] = i.Message
				}
			}
		}
		e := apperr.New(422, "NOT_READY", "The scenario is not ready to publish. Resolve the outstanding issues first.")
		e.Details = details
		return nil, nil, e
	}

	now := s.now()
	if err := s.repo.SetStatus(ctx, id, domain.ScenarioDraft, domain.ScenarioPublished, &now); err != nil {
		return nil, nil, statusErr(err)
	}
	return s.Get(ctx, actor, id)
}

// Unpublish returns a published scenario to DRAFT so it can be edited. It is
// refused once exercise sessions depend on the scenario.
func (s *ScenarioService) Unpublish(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.Scenario, []Issue, error) {
	sc, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, nil, err
	}
	if sc.Status != domain.ScenarioPublished {
		return nil, nil, apperr.Conflict("NOT_PUBLISHED", "Only a published scenario can be reverted to a draft.")
	}
	if err := s.ensureUnused(ctx, id, "reverted to a draft"); err != nil {
		return nil, nil, err
	}
	if err := s.repo.SetStatus(ctx, id, domain.ScenarioPublished, domain.ScenarioDraft, nil); err != nil {
		return nil, nil, statusErr(err)
	}
	return s.Get(ctx, actor, id)
}

func (s *ScenarioService) Delete(ctx context.Context, actor *domain.User, id uuid.UUID) error {
	if _, err := s.load(ctx, actor, id); err != nil {
		return err
	}
	if err := s.ensureUnused(ctx, id, "deleted"); err != nil {
		return err
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return errScenarioNotFound
		}
		return apperr.Internal(err)
	}
	return nil
}

func (s *ScenarioService) ensureUnused(ctx context.Context, id uuid.UUID, verb string) error {
	n, err := s.repo.CountSessions(ctx, id)
	if err != nil {
		return apperr.Internal(err)
	}
	if n > 0 {
		return apperr.Conflict("SCENARIO_IN_USE",
			fmt.Sprintf("This scenario has been used in %d exercise session(s) and cannot be %s.", n, verb))
	}
	return nil
}

// load fetches a scenario the actor may access. Someone else's scenario is
// reported as not found, so ids cannot be probed.
func (s *ScenarioService) load(ctx context.Context, actor *domain.User, id uuid.UUID) (*domain.Scenario, error) {
	sc, err := s.repo.Get(ctx, id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, errScenarioNotFound
	}
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if actor.Role != domain.RoleAdmin && sc.OwnerID != actor.ID {
		return nil, errScenarioNotFound
	}
	return sc, nil
}

func statusErr(err error) error {
	if errors.Is(err, domain.ErrRevisionConflict) {
		return apperr.Conflict("REVISION_CONFLICT",
			"This scenario was changed somewhere else. Reload to get the latest version.")
	}
	return apperr.Internal(err)
}

// checkStructure rejects documents that cannot be stored coherently: ids
// reused inside the document and references to channels that do not exist.
// Field formats and limits are enforced by the transport layer.
func checkStructure(s *domain.Scenario) map[string]string {
	details := map[string]string{}
	seen := map[uuid.UUID]bool{}
	claim := func(path string, id uuid.UUID) {
		if seen[id] {
			details[path] = "is used by more than one item"
		}
		seen[id] = true
	}

	channels := map[uuid.UUID]bool{}
	for i, c := range s.Channels {
		claim(fmt.Sprintf("channels[%d].id", i), c.ID)
		channels[c.ID] = true
	}
	checkChannel := func(path string, id *uuid.UUID) {
		if id != nil && !channels[*id] {
			details[path] = "refers to a channel that does not exist"
		}
	}

	for i, p := range s.Phases {
		base := fmt.Sprintf("phases[%d]", i)
		claim(base+".id", p.ID)
		for j, r := range p.Reports {
			claim(fmt.Sprintf("%s.reports[%d].id", base, j), r.ID)
			checkChannel(fmt.Sprintf("%s.reports[%d].channelId", base, j), r.ChannelID)
		}
		for j, r := range p.Rules {
			claim(fmt.Sprintf("%s.rules[%d].id", base, j), r.ID)
			checkChannel(fmt.Sprintf("%s.rules[%d].channelId", base, j), r.ChannelID)
		}
		for j, d := range p.DecisionPoints {
			claim(fmt.Sprintf("%s.decisionPoints[%d].id", base, j), d.ID)
			for k, o := range d.Options {
				claim(fmt.Sprintf("%s.decisionPoints[%d].options[%d].id", base, j, k), o.ID)
			}
		}
	}
	return details
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// normalize fills in everything derived from the document: parent ids,
// ordering, phase start times, the total duration and channel keys.
func normalize(s *domain.Scenario) {
	s.Title = strings.TrimSpace(s.Title)
	s.Summary = strings.TrimSpace(s.Summary)

	objectives := []string{}
	for _, o := range Objectives(s) {
		objectives = append(objectives, strings.TrimSpace(o))
	}
	raw, _ := json.Marshal(objectives)
	s.Objectives = domain.JSONB(raw)

	keys := map[string]bool{}
	for i := range s.Channels {
		c := &s.Channels[i]
		c.ScenarioID = s.ID
		c.Ord = i + 1
		c.Name = strings.TrimSpace(c.Name)
		key := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(c.Name), "-"), "-")
		if key == "" {
			key = "channel"
		}
		// Keys are unique per scenario even when names are not (yet).
		for n, base := 2, key; keys[key]; n++ {
			key = fmt.Sprintf("%s-%d", base, n)
		}
		keys[key] = true
		c.Key = key
	}

	start := 0
	for i := range s.Phases {
		p := &s.Phases[i]
		p.ScenarioID = s.ID
		p.Ord = i + 1
		p.Title = strings.TrimSpace(p.Title)
		p.StartsAtSim = start
		start += p.DurationSec

		for j := range p.Reports {
			p.Reports[j].ScenarioID, p.Reports[j].PhaseID, p.Reports[j].Ord = s.ID, p.ID, j+1
		}
		for j := range p.Rules {
			p.Rules[j].ScenarioID, p.Rules[j].PhaseID, p.Rules[j].Ord = s.ID, p.ID, j+1
		}
		for j := range p.DecisionPoints {
			d := &p.DecisionPoints[j]
			d.ScenarioID, d.PhaseID, d.Ord = s.ID, p.ID, j+1
			for k := range d.Options {
				d.Options[k].DecisionPointID = d.ID
				d.Options[k].Ord = k + 1
			}
		}
	}
	s.EstDurationSec = start
}
