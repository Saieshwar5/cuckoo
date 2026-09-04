// Package client serves the API the mobile app talks to.
//
// Every route here runs behind user authentication and acts on behalf of the
// person holding the device. The agent protocol and the management API are
// separate packages with their own authentication, so a mistake in one can
// never widen the surface of another.
package client

import (
	"github.com/go-chi/chi/v5"

	"github.com/cuckoo-chat/cuckoo/server/internal/users"
)

// Handler holds the services the client API needs.
type Handler struct {
	users *users.Service
}

// New builds the client API handler.
func New(userService *users.Service) *Handler {
	return &Handler{users: userService}
}

// Routes returns the client API routes, to be mounted behind authentication.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/me", h.getMe)
	r.Patch("/me", h.updateMe)

	return r
}
