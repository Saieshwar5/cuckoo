package client

import (
	"net/http"
	"strconv"
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

type messageResponse struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	Sender         senderResponse `json:"sender"`
	Body           bodyResponse   `json:"body"`
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
	return messageResponse{
		ID:             domain.FormatID(domain.PrefixMessage, m.ID),
		ConversationID: domain.FormatID(domain.PrefixConv, m.ConversationID),
		Sender: senderResponse{
			Kind: string(m.Sender.Kind),
			ID:   formatParticipantID(m.Sender.Kind, m.Sender.ID),
		},
		Body:      bodyResponse{Text: m.Body.Text},
		CreatedAt: m.CreatedAt,
	}
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
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, r, domain.InvalidField("limit", "invalid_limit",
				"Limit must be a whole number."))
			return
		}
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
