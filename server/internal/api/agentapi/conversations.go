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

// conversationResponse is a conversation as its agent sees it: the same
// shape the event envelope carries, plus who is in it.
type conversationResponse struct {
	events.Conversation
	Participants []events.Participant `json:"participants"`
}

type conversationEnvelope struct {
	Conversation conversationResponse `json:"conversation"`
}

// getConversation tells a backend what it is replying into.
func (h *Handler) getConversation(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}
	id, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	conv, err := h.conversations.GetForAgent(r.Context(), agentID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, conversationEnvelope{Conversation: conversationResponse{
		Conversation: events.ConversationOf(conv),
		Participants: events.ParticipantsOf(conv, agentID),
	}})
}

// conversationIDParam reads and validates the {id} path segment.
func conversationIDParam(r *http.Request) (uuid.UUID, error) {
	return domain.ParseID(domain.PrefixConv, chi.URLParam(r, "id"))
}
