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

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
)

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
	// own, so there is no cross-site request to defend against here.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"*"}})
	if err != nil {
		// Accept has already written the failure response.
		return
	}
	defer func() { _ = c.CloseNow() }()

	sub := h.hub.Subscribe(userID)
	defer sub.Close()

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

	default:
		return frame{}, fmt.Errorf("unknown event type %q", ev.Type)
	}
}
