package client

import (
	"net/http"
	"time"

	"github.com/google/uuid"

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
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Locale      string `json:"locale"`
	// The person's photo, as a media id. Unlike an agent's logo it is not
	// public: it is fetched from /media/{id}, which only they can read.
	AvatarMediaID *string   `json:"avatar_media_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func newUserResponse(u users.User) userResponse {
	resp := userResponse{
		ID:          domain.FormatID(domain.PrefixUser, u.ID),
		DisplayName: u.DisplayName,
		Locale:      u.Locale,
		CreatedAt:   u.CreatedAt,
		UpdatedAt:   u.UpdatedAt,
	}
	if u.AvatarMediaID != nil {
		id := domain.FormatID(domain.PrefixMedia, *u.AvatarMediaID)
		resp.AvatarMediaID = &id
	}
	return resp
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
	AvatarMediaID *string `json:"avatar_media_id"`
	DisplayName   *string `json:"display_name"`
	Locale        *string `json:"locale"`
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

	var avatar *uuid.UUID
	if req.AvatarMediaID != nil {
		id, err := domain.ParseID(domain.PrefixMedia, *req.AvatarMediaID)
		if err != nil {
			httpx.Error(w, r, domain.InvalidField("avatar_media_id", "invalid_id",
				"avatar_media_id is the id of a picture already uploaded."))
			return
		}
		avatar = &id
	}

	user, err := h.users.UpdateProfile(r.Context(), userID, users.UpdateProfileInput{
		AvatarMediaID: avatar,
		DisplayName:   req.DisplayName,
		Locale:        req.Locale,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	httpx.JSON(w, r, http.StatusOK, meEnvelope{User: newUserResponse(user)})
}
