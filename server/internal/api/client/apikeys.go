package client

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/apikeys"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// apiKeyResponse is a key as its owner sees it: everything but the key.
type apiKeyResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

func newAPIKeyResponse(k apikeys.Key) apiKeyResponse {
	return apiKeyResponse{
		ID:         domain.FormatID(domain.PrefixAPIKey, k.ID),
		Name:       k.Name,
		LastUsedAt: k.LastUsedAt,
		CreatedAt:  k.CreatedAt,
		RevokedAt:  k.RevokedAt,
	}
}

// createAPIKeyEnvelope carries the key alongside its record. This is the
// only response in the whole API that ever contains it.
type createAPIKeyEnvelope struct {
	APIKey apiKeyResponse `json:"api_key"`
	Key    string         `json:"key"`
}

type apiKeyListEnvelope struct {
	APIKeys []apiKeyResponse `json:"api_keys"`
}

type createAPIKeyRequest struct {
	Name string `json:"name"`
}

// createAPIKey issues a key. These routes run on the client API, behind a
// person's own credential, so a key can never mint another key: widening
// access stays something a human does, in the app, signed in.
func (h *Handler) createAPIKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	var req createAPIKeyRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	key, plaintext, err := h.apiKeys.Create(r.Context(), userID, req.Name)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, createAPIKeyEnvelope{
		APIKey: newAPIKeyResponse(key),
		Key:    plaintext,
	})
}

func (h *Handler) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	list, err := h.apiKeys.ListMine(r.Context(), userID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]apiKeyResponse, 0, len(list))
	for _, k := range list {
		out = append(out, newAPIKeyResponse(k))
	}
	httpx.JSON(w, r, http.StatusOK, apiKeyListEnvelope{APIKeys: out})
}

func (h *Handler) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := domain.ParseID(domain.PrefixAPIKey, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.apiKeys.Revoke(r.Context(), userID, id); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
