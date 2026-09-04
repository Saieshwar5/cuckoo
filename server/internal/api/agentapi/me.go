package agentapi

import (
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// meResponse is what an agent learns about itself. Deliberately its own type,
// separate from the owner's view in the management API: the two are different
// contracts, and an agent never sees who owns it beyond an identifier.
type meResponse struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type meEnvelope struct {
	Agent meResponse `json:"agent"`
}

func newMeResponse(a agents.Agent) meResponse {
	return meResponse{
		ID:          domain.FormatID(domain.PrefixAgent, a.ID),
		Handle:      a.Handle,
		DisplayName: a.DisplayName,
		Description: a.Description,
		OwnerID:     domain.FormatID(domain.PrefixUser, a.OwnerID),
		CreatedAt:   a.CreatedAt,
	}
}

// getMe returns the agent the caller's secret speaks for.
func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}

	agent, err := h.agents.Get(r.Context(), agentID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, meEnvelope{Agent: newMeResponse(agent)})
}
