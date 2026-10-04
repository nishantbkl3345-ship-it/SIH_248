package engine

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxMessageRunes = 2000

// ReportView is a report as a trainee sees it: content as it arrived, when it
// was sent and when it arrived. It carries no delivery state; whether the
// report is late, incomplete or contradicted is for the trainee to work out.
type ReportView struct {
	DeliveryID   uuid.UUID `json:"deliveryId"`
	Title        string    `json:"title"`
	Source       string    `json:"source"`
	Reliability  string    `json:"reliability,omitempty"`
	Priority     string    `json:"priority"`
	Content      string    `json:"content"`
	ChannelID    uuid.UUID `json:"channelId"`
	SentAtMs     SimMs     `json:"sentAtMs"`
	ReceivedAtMs SimMs     `json:"receivedAtMs"`
}

// MessageView is a trainee message as a recipient (or its sender) sees it.
type MessageView struct {
	ID           uuid.UUID `json:"id"`
	ChannelID    uuid.UUID `json:"channelId"`
	SenderID     uuid.UUID `json:"senderId"`
	SenderName   string    `json:"senderName"`
	Body         string    `json:"body"`
	SentAtMs     SimMs     `json:"sentAtMs"`
	ReceivedAtMs SimMs     `json:"receivedAtMs"`
}

func (d delivery) reportView() ReportView {
	return ReportView{
		DeliveryID: d.ID, Title: d.Title, Source: d.Source, Reliability: d.Reliability,
		Priority: d.Priority, Content: d.Content, ChannelID: d.ChannelID,
		SentAtMs: d.GeneratedMs, ReceivedAtMs: d.DeliveredMs,
	}
}

func (e *Engine) messageView(d delivery) MessageView {
	return MessageView{
		ID: d.ID, ChannelID: d.ChannelID, SenderID: d.SenderID, SenderName: e.byID[d.SenderID].Name,
		Body: d.Content, SentAtMs: d.GeneratedMs, ReceivedAtMs: d.DeliveredMs,
	}
}

// generatedPayload records an item at the moment it enters the exercise, with
// its original content.
type generatedPayload struct {
	ItemID    uuid.UUID  `json:"itemId"`
	ChannelID uuid.UUID  `json:"channelId"`
	Teams     []int      `json:"teams"`
	Title     string     `json:"title,omitempty"`
	Source    string     `json:"source,omitempty"`
	Content   string     `json:"content"`
	Conflict  *uuid.UUID `json:"conflictGroup,omitempty"`
}

// deliveryPayload records what happened to an item on its way to one team.
type deliveryPayload struct {
	DeliveryID  uuid.UUID     `json:"deliveryId"`
	ItemID      uuid.UUID     `json:"itemId"`
	Team        int           `json:"team"`
	ChannelID   uuid.UUID     `json:"channelId"`
	Source      string        `json:"source,omitempty"`
	Title       string        `json:"title,omitempty"`
	State       DeliveryState `json:"state"`
	DelayMs     int64         `json:"delayMs"`
	GeneratedMs SimMs         `json:"generatedAtMs"`
	DeliverAtMs *SimMs        `json:"deliverAtMs"` // null when dropped
	Content     string        `json:"content"`     // as delivered; empty when dropped
	Applied     []uuid.UUID   `json:"appliedRules"`
	Conflict    *uuid.UUID    `json:"conflictGroup,omitempty"`
	SenderID    *uuid.UUID    `json:"senderId,omitempty"`
}

func (d *delivery) payload(deliverAt *SimMs) deliveryPayload {
	p := deliveryPayload{
		DeliveryID: d.ID, ItemID: d.ItemID, Team: d.Team, ChannelID: d.ChannelID,
		Source: d.Source, Title: d.Title, State: d.State, DelayMs: d.DelayMs,
		GeneratedMs: d.GeneratedMs, DeliverAtMs: deliverAt, Content: d.Content,
		Applied: append([]uuid.UUID{}, d.Applied...), Conflict: d.Conflict,
	}
	if d.Message {
		id := d.SenderID
		p.SenderID = &id
	}
	return p
}

type eventSet struct{ delivered, delayed, dropped EventType }

var (
	reportEvents  = eventSet{EvReportDelivered, EvReportDelayed, EvReportDropped}
	messageEvents = eventSet{EvMessageDelivered, EvMessageDelayed, EvMessageDropped}
)

func (d *delivery) events() eventSet {
	if d.Message {
		return messageEvents
	}
	return reportEvents
}

// generateReport releases a scripted report into the exercise.
func (e *Engine) generateReport(r Report) {
	teams := e.teams(r.Team)
	e.record(EvReportGenerated, r.Team, nil, generatedPayload{
		ItemID: r.ID, ChannelID: r.ChannelID, Teams: teams, Title: r.Title, Source: r.Source, Content: r.Content,
	})
	for _, team := range teams {
		e.dispatch(delivery{
			ItemID: r.ID, Team: team, ChannelID: r.ChannelID, Title: r.Title, Source: r.Source,
			Reliability: r.Reliability, Priority: r.Priority, Content: r.Content,
		}, 0)
	}
}

