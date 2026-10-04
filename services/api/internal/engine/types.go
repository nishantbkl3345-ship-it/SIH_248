// Package engine is the simulation core: it turns a published scenario and a
// roster into a deterministic stream of recorded events.
//
// The engine knows nothing about HTTP, WebSockets or the database, and never
// reads the system clock or a random source. Every entry point takes the wall
// time as an argument, and every "random" choice is a keyed hash of the
// session seed, so the same inputs always produce the same timeline.
//
// It is not safe for concurrent use; the caller serialises access.
package engine

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SimMs is simulation time in milliseconds since the exercise started.
type SimMs = int64

type Status string

const (
	StatusIdle    Status = "IDLE"
	StatusRunning Status = "RUNNING"
	StatusPaused  Status = "PAUSED"
	StatusEnded   Status = "ENDED"
)

// DeliveryState is what happened to one item on its way to one team.
type DeliveryState string

const (
	StateNormal        DeliveryState = "NORMAL"
	StateDelayed       DeliveryState = "DELAYED"
	StateDropped       DeliveryState = "DROPPED"
	StatePartial       DeliveryState = "PARTIAL"
	StateContradictory DeliveryState = "CONTRADICTORY"
)

type EventType string

const (
	EvExerciseStarted       EventType = "EXERCISE_STARTED"
	EvExercisePaused        EventType = "EXERCISE_PAUSED"
	EvExerciseResumed       EventType = "EXERCISE_RESUMED"
	EvExerciseEnded         EventType = "EXERCISE_ENDED"
	EvSpeedChanged          EventType = "SPEED_CHANGED"
	EvPhaseStarted          EventType = "PHASE_STARTED"
	EvPhaseCompleted        EventType = "PHASE_COMPLETED"
	EvReportGenerated       EventType = "REPORT_GENERATED"
	EvReportDelivered       EventType = "REPORT_DELIVERED"
	EvReportDelayed         EventType = "REPORT_DELAYED"
	EvReportDropped         EventType = "REPORT_DROPPED"
	EvCommunicationDegraded EventType = "COMMUNICATION_DEGRADED"
	EvCommunicationRestored EventType = "COMMUNICATION_RESTORED"
	EvDecisionPointOpened   EventType = "DECISION_POINT_OPENED"
	EvDecisionPointClosed   EventType = "DECISION_POINT_CLOSED"
	EvDecisionSubmitted     EventType = "DECISION_SUBMITTED"
	EvMessageSent           EventType = "MESSAGE_SENT"
	EvMessageDelivered      EventType = "MESSAGE_DELIVERED"
	EvMessageDelayed        EventType = "MESSAGE_DELAYED"
	EvMessageDropped        EventType = "MESSAGE_DROPPED"
)

// Realtime notification names sent to clients.
const (
	NSimulationStarted   = "simulation.started"
	NSimulationPaused    = "simulation.paused"
	NSimulationResumed   = "simulation.resumed"
	NSimulationCompleted = "simulation.completed"
	NSimulationTime      = "simulation.time_update"
	NReportReceived      = "report.received"
	NDecisionOpened      = "decision.opened"
	NDecisionClosed      = "decision.closed"
	NDecisionSubmitted   = "decision.submitted"
	NMessageReceived     = "message.received"
	NPhaseChanged        = "phase.changed"
)

// instructorNames maps a recorded event to the name it carries on the
// instructor's monitoring stream.
var instructorNames = map[EventType]string{
	EvExerciseStarted:       NSimulationStarted,
	EvExercisePaused:        NSimulationPaused,
	EvExerciseResumed:       NSimulationResumed,
	EvExerciseEnded:         NSimulationCompleted,
	EvSpeedChanged:          "simulation.speed_changed",
	EvPhaseStarted:          NPhaseChanged,
	EvPhaseCompleted:        "phase.completed",
	EvReportGenerated:       "report.generated",
	EvReportDelivered:       NReportReceived,
	EvReportDelayed:         "report.delayed",
	EvReportDropped:         "report.dropped",
	EvCommunicationDegraded: "communication.degraded",
	EvCommunicationRestored: "communication.restored",
	EvDecisionPointOpened:   NDecisionOpened,
	EvDecisionPointClosed:   NDecisionClosed,
	EvDecisionSubmitted:     NDecisionSubmitted,
	EvMessageSent:           "message.sent",
	EvMessageDelivered:      NMessageReceived,
	EvMessageDelayed:        "message.delayed",
	EvMessageDropped:        "message.dropped",
}

// ------------------------------------------------------------- definition

// Definition is the immutable script the engine runs. All times are absolute
// simulation milliseconds. Team 0 means "every team"; a nil channel on a rule
// means "every channel".
type Definition struct {
	TeamCount      int
	TotalMs        SimMs
	Channels       []Channel
	Phases         []Phase
	Reports        []Report
	Rules          []Rule
	DecisionPoints []DecisionPoint
}

type ChannelKind string

const (
	ChannelTeam      ChannelKind = "TEAM"
	ChannelCrossTeam ChannelKind = "CROSS_TEAM"
	ChannelBroadcast ChannelKind = "BROADCAST"
	ChannelFeed      ChannelKind = "FEED"
)

type Channel struct {
	ID            uuid.UUID
	Name          string
	Kind          ChannelKind
	BaseLatencyMs int64
}

type Phase struct {
	ID      uuid.UUID
	Title   string
	StartMs SimMs
	EndMs   SimMs
}

type Report struct {
	ID          uuid.UUID
	AtMs        SimMs
	ChannelID   uuid.UUID
	Team        int
	Title       string
	Source      string
	Reliability string
	Priority    string
	Content     string
}

type RuleMode string

