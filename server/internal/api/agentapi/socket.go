package agentapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
)

// Close codes a backend may see. The SDK stops on both rather than
// reconnecting: another connection took over, or the credential is gone.
const (
	CloseReplaced    websocket.StatusCode = 4001
	CloseBindingGone websocket.StatusCode = 4002
)

const (
	// eventSocketOpened is published to the agent when a socket opens for
	// it. Every older socket for the same agent, on any instance, closes.
	eventSocketOpened = "agent.socket.opened"

	socketPingInterval = 30 * time.Second
	socketPingTimeout  = 15 * time.Second
	socketWriteTimeout = 10 * time.Second
	socketReadLimit    = 64 << 10
)

// inboundFrame is anything a backend sends: an ack, or an operation. One
// shape, so the reader needs one decode.
type inboundFrame struct {
	Ack            string            `json:"ack"`
	Op             string            `json:"op"`
	CID            string            `json:"cid"`
	ConversationID string            `json:"conversation_id"`
	MessageID      string            `json:"message_id"`
	Text           string            `json:"text"`
	IdempotencyKey string            `json:"idempotency_key"`
	ReplyTo        string            `json:"reply_to"`
	Buttons        [][]buttonInput   `json:"buttons"`
	QuickReplies   []quickReplyInput `json:"quick_replies"`
	State          string            `json:"state"`
	Label          string            `json:"label"`
	ScheduleID     string            `json:"schedule_id"`
}

type openedPayload struct {
	ConnID string `json:"conn_id"`
}

// socket holds a backend's connection and feeds it the outbox.
//
// On connect, everything pending for the agent is pushed in order; from then
// on, each nudge and each heartbeat pushes whatever is due, which includes
// anything pushed earlier and never acknowledged. Only an ack marks an event
// delivered. The socket never carries a reply: replies go over the same
// HTTP endpoint as everything else, which keeps one path with idempotency
// and rate limits.
func (h *Handler) socket(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}
	ep, err := h.agents.ActiveEndpoint(r.Context(), agentID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if ep == nil {
		httpx.Error(w, r, domain.Forbidden("no_binding", "This agent has no backend connected."))
		return
	}
	if ep.Mode != agents.ModeSocket {
		httpx.Error(w, r, domain.Forbidden("binding_not_socket",
			"This binding is in webhook mode; its events are posted to its URL. Set a socket binding to connect."))
		return
	}

	// Backends are programs, not browsers: no origin to check.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(socketReadLimit)

	agent, err := h.agents.Get(r.Context(), agentID)
	if err != nil {
		return
	}
	s := &agentSocket{
		h: h, c: c, agentID: agentID, agentName: agent.DisplayName, binding: ep.BindingID, connID: domain.NewID(),
		log: slog.Default().With("component", "agent-socket", "agent", agentID),
	}
	s.run(r.Context())
}

type agentSocket struct {
	h         *Handler
	c         *websocket.Conn
	agentID   uuid.UUID
	agentName string
	binding   uuid.UUID
	connID    uuid.UUID
	log       *slog.Logger
}

