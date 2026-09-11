package client

import (
	"context"
	"encoding/json"
	"fmt"
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

// socketSubprotocol is the name a browser client offers alongside its token;
// the hub selects it so the browser completes the handshake.
const socketSubprotocol = "cuckoo"

const (
	// pingInterval keeps the connection alive through proxies and tells us
	// when a phone has silently gone. A ping unanswered within pingTimeout
	// ends the connection; the app reconnects and catches up.
	pingInterval = 30 * time.Second
	pingTimeout  = 15 * time.Second
	writeTimeout = 10 * time.Second
)

// frame is what travels over the socket: a type and the same data shapes the
// REST API uses, so the app has one parser for both.
type frame struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type readyFrame struct {
	UserID string `json:"user_id"`
}

// agentStatusFrame says an agent's backend came, went, or stopped answering:
// connected, idle, unreachable, or none when there is no binding at all.
type agentStatusFrame struct {
	AgentID string `json:"agent_id"`
	Status  string `json:"status"`
}

type messageCreatedFrame struct {
	ConversationID string          `json:"conversation_id"`
	Message        messageResponse `json:"message"`
}

type deliveryUpdatedFrame struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	DeliveryStatus string `json:"delivery_status"`
}

// messageDeltaFrame is one piece of a streaming message. The app appends
// it to the bubble it drew on message.started.
type messageDeltaFrame struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Text           string `json:"text"`
}

