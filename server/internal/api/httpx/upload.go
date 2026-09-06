package httpx

import (
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// Upload finds the file in an upload request and returns it unread, so the
// bytes stream to storage instead of filling this process's memory.
//
// Two shapes are accepted, because two very different callers upload here.
// A phone sends a multipart form, which is what its picker and its fetch
// implementation produce. A script sends the bytes as the body with the
// name in the query, which is what one line of curl produces. Both hand
// back the same thing: something to read, and a name.
func Upload(r *http.Request) (io.Reader, string, error) {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		contentType = ""
	}
	if !strings.HasPrefix(contentType, "multipart/") {
		return r.Body, r.URL.Query().Get("name"), nil
	}

	mr, err := r.MultipartReader()
	if err != nil {
		return nil, "", domain.Invalid("invalid_upload", "That upload is not a valid form.")
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return nil, "", domain.InvalidField("file", "missing_file", "Attach the file as the field named file.")
		}
		if err != nil {
			return nil, "", domain.Invalid("invalid_upload", "That upload is not a valid form.")
		}
		if part.FormName() == "file" {
			name := part.FileName()
			if name == "" {
				name = r.URL.Query().Get("name")
			}
			return part, name, nil
		}
	}
}
