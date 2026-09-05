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

// messagePageEnvelope is one page of history. Paging back, NextBefore is the
// value to pass as ?before= for older messages; paging forward with ?after=,
// NextAfter continues. Null means the history ends there.
type messagePageEnvelope struct {
	Messages   []messageResponse `json:"messages"`
	NextBefore *string           `json:"next_before"`
	NextAfter  *string           `json:"next_after"`
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

// sendMessageRequest is a send. The idempotency key is optional on the wire
// and always sent by the app: a retry after a dropped response returns the
// message already created rather than a duplicate bubble.
type sendMessageRequest struct {
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
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

	res, err := h.conversations.SendAsUser(r.Context(), userID, conversationID, conversations.SendInput{
		Text:           req.Text,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	status := http.StatusCreated
	if !res.Created {
		status = http.StatusOK
	}
	httpx.JSON(w, r, status, messageEnvelope{Message: newMessageResponse(res.Message)})
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
	after, err := httpx.QueryID(r, "after", domain.PrefixMessage)
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
		conversations.ListMessagesInput{Before: before, After: after, Limit: limit})
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
	if page.NextAfter != nil {
		cursor := domain.FormatID(domain.PrefixMessage, *page.NextAfter)
		out.NextAfter = &cursor
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
