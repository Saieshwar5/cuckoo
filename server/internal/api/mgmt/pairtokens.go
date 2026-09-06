package mgmt

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skip2/go-qrcode"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// pairTokenResponse is a token as its owner sees it: everything but the
// secret in the link.
type pairTokenResponse struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Payload   json.RawMessage `json:"payload"`
	MaxUses   *int            `json:"max_uses"`
	UseCount  int             `json:"use_count"`
	ExpiresAt *time.Time      `json:"expires_at"`
	CreatedAt time.Time       `json:"created_at"`
	RevokedAt *time.Time      `json:"revoked_at"`
}

func newPairTokenResponse(t pairing.Token) pairTokenResponse {
	payload := t.Payload
	if len(payload) == 0 {
		payload = json.RawMessage("null")
	}
	return pairTokenResponse{
		ID:        domain.FormatID(domain.PrefixToken, t.ID),
		Kind:      t.Kind,
		Payload:   payload,
		MaxUses:   t.MaxUses,
		UseCount:  t.UseCount,
		ExpiresAt: t.ExpiresAt,
		CreatedAt: t.CreatedAt,
		RevokedAt: t.RevokedAt,
	}
}

type createPairTokenRequest struct {
	Payload json.RawMessage `json:"payload"`
	MaxUses *int            `json:"max_uses"`
	// ExpiresIn is in seconds.
	ExpiresIn *int `json:"expires_in"`
}

// createPairTokenEnvelope carries the link and the QR code alongside the
// token. This is the only response that ever contains the secret.
type createPairTokenEnvelope struct {
	Token pairTokenResponse `json:"token"`
	Code  string            `json:"code"`
	URL   string            `json:"url"`
	// QRPNG is a data URL an app can put straight into an image.
	QRPNG string `json:"qr_png"`
}

type pairTokenListEnvelope struct {
	Tokens []pairTokenResponse `json:"tokens"`
}

// qrSize is the rendered code in pixels: sharp on a phone, small on the wire.
const qrSize = 512

func (h *Handler) createPairToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req createPairTokenRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	in := pairing.CreateTokenInput{Payload: req.Payload, MaxUses: req.MaxUses}
	if req.ExpiresIn != nil {
		d := time.Duration(*req.ExpiresIn) * time.Second
		in.ExpiresIn = &d
	}

	token, code, err := h.pairing.CreateToken(r.Context(), userID, id, in)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	url := h.pairing.URL(code)
	png, err := qrcode.Encode(url, qrcode.Medium, qrSize)
	if err != nil {
		httpx.Error(w, r, domain.Internal(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, createPairTokenEnvelope{
		Token: newPairTokenResponse(token),
		Code:  code,
		URL:   url,
		QRPNG: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	})
}

// pictureEnvelope is the QR picture of a code the device kept.
type pictureEnvelope struct {
	URL   string `json:"url"`
	QRPNG string `json:"qr_png"`
}

func (h *Handler) pairTokenPicture(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	tokenID, err := domain.ParseID(domain.PrefixToken, chi.URLParam(r, "tid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	url, err := h.pairing.Picture(r.Context(), userID, id, tokenID, r.URL.Query().Get("code"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	png, err := qrcode.Encode(url, qrcode.Medium, qrSize)
	if err != nil {
		httpx.Error(w, r, domain.Internal(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, pictureEnvelope{
		URL:   url,
		QRPNG: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	})
}

func (h *Handler) listPairTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	list, err := h.pairing.ListTokens(r.Context(), userID, id)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	out := make([]pairTokenResponse, 0, len(list))
	for _, t := range list {
		out = append(out, newPairTokenResponse(t))
	}
	httpx.JSON(w, r, http.StatusOK, pairTokenListEnvelope{Tokens: out})
}

func (h *Handler) revokePairToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := agentIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	tokenID, err := domain.ParseID(domain.PrefixToken, chi.URLParam(r, "tid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.pairing.RevokeToken(r.Context(), userID, id, tokenID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}
