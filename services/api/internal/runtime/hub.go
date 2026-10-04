// Package runtime hosts live exercises: one engine per running session, the
// loop that advances it, and the subscribers watching it.
package runtime

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	"fogline/api/internal/engine"
)

// sendBuffer is how many undelivered frames a client may accumulate before it
// is cut off. A slow client must never hold up the exercise.
const sendBuffer = 256

// Viewer is who is on the other end of a subscription. An instructor sees the
// monitoring stream; a trainee sees what is addressed to them.
type Viewer struct {
	Instructor  bool
	Participant engine.Participant
}

// Client is one subscription. Frames arrive on Send; it is closed when the
// client is dropped or unsubscribed.
type Client struct {
	viewer Viewer
	send   chan []byte
	closed bool
}

func (c *Client) Send() <-chan []byte { return c.send }

// Envelope is the frame every realtime message is wrapped in.
type Envelope struct {
	Type    string `json:"type"`
	SimMs   int64  `json:"simMs"`
	Payload any    `json:"payload"`
}

// Hub fans notifications out to the subscribers entitled to them. Who gets
// what is decided here, on the server, from the notification's audience; a
// client cannot ask for more.
type Hub struct {
	mu      sync.Mutex
	clients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

// Subscribe registers a viewer. If first is non-nil it is queued ahead of
// anything broadcast afterwards.
func (h *Hub) Subscribe(v Viewer, first *Envelope) *Client {
	c := &Client{viewer: v, send: make(chan []byte, sendBuffer)}
	if first != nil {
		if frame, err := json.Marshal(first); err == nil {
			c.send <- frame
		}
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop(c)
}

func (h *Hub) drop(c *Client) {
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
	}
	if !c.closed {
		c.closed = true
		close(c.send)
	}
}

// SetRoster refreshes each trainee subscriber's team from the roster the
// exercise is starting with, since teams can be reassigned in the lobby after
// a client has connected. Trainees no longer on the roster are disconnected.
func (h *Hub) SetRoster(roster []engine.Participant) {
	byID := make(map[uuid.UUID]engine.Participant, len(roster))
	for _, p := range roster {
		byID[p.ID] = p
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.viewer.Instructor {
			continue
		}
		p, ok := byID[c.viewer.Participant.ID]
		if !ok {
			h.drop(c)
			continue
		}
		c.viewer.Participant = p
	}
}

func entitled(v Viewer, a engine.Audience) bool {
	if v.Instructor {
		return a.Instructor
	}
	// A trainee never receives an instructor-addressed notification, even one
	// that also names trainees.
	if a.Instructor && !a.AllTrainees {
		return false
	}
	return a.Includes(v.Participant)
}

// Broadcast delivers each notification to the subscribers it is addressed to.
func (h *Hub) Broadcast(notes []engine.Notification) {
	if len(notes) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.clients) == 0 {
		return
	}
	for _, n := range notes {
		var frame []byte // marshalled once, and only if someone is entitled
		for c := range h.clients {
			if !entitled(c.viewer, n.Audience) {
				continue
			}
			if frame == nil {
				var err error
				frame, err = json.Marshal(Envelope{Type: n.Type, SimMs: n.SimMs, Payload: n.Payload})
				if err != nil {
					break
				}
			}
			select {
			case c.send <- frame:
			default:
				h.drop(c) // too far behind: it can reconnect and resync
			}
		}
	}
}

// Close disconnects everyone.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		h.drop(c)
	}
}

func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}
