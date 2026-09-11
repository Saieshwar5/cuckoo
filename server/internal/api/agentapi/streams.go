package agentapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// appendRequest is one piece of a streaming message's text.
type appendRequest struct {
	Text string `json:"text"`
}

// appendStream adds text to a stream the agent started.
func (h *Handler) appendStream(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}
	messageID, err := messageIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req appendRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.conversations.AppendStream(r.Context(), agentID, messageID, req.Text); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// finishRequest ends a stream, with the buttons and quick replies the final
// message offers, if any.
type finishRequest struct {
	Buttons      [][]buttonInput   `json:"buttons"`
	QuickReplies []quickReplyInput `json:"quick_replies"`
}

// finishStream ends a stream the agent started and returns the message as
// it now is. Finishing twice returns the finished message again.
func (h *Handler) finishStream(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}
	messageID, err := messageIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req finishRequest
	if r.ContentLength != 0 {
		if err := httpx.Decode(w, r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}

	msg, err := h.conversations.FinishStream(r.Context(), agentID, messageID, conversations.FinishInput{
		Buttons: buttonsOf(req.Buttons), QuickReplies: quickRepliesOf(req.QuickReplies),
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
	httpx.JSON(w, r, http.StatusOK, messageEnvelope{Message: events.MessageOf(msg, agent.DisplayName)})
}

// typingRequest is an agent saying it is working, or has stopped.
type typingRequest struct {
	State string `json:"state"`
}

// activityRequest is an agent saying what it is doing: thinking, working at
// what the label names, or idle.
type activityRequest struct {
	State string `json:"state"`
	Label string `json:"label"`
}

// activity shows what the agent is doing on the person's device, or clears
// it. It lasts ten seconds unless said again.
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) {
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
	var req activityRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.conversations.Activity(r.Context(), agentID, conversationID, req.State, req.Label); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// typing shows or hides the "working" indicator on the person's device.
func (h *Handler) typing(w http.ResponseWriter, r *http.Request) {
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
	var req typingRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.conversations.Typing(r.Context(), agentID, conversationID, req.State); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// messageIDParam reads and validates the {id} path segment of a message.
func messageIDParam(r *http.Request) (uuid.UUID, error) {
	return domain.ParseID(domain.PrefixMessage, chi.URLParam(r, "id"))
}
