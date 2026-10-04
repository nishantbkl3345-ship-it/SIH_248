package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"fogline/api/internal/apperr"
	"fogline/api/internal/domain"
	"fogline/api/internal/runtime"
	"fogline/api/internal/service"
)

type participantDTO struct {
	ID          string `json:"id"`
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Team        int    `json:"team"`
}

type teamDTO struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

type sessionResponse struct {
	ID         string     `json:"id"`
	ScenarioID string     `json:"scenarioId"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	Speed      float64    `json:"speed"`
	SimMs      int64      `json:"simMs"`
	StartedAt  *time.Time `json:"startedAt"`
	EndedAt    *time.Time `json:"endedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	// Instructor only.
	JoinCode *string `json:"joinCode,omitempty"`
	Seed     *int64  `json:"seed,omitempty"`
	// Trainee only: the team the caller is on.
	MyTeam        *int             `json:"myTeam,omitempty"`
	ParticipantID *string          `json:"participantId,omitempty"`
	Teams         []teamDTO        `json:"teams"`
	Participants  []participantDTO `json:"participants"`
}

func sessionSummary(s *domain.ExerciseSession) sessionResponse {
	return sessionResponse{
		ID: s.ID.String(), ScenarioID: s.ScenarioID.String(), Name: s.Name, Status: string(s.Status),
		Speed: float64(s.SpeedMilli) / 1000, SimMs: s.SimMs, StartedAt: s.StartedAt, EndedAt: s.EndedAt,
		CreatedAt: s.CreatedAt, Teams: []teamDTO{}, Participants: []participantDTO{},
	}
}

// toSessionResponse shapes a session for whoever is asking. The instructor
// sees the join code, the seed and the whole roster; a trainee sees their own
// team's members only.
func toSessionResponse(a service.Access) sessionResponse {
	s := a.Session
	out := sessionSummary(s)
	teamNo := map[uuid.UUID]int{}
	for _, t := range s.Teams {
		teamNo[t.ID] = t.Number
		out.Teams = append(out.Teams, teamDTO{Number: t.Number, Name: t.Name})
	}
	myTeam := a.TeamNumber()
	if a.Instructor {
		out.JoinCode, out.Seed = &s.JoinCode, &s.Seed
	} else {
		id := a.Participant.ID.String()
		out.MyTeam, out.ParticipantID = &myTeam, &id
	}
	for i := range s.Participants {
		p := &s.Participants[i]
		n := 0
		if p.TeamID != nil {
			n = teamNo[*p.TeamID]
		}
		if !a.Instructor && n != myTeam {
			continue
		}
		dto := participantDTO{ID: p.ID.String(), UserID: p.UserID.String(), Team: n, DisplayName: "Trainee"}
		if p.User != nil {
			dto.DisplayName = p.User.DisplayName
		}
		out.Participants = append(out.Participants, dto)
	}
	return out
}

type createSessionRequest struct {
	ScenarioID string `json:"scenarioId" binding:"required,uuid"`
	Name       string `json:"name" binding:"max=120"`
	Seed       *int64 `json:"seed" binding:"omitempty,min=0"`
}

type joinSessionRequest struct {
	JoinCode string `json:"joinCode" binding:"required,min=4,max=16"`
}

type assignTeamRequest struct {
	Team int `json:"team" binding:"required,min=1,max=8"`
}

type speedRequest struct {
	Speed float64 `json:"speed" binding:"required,gt=0"`
}

type messageRequest struct {
	ChannelID string `json:"channelId" binding:"required,uuid"`
	Body      string `json:"body" binding:"required,max=8000"`
}

// decisionRequest is everything a trainee is allowed to supply. There is
// deliberately no timestamp, team or "information I had" field.
type decisionRequest struct {
	DecisionPointID string `json:"decisionPointId" binding:"required,uuid"`
	OptionID        string `json:"optionId" binding:"required,uuid"`
	Rationale       string `json:"rationale" binding:"max=16000"`
	Confidence      *int   `json:"confidence" binding:"omitempty,min=1,max=5"`
}

