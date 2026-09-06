// Package mgmt serves the management API: how owners create agents, connect
// them to backends, and take them down again.
//
// Every route runs as a signed-in person and acts only on agents that person
// owns. The mobile app calls this; so, later, will companies' own systems via
// API keys — through the same routes, since the authenticator is the only thing
// that changes.
package mgmt

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
)

// Handler holds the services the management API needs.
type Handler struct {
	agents  *agents.Service
	pairing *pairing.Service
	media   *media.Service
}

// New builds the management API handler.
func New(agentService *agents.Service, pairingService *pairing.Service, mediaService *media.Service) *Handler {
	return &Handler{agents: agentService, pairing: pairingService, media: mediaService}
}

// Routes returns the management routes, to be mounted behind user
// authentication.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	// Pictures for an agent to be published with. Here, rather than only on
	// the client API, because a company's servers hold a key and publishing
	// an agent's logo is an owner's job.
	r.Post("/media", h.uploadMedia)

	r.Post("/agents", h.createAgent)
	r.Get("/agents", h.listAgents)

	r.Route("/agents/{id}", func(r chi.Router) {
		r.Get("/", h.getAgent)
		r.Patch("/", h.updateAgent)
		r.Delete("/", h.deleteAgent)

		r.Post("/binding", h.setBinding)
		r.Delete("/binding", h.revokeBinding)

		// The codes that hand the agent out.
		r.Post("/pair-tokens", h.createPairToken)
		r.Get("/pair-tokens", h.listPairTokens)
		r.Delete("/pair-tokens/{tid}", h.revokePairToken)
		r.Get("/pair-tokens/{tid}/qr", h.pairTokenPicture)
	})

	return r
}

// agentIDParam reads and validates the {id} path segment.
func agentIDParam(r *http.Request) (uuid.UUID, error) {
	return domain.ParseID(domain.PrefixAgent, chi.URLParam(r, "id"))
}
