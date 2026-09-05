// Package agentapi serves the agent protocol: the API that agent backends
// speak.
//
// Every route runs as an authenticated agent, identified by its binding
// secret. This is the surface an outside developer builds against, so its
// shapes are part of the public contract and change only deliberately.
package agentapi

import (
	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
)

// Handler holds the services the agent API needs.
type Handler struct {
	agents        *agents.Service
	delivery      *delivery.Service
	conversations *conversations.Service
	hub           *realtime.Hub
	publisher     realtime.Publisher
}

// New builds the agent API handler.
func New(agentService *agents.Service, deliveryService *delivery.Service, conversationService *conversations.Service,
	hub *realtime.Hub, publisher realtime.Publisher) *Handler {
	return &Handler{
		agents: agentService, delivery: deliveryService, conversations: conversationService,
		hub: hub, publisher: publisher,
	}
}

// Routes returns the agent protocol routes, to be mounted behind agent
// authentication.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/me", h.getMe)
	r.Get("/events", h.listEvents)
	r.Get("/socket", h.socket)

	r.Route("/conversations/{id}", func(r chi.Router) {
		r.Get("/", h.getConversation)
		r.Get("/messages", h.listMessages)
		r.Post("/messages", h.sendMessage)
	})

	return r
}