const (
	ModeNormal      RuleMode = "NORMAL"
	ModeDelay       RuleMode = "DELAY"
	ModeDropout     RuleMode = "DROPOUT"
	ModePartial     RuleMode = "PARTIAL"
	ModeConflicting RuleMode = "CONFLICTING"
)

type Rule struct {
	ID          uuid.UUID
	Name        string
	Mode        RuleMode
	StartMs     SimMs
	EndMs       SimMs // exclusive; equals StartMs for CONFLICTING
	ChannelID   *uuid.UUID
	Team        int
	DelayMs     int64
	KeepPercent int
	Conflict    *Conflict
}

// Conflict is two synthetic reports about the same subject that disagree.
// The engine never marks either as correct.
type Conflict struct {
	Topic    string
	SourceA  string
	ContentA string
	SourceB  string
	ContentB string
}

type DecisionScope string

const (
	ScopeIndividual DecisionScope = "INDIVIDUAL"
	ScopeTeam       DecisionScope = "TEAM"
)

type DecisionPoint struct {
	ID                uuid.UUID
	OpenMs            SimMs
	CloseMs           SimMs
	Scope             DecisionScope
	Question          string
	RationaleRequired bool
	Options           []DecisionOption
}

type DecisionOption struct {
	ID          uuid.UUID `json:"id"`
	Label       string    `json:"label"`
	Description string    `json:"description"`
}

// Participant is one trainee in the exercise.
type Participant struct {
	ID   uuid.UUID
	Name string
	Team int
}

// ----------------------------------------------------------------- output

// Audience says who may receive a notification. Instructors are addressed
// only by notifications with Instructor set; everything else is for trainees.
type Audience struct {
	Instructor   bool
	AllTrainees  bool
	Teams        []int
	Participants []uuid.UUID
	// Except excludes one participant from an otherwise matching audience
	// (the sender of a message does not receive it back).
	Except uuid.UUID
}

// Includes reports whether a trainee is in the audience.
func (a Audience) Includes(p Participant) bool {
	if p.ID == a.Except && a.Except != uuid.Nil {
		return false
	}
	if a.AllTrainees {
		return true
	}
	for _, t := range a.Teams {
		if t == p.Team {
			return true
		}
	}
	for _, id := range a.Participants {
		if id == p.ID {
			return true
		}
	}
	return false
}

// Event is one immutable entry in the exercise timeline.
type Event struct {
	Seq           int64           `json:"seq"`
	SimMs         SimMs           `json:"simMs"`
	Wall          time.Time       `json:"wallTime"`
	Type          EventType       `json:"type"`
	Team          int             `json:"team,omitempty"`
	ParticipantID *uuid.UUID      `json:"participantId,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

func (e Event) clone() Event {
	e.Payload = append(json.RawMessage(nil), e.Payload...)
	if e.ParticipantID != nil {
		id := *e.ParticipantID
		e.ParticipantID = &id
	}
	return e
}

// Notification is something to push to connected clients.
type Notification struct {
	Type     string
	SimMs    SimMs
	Audience Audience
	Payload  any
}

// DecisionRecord is a submitted decision together with what its author had
// available at that moment. It is built entirely by the engine.
type DecisionRecord struct {
	ID              uuid.UUID        `json:"id"`
	DecisionPointID uuid.UUID        `json:"decisionPointId"`
	OptionID        uuid.UUID        `json:"optionId"`
	ParticipantID   uuid.UUID        `json:"participantId"`
	Team            int              `json:"team"`
	Rationale       string           `json:"rationale"`
	Confidence      *int             `json:"confidence,omitempty"`
	SimMs           SimMs            `json:"simMs"`
	Wall            time.Time        `json:"wallTime"`
	ResponseMs      int64            `json:"responseMs"`     // simulation time since the point opened
	ResponseWallMs  int64            `json:"responseWallMs"` // real time since the point opened
	Snapshot        DecisionSnapshot `json:"snapshot"`
}

// DecisionSnapshot is the information picture at the moment of a decision.
type DecisionSnapshot struct {
	PhaseID uuid.UUID `json:"phaseId"`
	// Reports and Messages are what had actually reached the decider.
	Reports  []AvailableItem `json:"reports"`
	Messages []AvailableItem `json:"messages"`
	// ActiveDegradation is ground truth the decider could not see.
	ActiveDegradation []ActiveRule `json:"activeDegradation"`
	// Withheld counts items generated for the decider's team that had not
	// arrived: still in transit, or dropped.
	PendingReports int `json:"pendingReports"`
	DroppedReports int `json:"droppedReports"`
}

type AvailableItem struct {
	DeliveryID   uuid.UUID     `json:"deliveryId"`
	ItemID       uuid.UUID     `json:"itemId"`
	Source       string        `json:"source"`
	Title        string        `json:"title,omitempty"`
	State        DeliveryState `json:"state"`
	ReceivedAtMs SimMs         `json:"receivedAtMs"`
}

type ActiveRule struct {
	RuleID    uuid.UUID  `json:"ruleId"`
	Mode      RuleMode   `json:"mode"`
	ChannelID *uuid.UUID `json:"channelId"`
	Team      int        `json:"team"`
}

// MessageRecord is a message as its sender wrote it.
type MessageRecord struct {
	ID        uuid.UUID `json:"id"`
	ChannelID uuid.UUID `json:"channelId"`
	SenderID  uuid.UUID `json:"senderId"`
	Team      int       `json:"team"`
	Body      string    `json:"body"`
	SimMs     SimMs     `json:"simMs"`
	Wall      time.Time `json:"wallTime"`
}

// Output is everything one engine call produced, in order.
type Output struct {
	Events        []Event
	Notifications []Notification
	Decisions     []DecisionRecord
	Messages      []MessageRecord
}
