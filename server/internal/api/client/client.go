// Package client serves the API the mobile app talks to.
//
// Every route here runs behind user authentication and acts on behalf of the
// person holding the device. The agent protocol and the management API are
// separate packages with their own authentication, so a mistake in one can
// never widen the surface of another.
package client

import (
	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/retention"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Handler holds the services the client API needs.
type Handler struct {
	users         *users.Service
	agents        *agents.Service
	conversations *conversations.Service
	hub           *realtime.Hub
	pairing       *pairing.Service
	apiKeys       *apikeys.Service
	media         *media.Service
	retention     *retention.Service
	signIn        *signin.Service
}

// New builds the client API handler.
func New(userService *users.Service, agentService *agents.Service, conversationService *conversations.Service,
	hub *realtime.Hub, pairingService *pairing.Service, apiKeyService *apikeys.Service, mediaService *media.Service,
	retentionService *retention.Service, signInService *signin.Service) *Handler {
	return &Handler{
		users: userService, agents: agentService, conversations: conversationService, hub: hub,
		pairing: pairingService, apiKeys: apiKeyService, media: mediaService, retention: retentionService,
		signIn: signInService,
	}
}

// Routes returns the client API routes, to be mounted behind authentication.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/me", h.getMe)
	r.Patch("/me", h.updateMe)
	r.Delete("/me", h.deleteMe)
	// What the person keeps on the hub, and for how long.
	r.Get("/me/storage", h.getStorage)

	r.Get("/socket", h.socket)

	r.Get("/conversations", h.listConversations)
	r.Route("/conversations/{id}", func(r chi.Router) {
		r.Get("/", h.getConversation)
		r.Get("/messages", h.listMessages)
		r.Post("/messages", h.sendMessage)
		// Out of my sight: one message, or everything so far. Nobody else's
		// view changes.
		r.Delete("/messages/{mid}", h.deleteMessageForMe)
		r.Post("/clear", h.clearConversation)
	})

	// Files: uploaded before the message that carries them, and read back
	// by anyone in the conversation it was sent into.
	r.Post("/media", h.uploadMedia)
	r.Get("/media/{id}", h.getMedia)

	// Adding agents: what a scanned code resolves to, and accepting it.
	// The catalogue: agents on offer, whether or not the person has them.
	r.Get("/catalogue", h.listCatalogue)
	r.Get("/pair/{code}", h.resolvePair)
	r.Post("/pair/{code}/accept", h.acceptPair)
	r.Get("/contacts", h.listContacts)
	r.Patch("/contacts/{id}", h.updateContact)
	r.Delete("/contacts/{id}", h.removeContact)
	r.Post("/agents/{id}/report", h.reportAgent)
	r.Post("/agents/{id}/block", h.blockAgent)
	r.Delete("/agents/{id}/block", h.unblockAgent)

	// API keys: the credential a person's own systems call the management
	// API with. Issued here, behind a person's own credential, so a key can
	// never mint another.
	// The devices a person is signed in on, and ending one or all the others.
	r.Get("/devices", h.listDevices)
	r.Delete("/devices", h.signOutOtherDevices)
	r.Delete("/devices/{id}", h.signOutDevice)

	r.Post("/api-keys", h.createAPIKey)
	r.Get("/api-keys", h.listAPIKeys)
	r.Delete("/api-keys/{id}", h.revokeAPIKey)

	return r
}
