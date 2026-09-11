package client

import (
	"net/http"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// stoppedEnvelope lists the replies a stop ended where they stood. Empty
// when the agent had not started writing: it was still thinking, and has
// been told.
type stoppedEnvelope struct {
	Stopped []messageResponse `json:"stopped"`
}

// stopConversation asks the agents in a conversation to stop. It answers as
// soon as the hub has done its part — the words stop, the agents are told —
// and never waits on a backend.
func (h *Handler) stopConversation(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	ended, err := h.conversations.Stop(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := stoppedEnvelope{Stopped: make([]messageResponse, 0, len(ended))}
	for _, m := range ended {
		out.Stopped = append(out.Stopped, newMessageResponse(m))
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

type readRequest struct {
	MessageID string `json:"message_id"`
}

// markRead records how far the person has read a conversation.
func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req readRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	messageID, err := domain.ParseID(domain.PrefixMessage, req.MessageID)
	if err != nil {
		httpx.Error(w, r, domain.InvalidField("message_id", "invalid_id", "message_id must be a message id."))
		return
	}
	if err := h.conversations.MarkRead(r.Context(), userID, id, messageID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
