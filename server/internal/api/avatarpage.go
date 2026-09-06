package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
)

// agentAvatarHandler serves an agent's published picture to anyone.
//
// This is the only route in Cuckoo that hands out bytes with no credential,
// and it exists for one moment: a stranger has scanned a code, the page has
// opened in a browser, and they are deciding whether to trust what they see.
// A grey disc and the word "Unverified" is not an argument. Its owner
// published this picture to be shown to exactly these people.
//
// Only the small copy is served. Nobody needs the original of a face, and a
// public route that hands out sixteen megabytes on request is a way to be
// knocked over.
func agentAvatarHandler(m *media.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := domain.ParseID(domain.PrefixAgent, chi.URLParam(r, "id"))
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		file, body, err := m.OpenAgentAvatar(r.Context(), id)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		defer func() { _ = body.Close() }()

		// A picture, inline, with no length: the small copy's size is not
		// something this server keeps count of.
		mimeType := file.MimeType
		if file.HasThumbnail {
			mimeType = "image/jpeg"
		}
		httpx.ServeFile(w, r, body, media.KindImage, mimeType, file.FileName, 0, true)
	}
}
