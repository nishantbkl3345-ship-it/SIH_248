package engine

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Recorder is the append-only exercise timeline. Entries get consecutive
// sequence numbers and can never be changed or removed: there is no method
// that does so, payloads are serialised at the moment of recording, and
// readers are handed copies.
type Recorder struct {
	events []Event
}

func (r *Recorder) append(sim SimMs, wall time.Time, typ EventType, team int, participant *uuid.UUID, payload any) Event {
	raw, err := json.Marshal(payload)
	if err != nil {
		// Payloads are plain structs defined in this package.
		panic("engine: unserialisable event payload: " + err.Error())
	}
	e := Event{
		Seq:           int64(len(r.events)) + 1,
		SimMs:         sim,
		Wall:          wall,
		Type:          typ,
		Team:          team,
		ParticipantID: participant,
		Payload:       raw,
	}
	r.events = append(r.events, e)
	return e.clone()
}

func (r *Recorder) Len() int { return len(r.events) }

// Since returns copies of every event with a sequence number above afterSeq.
func (r *Recorder) Since(afterSeq int64) []Event {
	if afterSeq < 0 {
		afterSeq = 0
	}
	if int(afterSeq) >= len(r.events) {
		return nil
	}
	out := make([]Event, 0, len(r.events)-int(afterSeq))
	for _, e := range r.events[afterSeq:] {
		out = append(out, e.clone())
	}
	return out
}
