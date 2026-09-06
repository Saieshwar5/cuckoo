package client

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// mediaResponse is an uploaded file. The id is what a send names to attach
// it; nothing here says where the bytes are, because they are fetched from
// this same endpoint by id.
type mediaResponse struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	MimeType     string    `json:"mime_type"`
	ByteSize     int64     `json:"byte_size"`
	FileName     string    `json:"file_name"`
	Width        int32     `json:"width,omitempty"`
	Height       int32     `json:"height,omitempty"`
	HasThumbnail bool      `json:"has_thumbnail"`
	CreatedAt    time.Time `json:"created_at"`
}

type mediaEnvelope struct {
	Media mediaResponse `json:"media"`
}

func newMediaResponse(f media.File) mediaResponse {
	return mediaResponse{
		ID:           domain.FormatID(domain.PrefixMedia, f.ID),
		Kind:         f.Kind,
		MimeType:     f.MimeType,
		ByteSize:     f.ByteSize,
		FileName:     f.FileName,
		Width:        f.Width,
		Height:       f.Height,
		HasThumbnail: f.HasThumbnail,
		CreatedAt:    f.CreatedAt,
	}
}

// mediaIDsOf parses the ids a send is attaching.
func mediaIDsOf(raw []string) ([]uuid.UUID, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(raw))
	for _, s := range raw {
		id, err := domain.ParseID(domain.PrefixMedia, s)
		if err != nil {
			return nil, domain.InvalidField("attachments", "invalid_id",
				"Attachments are the ids of files already uploaded.")
		}
		out = append(out, id)
	}
	return out, nil
}

// uploadMedia stores a file this person is about to send. It is theirs and
// invisible to anyone else until a message carries it.
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
	file, err := h.media.Upload(r.Context(), media.Owner{Kind: media.OwnerUser, ID: userID}, name, body)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, mediaEnvelope{Media: newMediaResponse(file)})
}

// getMedia sends the bytes, to someone in the conversation the file was
// sent into, or to whoever uploaded it. ?variant=thumb is the small copy of
// a picture, which is what a bubble draws.
func (h *Handler) getMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return
	}
	id, err := domain.ParseID(domain.PrefixMedia, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	variant := r.URL.Query().Get("variant")
	file, body, err := h.media.OpenForUser(r.Context(), userID, id, variant)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	size := file.ByteSize
	mimeType := file.MimeType
	if variant == media.VariantThumb {
		// The small copy is a different number of bytes, and this server
		// does not keep count of them.
		size = 0
		mimeType = "image/jpeg"
	}
	httpx.ServeFile(w, r, body, file.Kind, mimeType, file.FileName, size, true)
}
