// Package media holds the files a message carries: photos, voice notes,
// documents.
//
// Uploading is its own step. A file goes to the hub first and comes back as
// an id; the send that follows names that id. That is what makes a picture
// on a slow connection behave: the bytes take as long as they take, and the
// message is created in one quick call at the end, or never, leaving no
// half-sent bubble behind.
//
// What a file is, is decided here from the bytes themselves. A caller that
// says "image/png" and sends a video is a video, and a caller that renames
// a program to .jpg has not made it a picture.
package media

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Kinds. The protocol names four, because a phone draws each differently:
// a picture inline, a video with a play triangle, a voice note as a
// waveform, anything else as a card with a name and a size.
const (
	KindImage = "image"
	KindVideo = "video"
	KindAudio = "audio"
	KindFile  = "file"
)

// Owners. A file belongs to whoever uploaded it, and only they can attach
// it to a message.
const (
	OwnerUser  = "user"
	OwnerAgent = "agent"
)

// Size limits, per kind.
//
// India-first: a 16 MB photo is already far more than a phone camera
// produces after the app shrinks it, and 64 MB is a video worth sending on
// a mobile connection. They are the protocol's numbers, not a deployment's,
// so an agent written against one hub behaves the same on another.
const (
	MaxImageBytes = 16 << 20
	MaxAudioBytes = 16 << 20
	MaxVideoBytes = 64 << 20
	MaxFileBytes  = 64 << 20

	// sniffLen is what http.DetectContentType reads.
	sniffLen = 512

	// fileNameMaxLen matches the column.
	fileNameMaxLen = 255

	// thumbMaxPixels is the long side of the small copy of a picture: big
	// enough for a bubble on a dense screen, small enough to arrive at once.
	thumbMaxPixels = 480

	// maxImagePixels refuses a picture whose stated dimensions would cost
	// more memory to decode than it did to upload — a few kilobytes of
	// header can claim a gigabyte of pixels.
	maxImagePixels = 50_000_000
)

// Variants of one file.
const (
	VariantOriginal = ""
	VariantThumb    = "thumb"
)

// File is an uploaded file as the rest of the server sees it. The bytes are
// not here; they are opened separately, and only for someone allowed them.
type File struct {
	ID        uuid.UUID
	OwnerKind string
	OwnerID   uuid.UUID
	Kind      string
	MimeType  string
	ByteSize  int64
	FileName  string
	// Pixels, for a picture; zero for anything else.
	Width  int32
	Height int32
	// HasThumbnail is true when a small copy was made.
	HasThumbnail bool
	CreatedAt    time.Time
}

func fromRow(r gen.Medium) File {
	return File{
		ID:           r.ID,
		OwnerKind:    r.OwnerKind,
		OwnerID:      r.OwnerID,
		Kind:         r.Kind,
		MimeType:     r.MimeType,
		ByteSize:     r.ByteSize,
		FileName:     r.FileName,
		Width:        r.Width,
		Height:       r.Height,
		HasThumbnail: r.ThumbKey != nil,
		CreatedAt:    r.CreatedAt,
	}
}

// Owner is who uploaded a file.
type Owner struct {
	Kind string
	ID   uuid.UUID
}

// kindOf classifies sniffed bytes. http.DetectContentType knows the common
// formats and says application/octet-stream for the rest, which is exactly
// the honest answer for "some file".
func kindOf(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return KindImage
	case strings.HasPrefix(mimeType, "video/"):
		return KindVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return KindAudio
	default:
		return KindFile
	}
}

// limitFor is how many bytes that kind may be.
func limitFor(kind string) int64 {
	switch kind {
	case KindImage:
		return MaxImageBytes
	case KindAudio:
		return MaxAudioBytes
	case KindVideo:
		return MaxVideoBytes
	default:
		return MaxFileBytes
	}
}

// sniff names the content of a file from its first bytes.
func sniff(header []byte) string {
	mimeType := http.DetectContentType(header)
	// DetectContentType appends a charset for text; the type is the part
	// that decides anything here.
	if i := strings.IndexByte(mimeType, ';'); i > 0 {
		mimeType = strings.TrimSpace(mimeType[:i])
	}
	return mimeType
}

// cleanName reduces whatever a client sent to a name a file can be saved
// under: one segment, no separators, no control characters, never empty.
//
// The name is shown to people and used as the download name, so it is
// carried; it is never used to find the bytes, which live under a key this
// server generates.
func cleanName(raw string) string {
	name := strings.TrimSpace(raw)
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = strings.Map(func(r rune) rune {
		if r == 0 || unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." || name == "/" {
		name = ""
	}
	if name == "" {
		return "file"
	}
	if !utf8.ValidString(name) {
		return "file"
	}
	if len(name) > fileNameMaxLen {
		// Trim from the front of the extension so the suffix survives, which
		// is the part that decides how a phone opens it.
		ext := filepath.Ext(name)
		if len(ext) > 16 {
			ext = ""
		}
		name = name[:fileNameMaxLen-len(ext)] + ext
		name = strings.ToValidUTF8(name, "")
	}
	return name
}

// storageKey is where a file's bytes go. Two levels of fan-out keep any one
// directory small enough for a filesystem to be happy with, and the id
// stays readable in the path, which matters when someone is looking at a
// disk trying to understand what is on it.
func storageKey(id uuid.UUID, variant string) string {
	s := strings.ReplaceAll(id.String(), "-", "")
	key := s[0:2] + "/" + s[2:4] + "/" + s
	if variant != VariantOriginal {
		return key + "." + variant
	}
	return key
}
