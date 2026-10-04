package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Authoring-side entities: what an instructor writes before an exercise runs.
// All scenario content is fictional by project rule.
//
// Reports, rules and decision points belong to a phase and are timed as an
// offset from the start of that phase.

var (
	// ErrRevisionConflict means the scenario was saved by someone else since
	// the caller last read it.
	ErrRevisionConflict = errors.New("scenario revision conflict")
	// ErrIDConflict means a supplied entity id already exists elsewhere.
	ErrIDConflict = errors.New("entity id already in use")
	// ErrNotDraft means the operation needs a scenario in DRAFT status.
	ErrNotDraft = errors.New("scenario is not a draft")
)

type ScenarioStatus string

const (
	ScenarioDraft     ScenarioStatus = "DRAFT"
	ScenarioPublished ScenarioStatus = "PUBLISHED"
	ScenarioArchived  ScenarioStatus = "ARCHIVED"
)

type Difficulty string

const (
	DifficultyBasic        Difficulty = "BASIC"
	DifficultyIntermediate Difficulty = "INTERMEDIATE"
	DifficultyAdvanced     Difficulty = "ADVANCED"
)

type Scenario struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OwnerID        uuid.UUID `gorm:"type:uuid"`
	Title          string
	Summary        string
	Objectives     JSONB // string[]
	Difficulty     Difficulty
	Status         ScenarioStatus
	EstDurationSec int // sum of phase durations, maintained on save
	TeamCount      int
	TraineeCount   int
	Revision       int
	PublishedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time

	Channels []CommunicationChannel
	Phases   []ScenarioPhase
}

type ScenarioPhase struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID  uuid.UUID `gorm:"type:uuid"`
	Ord         int
	Title       string
	Description string
	DurationSec int
	StartsAtSim int // simulation seconds from exercise start, maintained on save
	CreatedAt   time.Time
	UpdatedAt   time.Time

	Reports        []InformationReport `gorm:"foreignKey:PhaseID"`
	Rules          []DegradationRule   `gorm:"foreignKey:PhaseID"`
	DecisionPoints []DecisionPoint     `gorm:"foreignKey:PhaseID"`
}

type ChannelKind string

const (
	ChannelTeam      ChannelKind = "TEAM"
	ChannelCrossTeam ChannelKind = "CROSS_TEAM"
	ChannelBroadcast ChannelKind = "BROADCAST"
	ChannelFeed      ChannelKind = "FEED"
)

type CommunicationChannel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID     uuid.UUID `gorm:"type:uuid"`
	Ord            int
	Key            string
	Name           string
	Kind           ChannelKind
	BaseLatencySec int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type ReportPriority string

const (
	PriorityLow      ReportPriority = "LOW"
	PriorityRoutine  ReportPriority = "ROUTINE"
	PriorityHigh     ReportPriority = "HIGH"
	PriorityCritical ReportPriority = "CRITICAL"
)

// InformationReport is an authored, synthetic piece of information released
// into an exercise.
type InformationReport struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID        uuid.UUID  `gorm:"type:uuid"`
	PhaseID           uuid.UUID  `gorm:"type:uuid"`
	ChannelID         *uuid.UUID `gorm:"type:uuid"` // unset only while drafting
	Ord               int
	SourceName        string
	SourceReliability string // A (most reliable) to E
	Title             string
	Body              string
	OffsetSec         int
	Priority          ReportPriority
	TargetTeam        *int // nil addresses every team
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TriggerKind string

const (
	TriggerScheduled   TriggerKind = "SCHEDULED"
	TriggerConditional TriggerKind = "CONDITIONAL"
	TriggerManual      TriggerKind = "MANUAL"
)

// ScenarioEvent is reserved for conditional and instructor-triggered events,
// which arrive with the event engine. Scheduled timeline events are derived
// from reports, rules, decision points and phase boundaries instead.
type ScenarioEvent struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID  uuid.UUID  `gorm:"type:uuid"`
	PhaseID     *uuid.UUID `gorm:"type:uuid"`
	Name        string
	TriggerKind TriggerKind
	AtSim       *int
	Condition   JSONB
	Actions     JSONB
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type DegradationMode string

const (
	// ModeNormal forces nominal communications for its window.
	ModeNormal DegradationMode = "NORMAL"
	// ModeDelay holds information back by a fixed time.
	ModeDelay DegradationMode = "DELAY"
	// ModeDropout makes information unavailable for its window.
	ModeDropout DegradationMode = "DROPOUT"
	// ModePartial delivers only part of each item.
	ModePartial DegradationMode = "PARTIAL"
	// ModeConflicting releases two contradictory synthetic reports.
	ModeConflicting DegradationMode = "CONFLICTING"
)

// RuleParams is the mode-specific part of a DegradationRule, stored as JSONB.
// Only the fields for the rule's mode are meaningful.
type RuleParams struct {
	DelaySec    int    `json:"delaySec,omitempty"`    // DELAY
	KeepPercent int    `json:"keepPercent,omitempty"` // PARTIAL: share of content delivered
	Topic       string `json:"topic,omitempty"`       // CONFLICTING
	SourceA     string `json:"sourceA,omitempty"`
	ContentA    string `json:"contentA,omitempty"`
	SourceB     string `json:"sourceB,omitempty"`
	ContentB    string `json:"contentB,omitempty"`
}

// DegradationRule is a communication rule in force for a window of a phase.
type DegradationRule struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID  uuid.UUID  `gorm:"type:uuid"`
	PhaseID     uuid.UUID  `gorm:"type:uuid"`
	ChannelID   *uuid.UUID `gorm:"type:uuid"` // nil applies to every channel
	TargetTeam  *int       // nil applies to every team
	Ord         int
	Name        string
	Mode        DegradationMode
	OffsetSec   int
	DurationSec int
	Params      JSONB // RuleParams
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type DecisionScope string

const (
	ScopeIndividual DecisionScope = "INDIVIDUAL"
	ScopeTeam       DecisionScope = "TEAM"
)

// DecisionPoint is the authored question; DecisionOption its possible answers;
// Decision (session.go) is a trainee's recorded answer. It opens at OffsetSec
// and closes TimeLimitSec later.
type DecisionPoint struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID         uuid.UUID `gorm:"type:uuid"`
	PhaseID            uuid.UUID `gorm:"type:uuid"`
	Ord                int
	Scope              DecisionScope
	Prompt             string
	OffsetSec          int
	TimeLimitSec       int
	RationaleRequired  bool
	EvaluationCriteria string
	CreatedAt          time.Time
	UpdatedAt          time.Time

	Options []DecisionOption
}

type DecisionOption struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	DecisionPointID uuid.UUID `gorm:"type:uuid"`
	Ord             int
	Label           string
	Description     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
