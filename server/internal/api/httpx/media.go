package httpx

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// ServeFile writes a stored file to the response.
//
// A picture, a video or a recording is served with its own type so a phone
// or a browser can show it. Everything else is served as a download of
// unnamed bytes, whatever it really is: these are files strangers uploaded,
// and a hub that renders one in a browser on its own origin has handed that
// stranger a page on it. Combined with nosniff, that is the whole of the
// defence, so it does not depend on anyone downstream remembering.
func ServeFile(w http.ResponseWriter, r *http.Request, body io.Reader,
	kind, mimeType, fileName string, size int64, inlineOK bool) {

	inline := inlineOK && (strings.HasPrefix(kind, "image") ||
		strings.HasPrefix(kind, "video") || strings.HasPrefix(kind, "audio"))

	w.Header().Set("X-Content-Type-Options", "nosniff")
	// A file's bytes never change: it is addressed by an id that was
	// created with them. Private, because who may read it was just decided.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	if inline {
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", "inline; filename="+strconv.Quote(fileName))
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename="+strconv.Quote(fileName))
	}
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}

	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, body); err != nil {
		// The status and headers are long gone; all that is left is to say
		// what happened where someone can see it.
		slog.WarnContext(r.Context(), "could not finish sending file",
			"path", r.URL.Path, "error", fmt.Sprintf("%v", err))
	}
}
