package engine

import (
	"encoding/json"

	"fogline/api/internal/domain"
)

// FromScenario converts an authored scenario into the engine's script:
// phase-relative seconds become absolute milliseconds, and windows are cut
// off at the end of their phase.
func FromScenario(s *domain.Scenario) Definition {
	def := Definition{TeamCount: s.TeamCount}
	for _, c := range s.Channels {
		def.Channels = append(def.Channels, Channel{
			ID: c.ID, Name: c.Name, Kind: ChannelKind(c.Kind), BaseLatencyMs: int64(c.BaseLatencySec) * 1000,
		})
	}

	var start SimMs
	for _, p := range s.Phases {
		end := start + int64(p.DurationSec)*1000
		def.Phases = append(def.Phases, Phase{ID: p.ID, Title: p.Title, StartMs: start, EndMs: end})
		at := func(offsetSec int) SimMs { return min(start+int64(offsetSec)*1000, end) }

		for _, r := range p.Reports {
			rep := Report{
				ID: r.ID, AtMs: at(r.OffsetSec), Title: r.Title, Source: r.SourceName,
				Reliability: r.SourceReliability, Priority: string(r.Priority), Content: r.Body,
			}
			if r.ChannelID != nil {
				rep.ChannelID = *r.ChannelID
			}
			if r.TargetTeam != nil {
				rep.Team = *r.TargetTeam
			}
			def.Reports = append(def.Reports, rep)
		}

		for _, r := range p.Rules {
			var params domain.RuleParams
			if len(r.Params) > 0 {
				_ = json.Unmarshal(r.Params, &params)
			}
			rule := Rule{
				ID: r.ID, Name: r.Name, Mode: RuleMode(r.Mode), StartMs: at(r.OffsetSec),
				ChannelID: r.ChannelID, DelayMs: int64(params.DelaySec) * 1000, KeepPercent: params.KeepPercent,
			}
			rule.EndMs = at(r.OffsetSec + r.DurationSec)
			if r.TargetTeam != nil {
				rule.Team = *r.TargetTeam
			}
			if rule.Mode == ModeConflicting {
				rule.EndMs = rule.StartMs
				rule.Conflict = &Conflict{
					Topic: params.Topic, SourceA: params.SourceA, ContentA: params.ContentA,
					SourceB: params.SourceB, ContentB: params.ContentB,
				}
			}
			def.Rules = append(def.Rules, rule)
		}

		for _, d := range p.DecisionPoints {
			point := DecisionPoint{
				ID: d.ID, OpenMs: at(d.OffsetSec), Scope: DecisionScope(d.Scope),
				Question: d.Prompt, RationaleRequired: d.RationaleRequired,
			}
			point.CloseMs = point.OpenMs + int64(d.TimeLimitSec)*1000
			for _, o := range d.Options {
				point.Options = append(point.Options, DecisionOption{ID: o.ID, Label: o.Label, Description: o.Description})
			}
			def.DecisionPoints = append(def.DecisionPoints, point)
		}
		start = end
	}
	def.TotalMs = start
	// A deadline cannot outlive the exercise.
	for i := range def.DecisionPoints {
		def.DecisionPoints[i].CloseMs = min(def.DecisionPoints[i].CloseMs, def.TotalMs)
	}
	return def
}
