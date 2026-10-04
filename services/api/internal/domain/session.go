package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Runtime-side entities: what is created and recorded while an exercise runs.
// Simulation time is in milliseconds since the exercise started.

var (
	ErrAlreadyJoined = errors.New("already a participant")
	ErrJoinCodeTaken = errors.New("join code already in use")
)

type SessionStatus string

const (
	SessionLobby    SessionStatus = "LOBBY"
	SessionBriefing SessionStatus = "BRIEFING"
	SessionRunning  SessionStatus = "RUNNING"
	SessionPaused   SessionStatus = "PAUSED"
	SessionEnded    SessionStatus = "ENDED"
)

type ExerciseSession struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	ScenarioID   uuid.UUID `gorm:"type:uuid"`
	InstructorID uuid.UUID `gorm:"type:uuid"`
	Name         string
	JoinCode     string
	Seed         int64 // drives every seeded degradation outcome
	Status       SessionStatus
	SpeedMilli   int   // thousandths: 1000 is real time
	SimMs        int64 // last persisted simulation time
	StartedAt    *time.Time
	EndedAt      *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time

	Teams        []Team               `gorm:"foreignKey:SessionID"`
	Participants []SessionParticipant `gorm:"foreignKey:SessionID"`
}

type Team struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID uuid.UUID `gorm:"type:uuid"`
	Number    int       // 1-based; what scenario content refers to
	Name      string
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SessionParticipant struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID uuid.UUID  `gorm:"type:uuid"`
	UserID    uuid.UUID  `gorm:"type:uuid"`
	TeamID    *uuid.UUID `gorm:"type:uuid"`
	Seat      string
	IsReady   bool
	JoinedAt  time.Time `gorm:"autoCreateTime"`
	CreatedAt time.Time
	UpdatedAt time.Time

	User *User `gorm:"foreignKey:UserID"`
}

type Message struct {
	ID                  uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID           uuid.UUID `gorm:"type:uuid"`
	ChannelID           uuid.UUID `gorm:"type:uuid"`
	SenderParticipantID uuid.UUID `gorm:"type:uuid"`
	TeamNo              int
	Body                string
	SimMs               int64
	CreatedAt           time.Time
}

// Decision is immutable once recorded. Snapshot holds what the trainee had
// available at that moment and is built by the server, never by the client.
type Decision struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID       uuid.UUID `gorm:"type:uuid"`
	DecisionPointID uuid.UUID `gorm:"type:uuid"`
	OptionID        uuid.UUID `gorm:"type:uuid"`
	ParticipantID   uuid.UUID `gorm:"type:uuid"`
	TeamNo          int
	Rationale       string
	Confidence      *int16
	SimMs           int64
	WallTime        time.Time
	ResponseMs      int64 // simulation time since the decision point opened
	ResponseWallMs  int64 // real time since the decision point opened
	Snapshot        JSONB
	CreatedAt       time.Time
}

// TimelineEvent is one entry in the append-only record of what actually
// happened in a session, ordered by Seq. The database refuses UPDATE and
// DELETE on this table.
type TimelineEvent struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID          uuid.UUID `gorm:"type:uuid"`
	Seq                int64
	SimMs              int64
	WallTime           time.Time
	Type               string
	ActorParticipantID *uuid.UUID `gorm:"type:uuid"`
	TeamNo             int
	Payload            JSONB
	CreatedAt          time.Time
}

type AARStatus string

const (
	AARGenerating AARStatus = "GENERATING"
	AARReady      AARStatus = "READY"
	AARFailed     AARStatus = "FAILED"
)

type AARReport struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	SessionID          uuid.UUID `gorm:"type:uuid"`
	Status             AARStatus
	Content            JSONB
	LessonsLearned     string
	ReleasedToTrainees bool
	GeneratedAt        *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (AARReport) TableName() string { return "aar_reports" }
