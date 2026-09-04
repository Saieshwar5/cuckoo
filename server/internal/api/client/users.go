package client

import (
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// userResponse is the wire shape of an account.
//
// It is written by hand rather than reusing the domain type so that renaming a
// field in Go can never silently change the API the app depends on.
type userResponse struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Locale      string    `json:"locale"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func newUserResponse(u users.User) userResponse {
	return userResponse{
		ID:          domain.FormatID(domain.PrefixUser, u.ID),
		DisplayName: u.DisplayName,
		Locale:      u.Locale,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
}

// meEnvelope keeps the account under a named key so the response can gain
// siblings later without breaking anyone parsing it.
type meEnvelope struct {
	User userResponse `json:"user"`
}

// getMe returns the signed-in account.
func (h *Handler) getMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}

	user, err := h.users.Get(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, meEnvelope{User: newUserResponse(user)})
}

// updateMeRequest is a partial update: an omitted field stays as it is.
//
// Pointers distinguish "not sent" from "sent as empty", which is the difference
// between leaving a name alone and trying to erase it.
type updateMeRequest struct {
	DisplayName *string `json:"display_name"`
	Locale      *string `json:"locale"`
}

// updateMe changes the signed-in account's profile.
func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}

	var req updateMeRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}

	user, err := h.users.UpdateProfile(r.Context(), userID, users.UpdateProfileInput{
		DisplayName: req.DisplayName,
		Locale:      req.Locale,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, meEnvelope{User: newUserResponse(user)})
}
