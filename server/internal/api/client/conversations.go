package client

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// participantResponse is a member of a conversation as the app shows it. The
// same shape names a person or an agent; kind says which, and only agents
// have a handle.
type participantResponse struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Handle      string `json:"handle,omitempty"`
	// Status is an agent's connection health: idle, connected or
	// unreachable, and absent when no backend is connected. The dot on the
	// avatar.
	Status string `json:"status,omitempty"`
}

// conversationResponse is a row of the chat list. LastMessage is null until
// someone has spoken.
type conversationResponse struct {
	ID           string                `json:"id"`
	Kind         string                `json:"kind"`
	Participants []participantResponse `json:"participants"`
	LastMessage  *messageResponse      `json:"last_message"`
	CreatedAt    time.Time             `json:"created_at"`
}

type conversationEnvelope struct {
	Conversation conversationResponse `json:"conversation"`
}

type conversationListEnvelope struct {
	Conversations []conversationResponse `json:"conversations"`
}

func newConversationResponse(c conversations.Conversation) conversationResponse {
	resp := conversationResponse{
		ID:           domain.FormatID(domain.PrefixConv, c.ID),
		Kind:         string(c.Kind),
		Participants: make([]participantResponse, 0, len(c.Participants)),
		CreatedAt:    c.CreatedAt,
	}
	for _, p := range c.Participants {
		resp.Participants = append(resp.Participants, participantResponse{
			Kind:        string(p.Kind),
			ID:          formatParticipantID(p.Kind, p.ID),
			DisplayName: p.DisplayName,
			Handle:      p.Handle,
			Status:      p.Status,
		})
	}
	if c.LastMessage != nil {
		m := newMessageResponse(*c.LastMessage)
		resp.LastMessage = &m
	}
	return resp
}

// formatParticipantID prefixes an identifier by what it names, so a person
// and an agent can never be confused even when they share a field.
func formatParticipantID(kind conversations.ParticipantKind, id uuid.UUID) string {
	if kind == conversations.ParticipantAgent {
		return domain.FormatID(domain.PrefixAgent, id)
	}
	return domain.FormatID(domain.PrefixUser, id)
}

func (h *Handler) listConversations(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}

	list, err := h.conversations.ListMine(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	out := make([]conversationResponse, 0, len(list))
	for _, c := range list {
		out = append(out, newConversationResponse(c))
	}
	httpx.JSON(w, r, http.StatusOK, conversationListEnvelope{Conversations: out})
}

func (h *Handler) getConversation(w http.ResponseWriter, r *http.Request) {
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

	conv, err := h.conversations.GetMine(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, conversationEnvelope{Conversation: newConversationResponse(conv)})
}

// conversationIDParam reads and validates the {id} path segment.
func conversationIDParam(r *http.Request) (uuid.UUID, error) {
	return domain.ParseID(domain.PrefixConv, chi.URLParam(r, "id"))
}
