package mgmt

import (
	"net/http"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

type setBindingRequest struct {
	Mode       string `json:"mode"`
	WebhookURL string `json:"webhook_url"`
}

// setBindingEnvelope carries the secret alongside the binding. This is the
// only response in the whole API that ever contains it.
type setBindingEnvelope struct {
	Binding bindingResponse `json:"binding"`
	Secret  string          `json:"secret"`
}

// setBinding connects an agent to a backend, replacing any existing binding,
// and returns the secret the backend will authenticate with — once.
func (h *Handler) setBinding(w http.ResponseWriter, r *http.Request) {
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

	var req setBindingRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	binding, secret, err := h.agents.SetBinding(r.Context(), userID, id, agents.SetBindingInput{
		Mode:       agents.Mode(req.Mode),
		WebhookURL: req.WebhookURL,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusCreated, setBindingEnvelope{
		Binding: newBindingResponse(binding),
		Secret:  secret,
	})
}

func (h *Handler) revokeBinding(w http.ResponseWriter, r *http.Request) {
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

	if err := h.agents.RevokeBinding(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
