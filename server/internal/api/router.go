// Package api assembles the server's HTTP surface.
//
// The three APIs — the app's, the agent protocol's, and agent owners' — are
// mounted here under separate prefixes with separate authentication. Handlers
// live in sub-packages; this file only decides what is reachable and what
// guards it.
package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Saieshwar5/cuckoo/server/internal/api/client"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/api/middleware"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Deps is everything the HTTP layer needs, supplied by main.
//
// Passing dependencies explicitly rather than reaching for globals is what
// makes the whole router constructible in a test in one line.
type Deps struct {
	Logger *slog.Logger
	Auth   middleware.Authenticator
	Users  *users.Service
	Health map[string]HealthCheck
}

// NewRouter builds the server's HTTP handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(middleware.Logger(d.Logger))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, domain.NotFound("route_not_found", "No such endpoint."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, domain.Invalid("method_not_allowed",
			"That method is not allowed on this endpoint."))
	})

	r.Get("/healthz", healthHandler(d.Health))

	// The app's API. Every route below requires a signed-in person.
	r.Route("/v1/client", func(r chi.Router) {
		r.Use(middleware.RequireUser(d.Auth))
		r.Mount("/", client.New(d.Users).Routes())
	})

	// /v1/agent — the agent protocol — and /v1/mgmt — agent management — mount
	// here next, each behind its own authentication.

	return r
}