// activityFrame says what an agent is doing: thinking, working at what the
// label names, or idle. The app shows it until ExpiresAt, an idle frame, or
// a message, whichever comes first.
type activityFrame struct {
	ConversationID string    `json:"conversation_id"`
	AgentID        string    `json:"agent_id"`
	State          string    `json:"state"`
	Label          string    `json:"label,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// readFrame says this person has read a conversation up to a message, on
// this device or another: the badge goes.
type readFrame struct {
	ConversationID string `json:"conversation_id"`
	ReadUpTo       string `json:"read_up_to"`
}

// socket holds a live connection for the signed-in person and pushes them
// every event in their conversations until they leave.
//
// The socket only ever pushes; the app sends nothing but pongs. Everything
// the app wants to do goes through the REST API, which keeps one set of
// rules, one set of errors, and one place to rate limit.
func (h *Handler) socket(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}

	// Any origin is accepted: callers authenticate with a header the app
	// sets explicitly, never with a cookie a browser would attach on its
	// own, so there is no cross-site request to defend against here. The
	// "cuckoo" subprotocol is how a browser client carries its token.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
		Subprotocols:   []string{socketSubprotocol},
	})
	if err != nil {
		// Accept has already written the failure response.
		return
	}
	defer func() { _ = c.CloseNow() }()

	sub := h.hub.Subscribe(userID)
	defer sub.Close()

	// Say this person is looking, and keep saying it. What stops a
	// notification arriving about a message they are watching land — and it
	// is shared, so a second instance of the hub knows too, which a map in
	// this process would not.
	presenceCtx, stopPresence := context.WithCancel(r.Context())
	defer stopPresence()
	h.announcePresence(presenceCtx, userID)

	// The app sends nothing; reading in the background is what processes
	// its pongs and close frames, and ctx ends when the connection does.
	ctx := c.CloseRead(r.Context())

	if err := write(ctx, c, frame{Type: "ready", Data: readyFrame{
		UserID: domain.FormatID(domain.PrefixUser, userID),
	}}); err != nil {
		return
	}

	pings := time.NewTicker(pingInterval)
	defer pings.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case ev, open := <-sub.C:
			if !open {
				// Dropped by the hub for falling behind, or the server is
				// stopping. Either way: reconnect and catch up.
				_ = c.Close(websocket.StatusGoingAway, "reconnect")
				return
			}
			fr, err := frameOf(ev)
			if err != nil {
				slog.WarnContext(ctx, "socket: unrenderable event", "type", ev.Type, "error", err)
				continue
			}
			if err := write(ctx, c, fr); err != nil {
				return
			}

		case <-pings.C:
			pingCtx, cancel := context.WithTimeout(ctx, pingTimeout)
			err := c.Ping(pingCtx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func write(ctx context.Context, c *websocket.Conn, fr frame) error {
	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return wsjson.Write(ctx, c, fr)
}

// frameOf renders an internal event as the app sees it.
func frameOf(ev realtime.Event) (frame, error) {
	switch ev.Type {
	case conversations.EventMessageCreated, conversations.EventMessageStarted, conversations.EventMessageCompleted:
		var p conversations.MessageCreatedEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return frame{}, err
		}
		return frame{Type: ev.Type, Data: messageCreatedFrame{
			ConversationID: domain.FormatID(domain.PrefixConv, p.ConversationID),
			Message:        newMessageResponse(p.Message),
		}}, nil

	case conversations.EventMessageDelta:
		var p conversations.MessageDeltaEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return frame{}, err
		}
		return frame{Type: ev.Type, Data: messageDeltaFrame{
			ConversationID: domain.FormatID(domain.PrefixConv, p.ConversationID),
			MessageID:      domain.FormatID(domain.PrefixMessage, p.MessageID),
			Text:           p.Text,
		}}, nil

	case conversations.EventActivity:
		var p conversations.ActivityEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return frame{}, err
		}
		return frame{Type: ev.Type, Data: activityFrame{
			ConversationID: domain.FormatID(domain.PrefixConv, p.ConversationID),
			AgentID:        domain.FormatID(domain.PrefixAgent, p.AgentID),
			State:          p.State,
			Label:          p.Label,
			ExpiresAt:      p.ExpiresAt,
		}}, nil

	case conversations.EventConversationRead:
		var p conversations.ReadEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return frame{}, err
		}
		return frame{Type: ev.Type, Data: readFrame{
			ConversationID: domain.FormatID(domain.PrefixConv, p.ConversationID),
			ReadUpTo:       domain.FormatID(domain.PrefixMessage, p.ReadUpTo),
		}}, nil

	case conversations.EventDeliveryUpdated:
		var p conversations.DeliveryUpdatedEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return frame{}, err
		}
		return frame{Type: ev.Type, Data: deliveryUpdatedFrame{
			ConversationID: domain.FormatID(domain.PrefixConv, p.ConversationID),
			MessageID:      domain.FormatID(domain.PrefixMessage, p.MessageID),
			DeliveryStatus: string(p.DeliveryStatus),
		}}, nil

	case agents.EventAgentStatus:
		var p agents.AgentStatusEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return frame{}, err
		}
		return frame{Type: ev.Type, Data: agentStatusFrame{
			AgentID: domain.FormatID(domain.PrefixAgent, p.AgentID),
			Status:  p.Status,
		}}, nil
	default:
		return frame{}, fmt.Errorf("unknown event type %q", ev.Type)
	}
}

// announcePresence marks somebody as connected and keeps the mark fresh until
// the connection ends.
//
// A heartbeat rather than a flag, so a process killed mid-connection stops
// suppressing that person's notifications within a minute rather than
// forever.
func (h *Handler) announcePresence(ctx context.Context, userID uuid.UUID) {
	if h.presence == nil {
		return
	}
	mark := func() {
		if err := h.presence.Arrived(ctx, userID); err != nil {
			slog.WarnContext(ctx, "presence: could not mark connected", "user", userID, "error", err)
		}
	}
	mark()
	go func() {
		ticker := time.NewTicker(realtime.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				// Best effort, with a context of its own: the request's is
				// already cancelled, and the key expires regardless.
				gone, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
				defer cancel()
				if err := h.presence.Left(gone, userID); err != nil {
					slog.WarnContext(gone, "presence: could not mark gone", "user", userID, "error", err)
				}
				return
			case <-ticker.C:
				mark()
			}
		}
	}()
}
