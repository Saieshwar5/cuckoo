package agentapi

import (
	"net/http"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

type messageEnvelope struct {
	Message events.Message `json:"message"`
}

// messagePageEnvelope is one page of history, newest first. NextBefore is
// the value to pass as ?before= for older messages, or null at the start of
// what the agent may see.
type messagePageEnvelope struct {
	Messages   []events.Message `json:"messages"`
	NextBefore *string          `json:"next_before"`
}

// sendMessageRequest is a reply. The idempotency key is optional on the
// wire and always sent by the SDK: a retry after a lost response returns
// the message already created rather than a duplicate.
type sendMessageRequest struct {
	Text           string `json:"text"`
	IdempotencyKey string `json:"idempotency_key"`
}

// sendMessage posts a message from the agent into a conversation it is in.
// 201 when created; 200 when the idempotency key matched an earlier send.
func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
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

	res, err := h.conversations.SendAsAgent(r.Context(), agentID, conversationID, conversations.SendInput{
		Text:           req.Text,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	agent, err := h.agents.Get(r.Context(), agentID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	status := http.StatusCreated
	if !res.Created {
		status = http.StatusOK
	}
	httpx.JSON(w, r, status, messageEnvelope{Message: events.MessageOf(res.Message, agent.DisplayName)})
}

// listMessages pages a conversation's history as the agent may see it.
func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
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

	// The conversation is loaded for its members' names, which is also the
	// membership check; the page query repeats that check cheaply.
	conv, err := h.conversations.GetForAgent(r.Context(), agentID, conversationID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	page, err := h.conversations.ListMessagesForAgent(r.Context(), agentID, conversationID,
		conversations.ListMessagesInput{Before: before, Limit: limit})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	out := messagePageEnvelope{Messages: make([]events.Message, 0, len(page.Messages))}
	for _, m := range page.Messages {
		out.Messages = append(out.Messages, events.MessageOf(m, events.SenderName(conv, m.Sender)))
	}
	if page.NextBefore != nil {
		cursor := domain.FormatID(domain.PrefixMessage, *page.NextBefore)
		out.NextBefore = &cursor
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
