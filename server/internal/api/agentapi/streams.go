package agentapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
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

// finishRequest ends a stream. It is an object so buttons and attachments
// can join it.
type finishRequest struct{}

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
	if r.ContentLength != 0 {
		var req finishRequest
		if err := httpx.Decode(w, r, &req); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}

	msg, err := h.conversations.FinishStream(r.Context(), agentID, messageID)
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

// messageIDParam reads and validates the {id} path segment of a message.
func messageIDParam(r *http.Request) (uuid.UUID, error) {
	return domain.ParseID(domain.PrefixMessage, chi.URLParam(r, "id"))
}
