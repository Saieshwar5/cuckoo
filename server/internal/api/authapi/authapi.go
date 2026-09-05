// Package authapi serves sign-in: proving an email address and getting a
// session, and giving the session up.
//
// Start and verify are the only endpoints on the hub that need no
// credential. Sign-out needs the session it ends.
package authapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// Handler holds the sign-in service.
type Handler struct {
	signin *signin.Service
}

// New builds the sign-in API handler.
func New(s *signin.Service) *Handler { return &Handler{signin: s} }

// Routes returns the sign-in routes. requireUser guards sign-out, which is
// the one route here that needs a caller.
func (h *Handler) Routes(requireUser func(http.Handler) http.Handler) chi.Router {
	r := chi.NewRouter()

	r.Post("/email/start", h.start)
	r.Post("/email/verify", h.verify)
	r.With(requireUser).Post("/logout", h.logout)

	return r
}

type startRequest struct {
	Email string `json:"email"`
}

// start sends a code. The response is the same whether or not an account
// exists for the address.
func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	var req startRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.signin.Start(r.Context(), req.Email); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

type verifyRequest struct {
	Email      string `json:"email"`
	Code       string `json:"code"`
	DeviceName string `json:"device_name"`
}

// userResponse is the account as the sign-in response carries it. Its own
// type: this is a different contract from the profile endpoint's, even
// though today it says the same things.
type userResponse struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Locale      string    `json:"locale"`
	CreatedAt   time.Time `json:"created_at"`
}

type sessionResponse struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// verifiedResponse is what a successful sign-in returns. The token is the
// credential for every request from now on and is never shown again.
type verifiedResponse struct {
	Token   string          `json:"token"`
	Session sessionResponse `json:"session"`
	User    userResponse    `json:"user"`
	IsNew   bool            `json:"is_new"`
}

func newUserResponse(u users.User) userResponse {
	return userResponse{
		ID:          domain.FormatID(domain.PrefixUser, u.ID),
		DisplayName: u.DisplayName,
		Locale:      u.Locale,
		CreatedAt:   u.CreatedAt,
	}
}

// verify checks a code and returns a session.
func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	v, err := h.signin.Verify(r.Context(), req.Email, req.Code, req.DeviceName)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, verifiedResponse{
		Token: v.Token,
		Session: sessionResponse{
			ID:        domain.FormatID(domain.PrefixSession, v.Session.ID),
			ExpiresAt: v.Session.ExpiresAt,
		},
		User:  newUserResponse(v.User),
		IsNew: v.IsNew,
	})
}

// logout ends the session the caller presented.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	sessionID, ok := principal.SessionID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Invalid("no_session",
			"Sign-out applies to a session token, not to the development header."))
		return
	}
	if err := h.signin.Logout(r.Context(), sessionID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
