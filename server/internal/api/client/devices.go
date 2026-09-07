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
