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

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/agentapi"
	"github.com/Saieshwar5/cuckoo/server/internal/api/authapi"
	"github.com/Saieshwar5/cuckoo/server/internal/api/client"
	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/api/mgmt"
	"github.com/Saieshwar5/cuckoo/server/internal/api/middleware"
	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/retention"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Deps is everything the HTTP layer needs, supplied by main.
//
// Passing dependencies explicitly rather than reaching for globals is what
// makes the whole router constructible in a test in one line.
type Deps struct {
	Logger *slog.Logger

	// UserAuth identifies people; AgentAuth identifies agent backends. They
	// are different credentials producing different principals, and each API
	// below accepts exactly one of them.
	UserAuth  middleware.Authenticator
	AgentAuth middleware.Authenticator
	// MgmtAuth identifies an agent's owner on the management API: a person
	// signed in on their phone, or a server holding that person's API key.
	MgmtAuth middleware.Authenticator

	Users         *users.Service
	SignIn        *signin.Service
	Agents        *agents.Service
	Conversations *conversations.Service
	Delivery      *delivery.Service
	Pairing       *pairing.Service
	APIKeys       *apikeys.Service
	Media         *media.Service
	// PublicURL is where people reach this hub from outside: the base of
	// the addresses inside a page a stranger opens.
	PublicURL string
	Hub       *realtime.Hub
	Bus       realtime.Publisher
	// CORSOrigins are the browser origins allowed to call the API; empty
	// means browsers are not served at all.
	CORSOrigins []string
	// Retention is the hub's window: what a person may keep and for how
	// long, which the app shows and which decides when history is trimmed.
	Retention *retention.Service
	Health    map[string]HealthCheck
	// Version is the build stamp shown on /healthz; empty reads as "dev".
	Version string
}

// NewRouter builds the server's HTTP handler.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	// chi's RealIP is deliberately absent. It rewrites RemoteAddr from
	// X-Forwarded-For / X-Real-IP, which any client can forge, so the moment
	// anything trusts RemoteAddr — rate limiting by address, abuse blocking —
	// a caller could spoof another user's address or evade their own limit.
	// When real client addresses are needed, they must come from a proxy we
	// configure and trust by name, not from a header we accept from anyone.
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(middleware.Logger(d.Logger))
	r.Use(middleware.CORS(d.CORSOrigins))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, domain.NotFound("route_not_found", "No such endpoint."))
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.Error(w, r, domain.Invalid("method_not_allowed",
			"That method is not allowed on this endpoint."))
	})

	r.Get("/healthz", healthHandler(d.Health, d.Version))

	// What a QR code opens for someone without the app, and the picture on
	// it. Both are public: this is the moment a stranger decides whether to
	// trust an agent, and it happens before they have an account.
	r.Get("/p/{code}", pairPageHandler(d.Pairing, d.PublicURL))
	r.Get("/a/{id}/avatar", agentAvatarHandler(d.Media))

	// Signing in: the only routes a person reaches before they have a
	// credential, plus signing out, which needs the one it ends.
	r.Route("/v1/auth", func(r chi.Router) {
		r.Mount("/", authapi.New(d.SignIn).Routes(middleware.RequireUser(d.UserAuth)))
	})

	// The app's API. Every route below requires a signed-in person.
	r.Route("/v1/client", func(r chi.Router) {
		r.Use(middleware.RequireUser(d.UserAuth))
		r.Mount("/", client.New(d.Users, d.Agents, d.Conversations, d.Hub, d.Pairing, d.APIKeys, d.Media, d.Retention).Routes())
	})

	// Agent management: owners creating agents, connecting backends and
	// minting the codes that hand them out. A signed-in person, or a server
	// holding that person's API key — the same routes either way, because
	// they are the same powers.
	r.Route("/v1/mgmt", func(r chi.Router) {
		r.Use(middleware.RequireUser(d.MgmtAuth))
		r.Mount("/", mgmt.New(d.Agents, d.Pairing, d.Media).Routes())
	})

	// The agent protocol. Only a binding secret gets in.
	r.Route("/v1/agent", func(r chi.Router) {
		r.Use(middleware.RequireAgent(d.AgentAuth))
		r.Mount("/", agentapi.New(d.Agents, d.Delivery, d.Conversations, d.Hub, d.Bus, d.Media).Routes())
	})

	return r
}
