// Package client serves the API the mobile app talks to.
//
// Every route here runs behind user authentication and acts on behalf of the
// person holding the device. The agent protocol and the management API are
// separate packages with their own authentication, so a mistake in one can
// never widen the surface of another.
package client

import (
	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Handler holds the services the client API needs.
type Handler struct {
	users         *users.Service
	conversations *conversations.Service
	hub           *realtime.Hub
}

// New builds the client API handler.
func New(userService *users.Service, conversationService *conversations.Service, hub *realtime.Hub) *Handler {
	return &Handler{users: userService, conversations: conversationService, hub: hub}
}

// Routes returns the client API routes, to be mounted behind authentication.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/me", h.getMe)
	r.Patch("/me", h.updateMe)

	r.Get("/socket", h.socket)

	r.Get("/conversations", h.listConversations)
	r.Route("/conversations/{id}", func(r chi.Router) {
		r.Get("/", h.getConversation)
		r.Get("/messages", h.listMessages)
		r.Post("/messages", h.sendMessage)
	})

	return r
}
