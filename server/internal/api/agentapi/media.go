package agentapi

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

// mediaResponse is an uploaded file, as a backend sees it. The same shape
// the app gets: one protocol, two callers.
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

// uploadMedia stores a file this agent is about to send.
func (h *Handler) uploadMedia(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}
	body, name, err := httpx.Upload(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	file, err := h.media.Upload(r.Context(), media.Owner{Kind: media.OwnerAgent, ID: agentID}, name, body)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, mediaEnvelope{Media: newMediaResponse(file)})
}

// getMedia sends the bytes of a file on a message in one of this agent's
// conversations — the photo a person just sent it — or of its own upload.
func (h *Handler) getMedia(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}
	id, err := domain.ParseID(domain.PrefixMedia, chi.URLParam(r, "id"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	variant := r.URL.Query().Get("variant")
	file, body, err := h.media.OpenForAgent(r.Context(), agentID, id, variant)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	defer func() { _ = body.Close() }()

	size := file.ByteSize
	mimeType := file.MimeType
	if variant == media.VariantThumb {
		size = 0
		mimeType = "image/jpeg"
	}
	// A backend saves what it downloads to a path it chose; nothing here is
	// rendered in a browser, so the bytes go with their own type.
	httpx.ServeFile(w, r, body, file.Kind, mimeType, file.FileName, size, false)
}
