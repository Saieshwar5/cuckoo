package auth

import (
	"net/http"
	"strings"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// BindingAuthenticator identifies an agent backend by its binding secret.
//
// This is the credential of the agent protocol: `Authorization: Bearer
// bnd_sec_…`. It is the second authenticator behind the same interface as the
// development header, and — the point of the interface — no handler knows
// which one it is talking through.
type BindingAuthenticator struct {
	agents *agents.Service
}

// NewBinding builds the authenticator.
func NewBinding(svc *agents.Service) *BindingAuthenticator {
	return &BindingAuthenticator{agents: svc}
}

// Authenticate reads the bearer secret and resolves it to an agent.
func (a *BindingAuthenticator) Authenticate(r *http.Request) (principal.Principal, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return principal.Principal{}, domain.Unauthorized("missing_credentials",
			"Send Authorization: Bearer <binding secret>.")
	}

	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return principal.Principal{}, domain.Unauthorized("invalid_credentials",
			"Authorization header must be: Bearer <binding secret>.")
	}

	agentID, err := a.agents.ResolveSecret(r.Context(), strings.TrimSpace(token))
	if err != nil {
		return principal.Principal{}, err
	}
	return principal.Agent(agentID), nil
}
