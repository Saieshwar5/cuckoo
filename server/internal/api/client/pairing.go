package client

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// agentCardResponse is an agent as someone deciding to add it sees it: who
// it is and who is behind it. Nothing an owner would keep private.
type agentCardResponse struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// A published picture, at /a/{id}/avatar.
	HasAvatar bool `json:"has_avatar"`
	Owner     struct {
		DisplayName string `json:"display_name"`
	} `json:"owner"`
	Status string `json:"status,omitempty"`
	// Verified is reserved: nothing sets it yet, and the app shows an
	// Unverified label while it is false.
	Verified bool `json:"verified"`
}

func newAgentCard(c pairing.Card) agentCardResponse {
	resp := agentCardResponse{
		ID:          domain.FormatID(domain.PrefixAgent, c.Agent.ID),
		Handle:      c.Agent.Handle,
		DisplayName: c.Agent.DisplayName,
		Description: c.Agent.Description,
		HasAvatar:   c.Agent.AvatarMediaID != nil,
	}
	resp.Owner.DisplayName = c.OwnerName
	if c.Status != nil {
		resp.Status = string(*c.Status)
	}
	return resp
}

// pairResponse is what a scanned link resolves to.
type pairResponse struct {
	Kind           string            `json:"kind"`
	Agent          agentCardResponse `json:"agent"`
	AlreadyAdded   bool              `json:"already_added"`
	Blocked        bool              `json:"blocked"`
	ConversationID *string           `json:"conversation_id"`
}

type acceptResponse struct {
	Conversation conversationResponse `json:"conversation"`
	New          bool                 `json:"new"`
}

// contactResponse is a row of the person's list of agents.
type contactResponse struct {
	Agent          agentCardResponse `json:"agent"`
	AddedVia       string            `json:"added_via"`
	Blocked        bool              `json:"blocked"`
	ConversationID string            `json:"conversation_id"`
	CreatedAt      time.Time         `json:"created_at"`
	AgentDeleted   bool              `json:"agent_deleted"`
}

type contactListEnvelope struct {
	Contacts []contactResponse `json:"contacts"`
}

func (h *Handler) resolvePair(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	card, err := h.pairing.Resolve(r.Context(), userID, chi.URLParam(r, "code"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	resp := pairResponse{Kind: pairing.KindAddAgent, Agent: newAgentCard(card), AlreadyAdded: card.AlreadyAdded, Blocked: card.Blocked}
	if card.ConversationID != nil {
		id := domain.FormatID(domain.PrefixConv, *card.ConversationID)
		resp.ConversationID = &id
	}
	httpx.JSON(w, r, http.StatusOK, resp)
}

func (h *Handler) acceptPair(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	accepted, err := h.pairing.Accept(r.Context(), userID, chi.URLParam(r, "code"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	status := http.StatusOK
	if accepted.New {
		status = http.StatusCreated
	}
	httpx.JSON(w, r, status, acceptResponse{
		Conversation: newConversationResponse(accepted.Conversation),
		New:          accepted.New,
	})
}

func (h *Handler) listContacts(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	list, err := h.pairing.Contacts(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]contactResponse, 0, len(list))
	for _, c := range list {
		card := agentCardResponse{
			ID:          domain.FormatID(domain.PrefixAgent, c.AgentID),
			Handle:      c.Handle,
			DisplayName: c.DisplayName,
			Description: c.Description,
			HasAvatar:   c.HasAvatar,
		}
		card.Owner.DisplayName = c.OwnerName
		if c.Status != nil {
			card.Status = string(*c.Status)
		}
		out = append(out, contactResponse{
			Agent:          card,
			AddedVia:       c.AddedVia,
			Blocked:        c.Blocked,
			ConversationID: domain.FormatID(domain.PrefixConv, c.ConversationID),
			CreatedAt:      c.CreatedAt,
			AgentDeleted:   c.AgentDeleted,
		})
	}
	httpx.JSON(w, r, http.StatusOK, contactListEnvelope{Contacts: out})
}

func (h *Handler) blockAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := domain.ParseID(domain.PrefixAgent, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.pairing.Block(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func (h *Handler) unblockAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := domain.ParseID(domain.PrefixAgent, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.pairing.Unblock(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