func (s *agentSocket) run(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	// Subscribe before announcing, so this connection hears its own
	// announcement and ignores it, and any older one hears it and leaves.
	sub := s.h.hub.Subscribe(s.agentID)
	defer sub.Close()
	if err := s.announce(ctx); err != nil {
		s.log.Warn("could not announce socket", "error", err)
		return
	}

	if err := s.h.agents.SocketConnected(ctx, s.binding); err != nil {
		s.log.Warn("could not mark binding connected", "error", err)
	}
	replaced := false
	defer func() {
		// A newer socket owns the binding now; saying "idle" would undo it.
		if replaced {
			return
		}
		if err := s.h.agents.SocketClosed(context.WithoutCancel(ctx), s.binding); err != nil {
			s.log.Warn("could not mark binding idle", "error", err)
		}
	}()

	acks := make(chan uuid.UUID, 64)
	ops := make(chan inboundFrame, 64)
	go s.read(ctx, cancel, acks, ops)

	if !s.push(ctx) {
		return
	}

	pings := time.NewTicker(socketPingInterval)
	defer pings.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case ev, open := <-sub.C:
			if !open {
				_ = s.c.Close(websocket.StatusGoingAway, "reconnect")
				return
			}
			switch ev.Type {
			case conversations.EventDeliveryPending:
				if !s.push(ctx) {
					return
				}
			case eventSocketOpened:
				var p openedPayload
				if err := json.Unmarshal(ev.Payload, &p); err == nil && p.ConnID != s.connID.String() {
					replaced = true
					_ = s.c.Close(CloseReplaced, "replaced by a newer connection")
					return
				}
			}

		case id := <-acks:
			acked, err := s.h.delivery.Ack(ctx, s.agentID, id)
			if err != nil {
				s.log.Warn("ack failed", "event", id, "error", err)
				continue
			}
			if acked {
				if err := s.h.agents.RecordDeliverySuccess(ctx, s.binding); err != nil {
					s.log.Warn("could not record success", "error", err)
				}
			}

		case op := <-ops:
			if !s.handleOp(ctx, op) {
				return
			}

		case <-pings.C:
			if !s.heartbeat(ctx) {
				return
			}
		}
	}
}

// announce publishes this socket's arrival to every socket for the agent.
func (s *agentSocket) announce(ctx context.Context) error {
	ev, err := realtime.NewEvent(eventSocketOpened, []uuid.UUID{s.agentID}, openedPayload{ConnID: s.connID.String()})
	if err != nil {
		return err
	}
	return s.h.publisher.Publish(ctx, ev)
}

// read is the only reader. It turns frames into acks and operations for
// the main loop, so every piece of database work and every write stays on
// one goroutine, and it ends the connection when the backend goes.
func (s *agentSocket) read(ctx context.Context, cancel context.CancelFunc, acks chan<- uuid.UUID, ops chan<- inboundFrame) {
	defer cancel()
	for {
		_, data, err := s.c.Read(ctx)
		if err != nil {
			return
		}
		var f inboundFrame
		if err := json.Unmarshal(data, &f); err != nil {
			continue
		}
		switch {
		case f.Ack != "":
			id, err := domain.ParseID(domain.PrefixEvent, f.Ack)
			if err != nil {
				continue
			}
			select {
			case acks <- id:
			case <-ctx.Done():
				return
			}
		case f.Op != "":
			select {
			case ops <- f:
			case <-ctx.Done():
				return
			}
		}
	}
}

// push sends everything due until the outbox is drained. False means the
// connection is gone.
func (s *agentSocket) push(ctx context.Context) bool {
	for {
		envelopes, more, err := s.h.delivery.ClaimForSocket(ctx, s.agentID)
		if err != nil {
			s.log.Warn("could not claim outbox", "error", err)
			return true
		}
		for _, env := range envelopes {
			wctx, cancel := context.WithTimeout(ctx, socketWriteTimeout)
			err := wsjson.Write(wctx, s.c, env)
			cancel()
			if err != nil {
				return false
			}
		}
		if !more {
			return true
		}
	}
}

// heartbeat pings, confirms the binding still exists in socket mode, keeps
// last-seen fresh, and sweeps the outbox for anything due again.
func (s *agentSocket) heartbeat(ctx context.Context) bool {
	pctx, cancel := context.WithTimeout(ctx, socketPingTimeout)
	err := s.c.Ping(pctx)
	cancel()
	if err != nil {
		return false
	}

	current, err := s.h.agents.ActiveEndpoint(ctx, s.agentID)
	if err != nil {
		s.log.Warn("could not check binding", "error", err)
		return true
	}
	if current == nil || current.BindingID != s.binding || current.Mode != agents.ModeSocket {
		_ = s.c.Close(CloseBindingGone, "binding revoked or replaced")
		return false
	}
	if err := s.h.agents.SocketConnected(ctx, s.binding); err != nil {
		s.log.Warn("could not refresh binding", "error", err)
	}
	return s.push(ctx)
}
