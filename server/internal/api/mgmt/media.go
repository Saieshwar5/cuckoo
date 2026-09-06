package mgmt

import (
	"net/http"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// mediaResponse is an uploaded picture. The management API uploads faces
// and nothing else, so this is smaller than the client API's shape.
type mediaResponse struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	MimeType  string    `json:"mime_type"`
	ByteSize  int64     `json:"byte_size"`
	FileName  string    `json:"file_name"`
	Width     int32     `json:"width,omitempty"`
	Height    int32     `json:"height,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type mediaEnvelope struct {
	Media mediaResponse `json:"media"`
}

// uploadMedia stores a picture for a profile to be published with.
//
// It is here rather than only on the client API because of who does it. A
// company's servers hold an API key, and a key does what its owner can do
// in the management API — creating agents, connecting backends, minting
// codes. Publishing an agent with its logo is that same job, and it would
// be a strange rule that let a key create a bank's agent but not give it a
// face. The file is owned by the person the key belongs to, exactly as if
// they had uploaded it from the app.
func (h *Handler) uploadMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	body, name, err := httpx.Upload(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	file, err := h.media.Upload(r.Context(), media.Owner{Kind: media.OwnerUser, ID: userID}, name,
		media.Meta{}, body)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, mediaEnvelope{Media: mediaResponse{
		ID:        domain.FormatID(domain.PrefixMedia, file.ID),
		Kind:      file.Kind,
		MimeType:  file.MimeType,
		ByteSize:  file.ByteSize,
		FileName:  file.FileName,
		Width:     file.Width,
		Height:    file.Height,
		CreatedAt: file.CreatedAt,
	}})
}
