package client

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/signin"
)

// deviceResponse is one signed-in device as its owner sees it. LastSeen is
// null until the device has carried a request; Current marks the one asking,
// which the app draws differently and never offers to sign out by accident.
type deviceResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	LastSeen  *time.Time `json:"last_seen_at"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	Current   bool       `json:"current"`
}

type devicesEnvelope struct {
	Devices []deviceResponse `json:"devices"`
}

// listDevices serves GET /v1/client/devices.
// registerPushRequest is where this device can be reached when nobody is
// looking at it.
type registerPushRequest struct {
	// An Expo token, "ExponentPushToken[…]". Opaque to the hub.
	Token string `json:"token"`
	// "android" or "ios". Empty is accepted: the token is what matters.
	Platform string `json:"platform"`
}

// registerPush records the address of the device this session belongs to.
//
// The app calls it after signing in and again whenever the token changes —
// they rotate, and a stale one is a notification that silently goes nowhere.
func (h *Handler) registerPush(w http.ResponseWriter, r *http.Request) {
	if _, ok := principal.UserID(r.Context()); !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	sessionID, ok := principal.SessionID(r.Context())
	if !ok {
		// The development header has no session, and a notification address
		// with no device to belong to is meaningless.
		httpx.Error(w, r, domain.Forbidden("no_session",
			"Notifications are registered by a signed-in device."))
		return
	}
	var req registerPushRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.signIn.RegisterPush(r.Context(), sessionID, req.Token, req.Platform); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	// The session asking is what marks one row "this device"; the
	// development header has no session, and then nothing is current.
	sessionID, _ := principal.SessionID(r.Context())
	devices, err := h.signIn.Devices(r.Context(), userID, sessionID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := devicesEnvelope{Devices: make([]deviceResponse, 0, len(devices))}
	for _, d := range devices {
		out.Devices = append(out.Devices, newDeviceResponse(d))
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

// signOutDevice serves DELETE /v1/client/devices/{id}. Signing out the
// device asking is allowed and is the same as signing out.
func (h *Handler) signOutDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	sessionID, err := domain.ParseID(domain.PrefixSession, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.signIn.SignOutDevice(r.Context(), userID, sessionID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// signOutOtherDevices serves DELETE /v1/client/devices: everything but this
// one, which is what a person taps when a phone is lost.
func (h *Handler) signOutOtherDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	sessionID, _ := principal.SessionID(r.Context())
	if _, err := h.signIn.SignOutOthers(r.Context(), userID, sessionID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func newDeviceResponse(d signin.Device) deviceResponse {
	return deviceResponse{
		ID:        domain.FormatID(domain.PrefixSession, d.ID),
		Name:      d.Name,
		LastSeen:  d.LastSeen,
		CreatedAt: d.CreatedAt,
		ExpiresAt: d.ExpiresAt,
		Current:   d.Current,
	}
}