type timelineEventDTO struct {
	Seq           int64           `json:"seq"`
	SimMs         int64           `json:"simMs"`
	WallTime      time.Time       `json:"wallTime"`
	Type          string          `json:"type"`
	Team          int             `json:"team,omitempty"`
	ParticipantID *string         `json:"participantId,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

type sessionHandler struct {
	sessions       *service.SessionService
	allowedOrigins []string
}

func (h *sessionHandler) id(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		_ = c.Error(apperr.NotFound("Session not found."))
		return uuid.Nil, false
	}
	return id, true
}

func (h *sessionHandler) create(c *gin.Context) {
	var req createSessionRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	s, err := h.sessions.Create(c.Request.Context(), actor, uuid.MustParse(req.ScenarioID), req.Name, req.Seed)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"session": toSessionResponse(service.Access{Session: s, Instructor: true})})
}

func (h *sessionHandler) list(c *gin.Context) {
	actor, _ := currentUser(c)
	sessions, err := h.sessions.List(c.Request.Context(), actor)
	if err != nil {
		_ = c.Error(err)
		return
	}
	out := make([]sessionResponse, len(sessions))
	for i := range sessions {
		out[i] = sessionSummary(&sessions[i])
	}
	c.JSON(http.StatusOK, gin.H{"sessions": out})
}

func (h *sessionHandler) join(c *gin.Context) {
	var req joinSessionRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	a, err := h.sessions.Join(c.Request.Context(), actor, req.JoinCode)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": toSessionResponse(a)})
}

func (h *sessionHandler) get(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	actor, _ := currentUser(c)
	a, err := h.sessions.Authorize(c.Request.Context(), actor, id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": toSessionResponse(a)})
}

func (h *sessionHandler) assignTeam(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	pid, err := uuid.Parse(c.Param("pid"))
	if err != nil {
		_ = c.Error(apperr.NotFound("Participant not found."))
		return
	}
	var req assignTeamRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	s, err := h.sessions.AssignTeam(c.Request.Context(), actor, id, pid, req.Team)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": toSessionResponse(service.Access{Session: s, Instructor: true})})
}

// control wraps the instructor lifecycle actions, which share a shape.
func (h *sessionHandler) control(run func(*service.SessionService, context.Context, *domain.User, uuid.UUID) (*domain.ExerciseSession, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, ok := h.id(c)
		if !ok {
			return
		}
		actor, _ := currentUser(c)
		s, err := run(h.sessions, c.Request.Context(), actor, id)
		if err != nil {
			_ = c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"session": toSessionResponse(service.Access{Session: s, Instructor: true})})
	}
}

func (h *sessionHandler) speed(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req speedRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	s, err := h.sessions.SetSpeed(c.Request.Context(), actor, id, req.Speed)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": toSessionResponse(service.Access{Session: s, Instructor: true})})
}

func (h *sessionHandler) state(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	actor, _ := currentUser(c)
	a, snap, err := h.sessions.State(c.Request.Context(), actor, id)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": toSessionResponse(a), "state": snap})
}

func (h *sessionHandler) timeline(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	afterSeq, _ := strconv.ParseInt(c.Query("afterSeq"), 10, 64)
	limit, _ := strconv.Atoi(c.Query("limit"))
	actor, _ := currentUser(c)
	events, err := h.sessions.Timeline(c.Request.Context(), actor, id, afterSeq, limit)
	if err != nil {
		_ = c.Error(err)
		return
	}
	out := make([]timelineEventDTO, len(events))
	for i, e := range events {
		out[i] = timelineEventDTO{Seq: e.Seq, SimMs: e.SimMs, WallTime: e.WallTime, Type: e.Type, Team: e.TeamNo, Payload: json.RawMessage(e.Payload)}
		if e.ActorParticipantID != nil {
			pid := e.ActorParticipantID.String()
			out[i].ParticipantID = &pid
		}
	}
	c.JSON(http.StatusOK, gin.H{"events": out})
}

func (h *sessionHandler) sendMessage(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req messageRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	view, err := h.sessions.SendMessage(c.Request.Context(), actor, id, uuid.MustParse(req.ChannelID), req.Body)
	if err != nil {
		_ = c.Error(err)
		return
	}
	// "Sent" is all the sender is ever told.
	c.JSON(http.StatusCreated, gin.H{"message": view})
}

func (h *sessionHandler) submitDecision(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	var req decisionRequest
	if !bindJSON(c, &req) {
		return
	}
	actor, _ := currentUser(c)
	rec, err := h.sessions.SubmitDecision(c.Request.Context(), actor, id,
		uuid.MustParse(req.DecisionPointID), uuid.MustParse(req.OptionID), req.Rationale, req.Confidence)
	if err != nil {
		_ = c.Error(err)
		return
	}
	// The trainee gets a receipt, not the snapshot: that holds ground truth.
	c.JSON(http.StatusCreated, gin.H{"decision": gin.H{
		"id": rec.ID, "decisionPointId": rec.DecisionPointID, "optionId": rec.OptionID,
		"rationale": rec.Rationale, "confidence": rec.Confidence,
		"simMs": rec.SimMs, "wallTime": rec.Wall, "responseMs": rec.ResponseMs,
	}})
}

const (
	wsWriteTimeout = 10 * time.Second
	wsPingEvery    = 20 * time.Second
)

// originHosts turns the allowed origins into the host patterns the WebSocket
// handshake checks, so a page on another site cannot open a stream with the
// user's cookie.
func originHosts(origins []string) []string {
	var hosts []string
	for _, o := range origins {
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			hosts = append(hosts, u.Host)
		}
	}
	return hosts
}

// stream upgrades to a WebSocket and relays the caller's realtime events.
// The stream is one-way: commands go through the REST endpoints, where they
// are authorised and validated like any other request.
func (h *sessionHandler) stream(c *gin.Context) {
	id, ok := h.id(c)
	if !ok {
		return
	}
	actor, _ := currentUser(c)
	// Authorise before upgrading, so a refusal is an ordinary HTTP error.
	if _, err := h.sessions.Authorize(c.Request.Context(), actor, id); err != nil {
		_ = c.Error(err)
		return
	}

	conn, err := websocket.Accept(c.Writer, c.Request, &websocket.AcceptOptions{OriginPatterns: originHosts(h.allowedOrigins)})
	if err != nil {
		return // Accept has already written the HTTP error
	}
	defer conn.CloseNow()

	client, leave, err := h.sessions.Connect(c.Request.Context(), actor, id)
	if err != nil {
		conn.Close(websocket.StatusPolicyViolation, "not authorised")
		return
	}
	defer leave()

	// CloseRead discards anything the client sends and cancels ctx when the
	// connection goes away.
	ctx := conn.CloseRead(c.Request.Context())
	relay(ctx, conn, client)
}

func relay(ctx context.Context, conn *websocket.Conn, client *runtime.Client) {
	ping := time.NewTicker(wsPingEvery)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case frame, open := <-client.Send():
			if !open {
				conn.Close(websocket.StatusTryAgainLater, "disconnected by server; reconnect to resync")
				return
			}
			wctx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := conn.Write(wctx, websocket.MessageText, frame)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
