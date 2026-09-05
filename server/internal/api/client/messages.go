package client

import (
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

type senderResponse struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// bodyResponse is the message content. It is an object, not a string, so
// attachments and buttons can join text without changing the shape.
type bodyResponse struct {
	Text string `json:"text,omitempty"`
}

// messageResponse is a message as the app shows it. DeliveryStatus is the
// tick mark: pending, delivered or failed for a message the caller sent to
// agents, and null for one with nobody to deliver to.
type messageResponse struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	Sender         senderResponse `json:"sender"`
	Body           bodyResponse   `json:"body"`
	DeliveryStatus *string        `json:"delivery_status"`
	CreatedAt      time.Time      `json:"created_at"`
}

type messageEnvelope struct {
	Message messageResponse `json:"message"`
}

// messagePageEnvelope is one page of history. NextBefore is the value to pass
// as ?before= for older messages, or null when there are none.
type messagePageEnvelope struct {
	Messages   []messageResponse `json:"messages"`
	NextBefore *string           `json:"next_before"`
}

func newMessageResponse(m conversations.Message) messageResponse {
	resp := messageResponse{
		ID:             domain.FormatID(domain.PrefixMessage, m.ID),
		ConversationID: domain.FormatID(domain.PrefixConv, m.ConversationID),
		Sender: senderResponse{
			Kind: string(m.Sender.Kind),
			ID:   formatParticipantID(m.Sender.Kind, m.Sender.ID),
		},
		Body:      bodyResponse{Text: m.Body.Text},
		CreatedAt: m.CreatedAt,
	}
	if m.DeliveryStatus != "" {
		status := string(m.DeliveryStatus)
		resp.DeliveryStatus = &status
	}
	return resp
}

type sendMessageRequest struct {
	Text string `json:"text"`
}

func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	conversationID, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	var req sendMessageRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	msg, err := h.conversations.SendText(r.Context(), userID, conversationID, req.Text)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, messageEnvelope{Message: newMessageResponse(msg)})
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	conversationID, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	before, err := httpx.QueryID(r, "before", domain.PrefixMessage)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, err := httpx.QueryInt(r, "limit")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	page, err := h.conversations.ListMessages(r.Context(), userID, conversationID,
		conversations.ListMessagesInput{Before: before, Limit: limit})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	out := messagePageEnvelope{Messages: make([]messageResponse, 0, len(page.Messages))}
	for _, m := range page.Messages {
		out.Messages = append(out.Messages, newMessageResponse(m))
	}
	if page.NextBefore != nil {
		cursor := domain.FormatID(domain.PrefixMessage, *page.NextBefore)
		out.NextBefore = &cursor
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
