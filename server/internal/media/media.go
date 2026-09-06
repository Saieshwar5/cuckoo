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
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
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

	// maxDurationMS is the longest a recording may say it is: ten minutes.
	// A limit in bytes alone would allow an hour of speech, which is not a
	// message, and the number is the sender's claim, so it is bounded like
	// any other thing a caller says.
	maxDurationMS = 10 * 60 * 1000

	// waveformMax is how many loudness values a recording may carry. Fifty
	// or so bars is what fits across a bubble; more would be detail nobody
	// can see, stored in every copy of the message.
	waveformMax = 64

	// waveformPeak is the top of the scale. The values are a shape, not a
	// measurement, so they are plain small numbers rather than decibels.
	waveformPeak = 100

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
	// How long a recording or a video runs, in milliseconds, and its shape
	// over time. Both come from whoever recorded it; see the migration.
	DurationMS int32
	Waveform   []int32
	CreatedAt  time.Time
}

// Meta is what a caller can tell the hub about a file that the hub cannot
// see for itself.
type Meta struct {
	DurationMS int32
	Waveform   []int32
	// Audio says a container that could hold either is a recording. See
	// resolveKind.
	Audio bool
}

// MetaFromQuery reads what an upload declared about itself.
//
//	?duration_ms=8200&waveform=3,9,40,88,12&kind=audio
func MetaFromQuery(q url.Values) (Meta, error) {
	var meta Meta
	if raw := q.Get("duration_ms"); raw != "" {
		ms, err := strconv.Atoi(raw)
		if err != nil || ms < 0 || ms > maxDurationMS {
			return Meta{}, domain.InvalidField("duration_ms", "invalid_duration",
				fmt.Sprintf("Duration must be a whole number of milliseconds, at most %d.", maxDurationMS))
		}
		meta.DurationMS = int32(ms) // #nosec G109 -- bounded by maxDurationMS above
	}
	if raw := q.Get("waveform"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) > waveformMax {
			return Meta{}, domain.InvalidField("waveform", "invalid_waveform",
				fmt.Sprintf("A waveform is at most %d values.", waveformMax))
		}
		meta.Waveform = make([]int32, 0, len(parts))
		for _, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 0 || n > waveformPeak {
				return Meta{}, domain.InvalidField("waveform", "invalid_waveform",
					fmt.Sprintf("A waveform is whole numbers from 0 to %d, separated by commas.", waveformPeak))
			}
			meta.Waveform = append(meta.Waveform, int32(n)) // #nosec G109 -- bounded by waveformPeak above
		}
	}
	meta.Audio = q.Get("kind") == KindAudio
	return meta, nil
}

// forKind drops what does not apply. A picture has no duration, and a
// document has no shape; carrying a number a caller sent anyway would put
// it in front of people in a bubble.
func (m Meta) forKind(kind string) Meta {
	if kind == KindAudio || kind == KindVideo {
		return m
	}
	return Meta{}
}

func fromRow(r gen.Medium) File {
	return File{
		DurationMS:   r.DurationMs,
		Waveform:     r.Waveform,
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

// resolveKind decides what a file is.
//
// The bytes decide, with one exception: WebM, Ogg and MP4 are containers
// that hold either sound or pictures, and telling which without parsing
// them means decoding a stranger's media. So a caller may say "this is a
// recording" about one of those, and nothing else — a claim can only turn
// a video into audio, never a document into a picture. The cost of a lie
// is a waveform drawn over a video.
func resolveKind(mimeType string, meta Meta) string {
	kind := kindOf(mimeType)
	if meta.Audio && kind == KindVideo {
		return KindAudio
	}
	return kind
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

// Picture is what a profile may point at: an image, uploaded by whoever is
// setting it, and not already spoken for by a message.
//
// The rule lives here, once, and is applied by every profile that can have
// a face. A caller naming someone else's upload and one naming an id that
// does not exist get the same answer, so ids cannot be probed for.
func Picture(row gen.Medium, found bool, owner Owner) error {
	if !found || row.OwnerKind != owner.Kind || row.OwnerID != owner.ID {
		return domain.InvalidField("avatar_media_id", "unknown_picture",
			"That picture is not yours to use, or does not exist.")
	}
	if row.Kind != KindImage {
		return domain.InvalidField("avatar_media_id", "not_a_picture", "A face has to be a picture.")
	}
	if row.MessageID != nil {
		return domain.InvalidField("avatar_media_id", "picture_already_sent",
			"That picture was sent in a message. Upload it again to use it as a face.")
	}
	return nil
}
