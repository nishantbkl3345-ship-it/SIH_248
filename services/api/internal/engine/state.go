package engine

import (
	"time"

	"github.com/google/uuid"
)

// delivery is one item addressed to one team, with what became of it.
type delivery struct {
	ID          uuid.UUID
	Message     bool // a trainee message rather than a scenario report
	ItemID      uuid.UUID
	Team        int
	ChannelID   uuid.UUID
	Title       string
	Source      string
	Reliability string
	Priority    string
	Content     string // as delivered
	SenderID    uuid.UUID
	GeneratedMs SimMs
	DeliveredMs SimMs
	State       DeliveryState
	DelayMs     int64
	Applied     []uuid.UUID
	Conflict    *uuid.UUID
}

type openDecision struct {
	openedMs   SimMs
	openedWall time.Time
}

// decisionKey identifies who a decision belongs to: a team for team-scope
// points, a participant for individual ones.
type decisionKey struct {
	point       uuid.UUID
	team        int
	participant uuid.UUID
}

// State is everything that changes while an exercise runs. Collections that
// are iterated are slices in a defined order; maps are used for lookup only.
type State struct {
	status     Status
	phaseIndex int // -1 before the first phase starts

	activeRules []int // indexes into Definition.Rules, ascending

	openDecisions map[uuid.UUID]openDecision
	decided       map[decisionKey]bool
	decisions     []DecisionRecord

	inbox   map[int][]delivery       // delivered items, per team, in arrival order
	sent    map[uuid.UUID][]delivery // each participant's own messages, as written
	pending map[int]int              // reports in transit, per team
	dropped map[int]int              // reports lost, per team

	idCounter uint64
}

func newState() State {
	return State{
		status:        StatusIdle,
		phaseIndex:    -1,
		openDecisions: map[uuid.UUID]openDecision{},
		decided:       map[decisionKey]bool{},
		inbox:         map[int][]delivery{},
		sent:          map[uuid.UUID][]delivery{},
		pending:       map[int]int{},
		dropped:       map[int]int{},
	}
}