// generateConflict releases two reports on the same subject that disagree.
// They are ordinary reports as far as trainees can tell; only the record
// links them, and nothing marks either as the correct one.
func (e *Engine) generateConflict(r Rule) {
	if r.Conflict == nil {
		return
	}
	channel := uuid.Nil
	if r.ChannelID != nil {
		channel = *r.ChannelID
	} else if len(e.def.Channels) > 0 {
		channel = e.def.Channels[0].ID
	}
	teams := e.teams(r.Team)
	group := r.ID
	title := strings.TrimSpace(r.Conflict.Topic)
	if title == "" {
		title = "Report"
	}

	sides := []struct{ tag, source, content string }{
		{"A", r.Conflict.SourceA, r.Conflict.ContentA},
		{"B", r.Conflict.SourceB, r.Conflict.ContentB},
	}
	for _, side := range sides {
		itemID := uuid.NewSHA1(r.ID, []byte(side.tag))
		e.record(EvReportGenerated, r.Team, nil, generatedPayload{
			ItemID: itemID, ChannelID: channel, Teams: teams, Title: title,
			Source: side.source, Content: side.content, Conflict: &group,
		})
		for _, team := range teams {
			e.dispatch(delivery{
				ItemID: itemID, Team: team, ChannelID: channel, Title: title, Source: side.source,
				Priority: "ROUTINE", Content: side.content, Conflict: &group,
			}, 0)
		}
	}
}

// dispatch sends one item toward one team: it looks up the channel, applies
// the rules in force right now, and then delivers, schedules or drops the
// item, recording which.
func (e *Engine) dispatch(d delivery, senderTeam int) {
	kind := "report"
	if d.Message {
		kind = "message"
	}
	d.ID = e.newID(kind)
	d.GeneratedMs = e.now

	out := Degrade(e.seed, Intent{
		ItemID: d.ItemID, ChannelID: d.ChannelID, Team: d.Team, SenderTeam: senderTeam, Content: d.Content,
	}, e.activeRules())

	d.State = out.State
	d.Content = out.Content
	d.DelayMs = out.DelayMs
	d.Applied = out.Applied
	if d.Conflict != nil && d.State != StateDropped {
		d.State = StateContradictory
	}
	events := d.events()

	if out.State == StateDropped {
		if !d.Message {
			e.st.dropped[d.Team]++
		}
		e.record(events.dropped, d.Team, nil, d.payload(nil))
		return
	}

	deliverAt := e.now + e.channel[d.ChannelID].BaseLatencyMs + out.DelayMs
	if deliverAt <= e.now {
		e.release(&d)
		return
	}
	if out.DelayMs > 0 {
		e.record(events.delayed, d.Team, nil, d.payload(&deliverAt))
	}
	if !d.Message {
		e.st.pending[d.Team]++
	}
	pending := d
	pending.DeliveredMs = -1
	e.sched.Schedule(task{at: deliverAt, rank: rankRelease, delivery: &pending})
}

// release puts an item in a team's inbox and tells that team. This is the
// only place information reaches trainees.
func (e *Engine) release(d *delivery) {
	if d.DeliveredMs == -1 && !d.Message {
		e.st.pending[d.Team]--
	}
	d.DeliveredMs = e.now
	e.st.inbox[d.Team] = append(e.st.inbox[d.Team], *d)

	at := e.now
	e.record(d.events().delivered, d.Team, nil, d.payload(&at))
	if d.Message {
		e.tell(NMessageReceived, Audience{Teams: []int{d.Team}, Except: d.SenderID}, e.messageView(*d))
		return
	}
	e.tell(NReportReceived, Audience{Teams: []int{d.Team}}, d.reportView())
}

// SendMessage sends a trainee's message on a channel. It goes through the
// same degradation as scripted reports. The sender is told only that it was
// sent, never whether it arrived.
func (e *Engine) SendMessage(wall time.Time, senderID, channelID uuid.UUID, body string) (MessageView, Output, error) {
	e.begin(wall)
	e.advance()
	sender, rejected := e.participant(senderID)
	if rejected != nil {
		return MessageView{}, e.finish(), rejected
	}
	ch, ok := e.channel[channelID]
	if !ok {
		return MessageView{}, e.finish(), reject("UNKNOWN_CHANNEL", "that channel does not exist in this exercise")
	}
	if ch.Kind != ChannelTeam && ch.Kind != ChannelCrossTeam {
		return MessageView{}, e.finish(), reject("CHANNEL_NOT_WRITABLE", "trainees cannot send on %s", ch.Name)
	}
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > MaxMessageRunes {
		return MessageView{}, e.finish(), reject("INVALID_MESSAGE", "a message must be between 1 and %d characters", MaxMessageRunes)
	}

	id := e.newID("message-item")
	teams := []int{sender.Team}
	if ch.Kind == ChannelCrossTeam {
		teams = e.teams(0)
	}

	pid := sender.ID
	e.record(EvMessageSent, sender.Team, &pid, generatedPayload{ItemID: id, ChannelID: ch.ID, Teams: teams, Source: sender.Name, Content: body})
	e.out.Messages = append(e.out.Messages, MessageRecord{
		ID: id, ChannelID: ch.ID, SenderID: sender.ID, Team: sender.Team, Body: body, SimMs: e.now, Wall: wall,
	})

	own := delivery{
		ID: id, Message: true, ItemID: id, Team: sender.Team, ChannelID: ch.ID, Source: sender.Name,
		Content: body, SenderID: sender.ID, GeneratedMs: e.now, DeliveredMs: e.now, State: StateNormal,
	}
	e.st.sent[sender.ID] = append(e.st.sent[sender.ID], own)

	for _, team := range teams {
		e.dispatch(delivery{
			Message: true, ItemID: id, Team: team, ChannelID: ch.ID, Source: sender.Name,
			Content: body, SenderID: sender.ID,
		}, sender.Team)
	}
	return e.messageView(own), e.finish(), nil
}
