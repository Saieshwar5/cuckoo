package client

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// clearConversation puts everything said so far out of the caller's view.
func (h *Handler) clearConversation(w http.ResponseWriter, r *http.Request) {
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
	if err := h.conversations.Clear(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteMessageForMe takes one message out of the caller's view. A person
// can never delete for everyone: the agent has the message already.
func (h *Handler) deleteMessageForMe(w http.ResponseWriter, r *http.Request) {
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
	messageID, err := domain.ParseID(domain.PrefixMessage, chi.URLParam(r, "mid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.conversations.Hide(r.Context(), userID, id, messageID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type reportRequest struct {
	Reason    string `json:"reason"`
	Note      string `json:"note"`
	MessageID string `json:"message_id"`
}

// reportAgent files a complaint for the hub's operator to read.
func (h *Handler) reportAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	agentID, err := domain.ParseID(domain.PrefixAgent, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req reportRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	var messageID *uuid.UUID
	if req.MessageID != "" {
		id, err := domain.ParseID(domain.PrefixMessage, req.MessageID)
		if err != nil {
			httpx.Error(w, r, domain.InvalidField("message_id", "invalid_id", "message_id must be a message id."))
			return
		}
		messageID = &id
	}
	if err := h.pairing.Report(r.Context(), userID, agentID, pairing.ReportInput{
		Reason: req.Reason, Note: req.Note, MessageID: messageID,
	}); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}
