package mgmt

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// agentResponse is the wire shape of an agent as its owner sees it. The
// binding is included, without its secret, so the app can show "connected" or
// "not connected" without a second request.
type agentResponse struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// HasAvatar says a picture is published at /a/{id}/avatar.
	HasAvatar bool             `json:"has_avatar"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Binding   *bindingResponse `json:"binding"`
}

type bindingResponse struct {
	ID         string     `json:"id"`
	Mode       string     `json:"mode"`
	WebhookURL *string    `json:"webhook_url"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func newAgentResponse(a agents.Agent, b *agents.Binding) agentResponse {
	resp := agentResponse{
		ID:          domain.FormatID(domain.PrefixAgent, a.ID),
		Handle:      a.Handle,
		DisplayName: a.DisplayName,
		Description: a.Description,
		HasAvatar:   a.AvatarMediaID != nil,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}
	if b != nil {
		br := newBindingResponse(*b)
		resp.Binding = &br
	}
	return resp
}

func newBindingResponse(b agents.Binding) bindingResponse {
	return bindingResponse{
		ID:         domain.FormatID(domain.PrefixBinding, b.ID),
		Mode:       string(b.Mode),
		WebhookURL: b.WebhookURL,
		Status:     string(b.Status),
		LastSeenAt: b.LastSeenAt,
		CreatedAt:  b.CreatedAt,
	}
}

type agentEnvelope struct {
	Agent agentResponse `json:"agent"`
}

type agentListEnvelope struct {
	Agents []agentResponse `json:"agents"`
}

type createAgentRequest struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// A picture already uploaded, to publish the agent with.
	AvatarMediaID string `json:"avatar_media_id"`
}

func (h *Handler) createAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}

	var req createAgentRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	avatar, err := avatarIDOf(req.AvatarMediaID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	agent, err := h.agents.Create(r.Context(), userID, agents.CreateInput{
		Handle:        req.Handle,
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		AvatarMediaID: avatar,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusCreated, agentEnvelope{Agent: newAgentResponse(agent, nil)})
}

func (h *Handler) listAgents(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}

	list, err := h.agents.ListMine(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	out := make([]agentResponse, 0, len(list))
	for _, a := range list {
		b, err := h.agents.ActiveBinding(r.Context(), userID, a.ID)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		out = append(out, newAgentResponse(a, b))
	}

	httpx.JSON(w, r, http.StatusOK, agentListEnvelope{Agents: out})
}

func (h *Handler) getAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	agent, err := h.agents.GetOwned(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	binding, err := h.agents.ActiveBinding(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, agentEnvelope{Agent: newAgentResponse(agent, binding)})
}

type updateAgentRequest struct {
	DisplayName   *string `json:"display_name"`
	Description   *string `json:"description"`
	AvatarMediaID *string `json:"avatar_media_id"`
}

// avatarIDOf parses the id of a picture a profile is being given.
func avatarIDOf(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := domain.ParseID(domain.PrefixMedia, raw)
	if err != nil {
		return nil, domain.InvalidField("avatar_media_id", "invalid_id",
			"avatar_media_id is the id of a picture already uploaded.")
	}
	return &id, nil
}

func (h *Handler) updateAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	var req updateAgentRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	var avatar *uuid.UUID
	if req.AvatarMediaID != nil {
		if avatar, err = avatarIDOf(*req.AvatarMediaID); err != nil {
			httpx.Error(w, r, err)
			return
		}
	}
	agent, err := h.agents.Update(r.Context(), userID, id, agents.UpdateInput{
		DisplayName:   req.DisplayName,
		Description:   req.Description,
		AvatarMediaID: avatar,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	binding, err := h.agents.ActiveBinding(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, agentEnvelope{Agent: newAgentResponse(agent, binding)})
}

func (h *Handler) deleteAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	if err := h.agents.Delete(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
