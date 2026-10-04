package engine

import (
	"encoding/binary"
	"hash/fnv"
	"strings"

	"github.com/google/uuid"
)

// Redacted replaces a word lost to partial delivery.
const Redacted = "▒▒▒"

// Intent is one item about to travel to one team.
type Intent struct {
	ItemID    uuid.UUID
	ChannelID uuid.UUID
	// Team is the recipient. SenderTeam is set for trainee messages: a rule
	// aimed at either end of the link applies.
	Team       int
	SenderTeam int
	Content    string
}

// Outcome is what the communication rules did to an Intent.
type Outcome struct {
	State   DeliveryState // NORMAL, DELAYED, DROPPED or PARTIAL
	DelayMs int64
	Content string      // as delivered; empty when dropped
	Applied []uuid.UUID // rules that affected the item, in definition order
}

func (r Rule) matches(in Intent) bool {
	if r.ChannelID != nil && *r.ChannelID != in.ChannelID {
		return false
	}
	return r.Team == 0 || r.Team == in.Team || (in.SenderTeam != 0 && r.Team == in.SenderTeam)
}

// Degrade applies the active communication rules to an item. It is a pure
// function: no clock, no shared state, no random source. active must be in
// definition order.
//
// Precedence, fixed:
//  1. NORMAL forces clear communications and overrides everything else.
//  2. DROPOUT loses the item.
//  3. PARTIAL removes words; the strictest matching rule wins.
//  4. DELAY holds the item back; matching delays add up.
//
// PARTIAL and DELAY combine: the item arrives late and incomplete, and is
// reported as PARTIAL with a non-zero delay.
func Degrade(seed int64, in Intent, active []Rule) Outcome {
	var (
		forced, dropped []uuid.UUID
		delayed         []uuid.UUID
		partial         *Rule
		delayMs         int64
	)
	for i := range active {
		r := &active[i]
		if !r.matches(in) {
			continue
		}
		switch r.Mode {
		case ModeNormal:
			forced = append(forced, r.ID)
		case ModeDropout:
			dropped = append(dropped, r.ID)
		case ModePartial:
			if partial == nil || r.KeepPercent < partial.KeepPercent {
				partial = r
			}
		case ModeDelay:
			delayed = append(delayed, r.ID)
			delayMs += r.DelayMs
		}
	}

	switch {
	case len(forced) > 0:
		return Outcome{State: StateNormal, Content: in.Content, Applied: forced}
	case len(dropped) > 0:
		return Outcome{State: StateDropped, Applied: dropped}
	}

	out := Outcome{State: StateNormal, Content: in.Content, DelayMs: delayMs}
	if partial != nil {
		out.State = StatePartial
		out.Content = redact(seed, *partial, in)
		out.Applied = append(out.Applied, partial.ID)
	} else if delayMs > 0 {
		out.State = StateDelayed
	}
	out.Applied = append(out.Applied, delayed...)
	return out
}

// redact keeps roughly KeepPercent of the words. Whether a given word
// survives depends only on (seed, rule, item, team, word position), so the
// result is repeatable and two teams can lose different words.
func redact(seed int64, rule Rule, in Intent) string {
	words := strings.Fields(in.Content)
	for i := range words {
		if keyedPercent(seed, rule.ID, in.ItemID, in.Team, i) >= rule.KeepPercent {
			words[i] = Redacted
		}
	}
	return strings.Join(words, " ")
}

// keyedPercent returns a value in [0, 100) determined entirely by its inputs.
func keyedPercent(seed int64, rule, item uuid.UUID, team, index int) int {
	h := fnv.New64a()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(seed))
	h.Write(buf[:])
	h.Write(rule[:])
	h.Write(item[:])
	binary.BigEndian.PutUint64(buf[:], uint64(team))
	h.Write(buf[:])
	binary.BigEndian.PutUint64(buf[:], uint64(index))
	h.Write(buf[:])
	return int(h.Sum64() % 100)
}
