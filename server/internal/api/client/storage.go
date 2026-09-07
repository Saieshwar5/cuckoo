package client

import (
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
	"github.com/Saieshwar5/cuckoo/server/internal/retention"
)

// storageEnvelope is what a person keeps on the hub and how long it stays:
// the line the Settings screen shows. KeptDays is 0 and KeptSince null when
// messages are kept forever.
type storageEnvelope struct {
	Media    storageMedia    `json:"media"`
	Messages storageMessages `json:"messages"`
}

type storageMedia struct {
	UsedBytes   int64 `json:"used_bytes"`
	BudgetBytes int64 `json:"budget_bytes"`
}

type storageMessages struct {
	KeptDays  int        `json:"kept_days"`
	KeptSince *time.Time `json:"kept_since"`
}

// getStorage serves GET /v1/client/me/storage.
func (h *Handler) getStorage(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	usage, err := h.retention.Usage(r.Context(), retention.OwnerUser, userID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := storageEnvelope{Media: storageMedia{UsedBytes: usage.UsedBytes, BudgetBytes: usage.BudgetBytes}}
	if since, kept := h.retention.Window(); kept {
		out.Messages.KeptDays = int(h.retention.Policy().MessageAge / (24 * time.Hour))
		out.Messages.KeptSince = &since
	}
	httpx.JSON(w, r, http.StatusOK, out)
}
