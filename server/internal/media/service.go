package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// uploadPolicy is per uploader: a burst for picking several photos at once,
// then a steady trickle. Bytes are bounded per file; this bounds how many
// files, which is the part a size limit cannot see.
var (
	userUploadPolicy  = ratelimit.Policy{Rate: 0.5, Burst: 20}
	agentUploadPolicy = ratelimit.Policy{Rate: 5, Burst: 40}
)

// Store is the slice of the database this package uses.
type Store interface {
	CreateMedia(ctx context.Context, arg gen.CreateMediaParams) (gen.Medium, error)
	GetMedia(ctx context.Context, id uuid.UUID) (gen.Medium, error)
	UserCanReadMedia(ctx context.Context, arg gen.UserCanReadMediaParams) (bool, error)
	AgentCanReadMedia(ctx context.Context, arg gen.AgentCanReadMediaParams) (bool, error)
	DeleteUnclaimedMedia(ctx context.Context, before time.Time) ([]gen.DeleteUnclaimedMediaRow, error)
}

// Service holds the rules for uploading and reading files.
type Service struct {
	store   Store
	blobs   blobs.Store
	limiter ratelimit.Limiter
	log     *slog.Logger
}

// Option configures the service.
type Option func(*Service)

// WithLimiter bounds how fast one uploader may upload.
func WithLimiter(l ratelimit.Limiter) Option {
	return func(s *Service) { s.limiter = l }
}

// WithLogger sets where the service reports what it could not do but did
// not have to fail for.
func WithLogger(l *slog.Logger) Option {
	return func(s *Service) { s.log = l }
}

// New builds the service.
func New(st Store, b blobs.Store, opts ...Option) *Service {
	s := &Service{store: st, blobs: b, limiter: ratelimit.Unlimited{}, log: slog.Default()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Upload stores a file and records it, unattached. A later send names its
// id and takes ownership of it; until then it belongs to the uploader and
// nobody else can see it.
//
// What the file is, is read from the bytes. The name is carried along for
// people to read and for the download to be saved under, and is never used
// to decide anything.
func (s *Service) Upload(ctx context.Context, owner Owner, name string, meta Meta, r io.Reader) (File, error) {
	if err := s.allow(ctx, owner); err != nil {
		return File{}, err
	}

	// The first bytes say what this is, which says how many more of them
	// are allowed.
	header := make([]byte, sniffLen)
	n, err := io.ReadFull(r, header)
	switch {
	case errors.Is(err, io.EOF):
		return File{}, errEmpty()
	case err == nil, errors.Is(err, io.ErrUnexpectedEOF):
	default:
		return File{}, errIncomplete(err)
	}
	header = header[:n]

	mimeType := sniff(header)
	kind := resolveKind(mimeType, meta)
	meta = meta.forKind(kind)
	id := domain.NewID()
	key := storageKey(id, VariantOriginal)
	body := io.MultiReader(bytes.NewReader(header), r)

	var (
		size          int64
		width, height int32
		thumbKey      *string
	)
	if kind == KindImage {
		size, width, height, thumbKey, err = s.putImage(ctx, id, key, body)
	} else {
		size, err = s.putStream(ctx, key, body, kind)
	}
	if err != nil {
		s.discard(ctx, key, thumbKey)
		return File{}, err
	}
	if size == 0 {
		s.discard(ctx, key, thumbKey)
		return File{}, errEmpty()
	}

	row, err := s.store.CreateMedia(ctx, gen.CreateMediaParams{
		ID:         id,
		OwnerKind:  owner.Kind,
		OwnerID:    owner.ID,
		Kind:       kind,
		MimeType:   mimeType,
		ByteSize:   size,
		FileName:   cleanName(name),
		Width:      width,
		Height:     height,
		StorageKey: key,
		ThumbKey:   thumbKey,
		DurationMs: meta.DurationMS,
		Waveform:   meta.Waveform,
	})
	if err != nil {
		// The row is the record; bytes with no row are unreachable litter.
		s.discard(ctx, key, thumbKey)
		return File{}, domain.Internal(fmt.Errorf("record upload: %w", err))
	}
	return fromRow(row), nil
}

// putImage reads a picture whole, because measuring it and making the small
// copy both need all of it, and 16 MB is a bounded thing to hold.
func (s *Service) putImage(ctx context.Context, id uuid.UUID, key string, body io.Reader) (int64, int32, int32, *string, error) {
	data, err := readLimited(body, MaxImageBytes)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return 0, 0, 0, nil, errFileTooLarge(KindImage)
		}
		return 0, 0, 0, nil, errIncomplete(err)
	}

	width, height, merr := measure(data)
	if errors.Is(merr, errTooManyPixels) {
		return 0, 0, 0, nil, domain.InvalidField("file", "image_too_large",
			"That picture has too many pixels to process.")
	}
	if err := s.blobs.Put(ctx, key, bytes.NewReader(data)); err != nil {
		return 0, 0, 0, nil, domain.Internal(fmt.Errorf("store upload: %w", err))
	}

	// A picture this server cannot decode is still a picture: it is stored
	// and sent as it is, and the app shows it if the phone can.
	var thumbKey *string
	if merr == nil {
		if thumb, terr := thumbnail(data, width, height); terr != nil {
			s.log.WarnContext(ctx, "could not make thumbnail", "media", id, "error", terr)
		} else {
			tk := storageKey(id, VariantThumb)
			if perr := s.blobs.Put(ctx, tk, bytes.NewReader(thumb)); perr != nil {
				s.log.WarnContext(ctx, "could not store thumbnail", "media", id, "error", perr)
			} else {
				thumbKey = &tk
			}
		}
	}
	return int64(len(data)), width, height, thumbKey, nil
}

// putStream sends the bytes to storage as they arrive, so a large video
// never sits in this process's memory.
func (s *Service) putStream(ctx context.Context, key string, body io.Reader, kind string) (int64, error) {
	guard := &guardedReader{r: body, remaining: limitFor(kind)}
	if err := s.blobs.Put(ctx, key, guard); err != nil {
		if errors.Is(err, errTooLarge) {
			return 0, errFileTooLarge(kind)
		}
		return 0, domain.Internal(fmt.Errorf("store upload: %w", err))
	}
	return guard.read, nil
}

// OpenForUser returns a file's bytes for a person: one they uploaded, or
// one on a message in a conversation they are in.
func (s *Service) OpenForUser(ctx context.Context, userID, id uuid.UUID, variant string) (File, io.ReadCloser, error) {
	return s.open(ctx, id, variant, func(ctx context.Context) (bool, error) {
		return s.store.UserCanReadMedia(ctx, gen.UserCanReadMediaParams{ID: id, UserID: userID})
	})
}

// OpenForAgent returns a file's bytes for a backend, by the same rule.
func (s *Service) OpenForAgent(ctx context.Context, agentID, id uuid.UUID, variant string) (File, io.ReadCloser, error) {
	return s.open(ctx, id, variant, func(ctx context.Context) (bool, error) {
		return s.store.AgentCanReadMedia(ctx, gen.AgentCanReadMediaParams{ID: id, AgentID: agentID})
	})
}

// open checks who is asking before it opens anything.
//
// A file nobody may see and a file that does not exist answer the same way,
// so the ids of other people's files cannot be confirmed by asking for them.
func (s *Service) open(ctx context.Context, id uuid.UUID, variant string,
	allowed func(context.Context) (bool, error)) (File, io.ReadCloser, error) {
	row, err := s.store.GetMedia(ctx, id)
	if err != nil {
		if store.IsNoRows(err) {
			return File{}, nil, errNoFile()
		}
		return File{}, nil, domain.Internal(fmt.Errorf("get media %s: %w", id, err))
	}
	ok, err := allowed(ctx)
	if err != nil {
		return File{}, nil, domain.Internal(fmt.Errorf("authorize media %s: %w", id, err))
	}
	if !ok {
		return File{}, nil, errNoFile()
	}

	file := fromRow(row)
	key := row.StorageKey
	if variant == VariantThumb {
		if row.ThumbKey == nil {
			return File{}, nil, domain.NotFound("no_thumbnail", "That file has no small version.")
		}
		key = *row.ThumbKey
	}
	rc, err := s.blobs.Open(ctx, key)
	if err != nil {
		if errors.Is(err, blobs.ErrNotFound) {
			// The row says it exists and the store disagrees. That is this
			// server's problem, not the caller's request being wrong.
			return File{}, nil, domain.Internal(fmt.Errorf("media %s: bytes missing at %s", id, key))
		}
		return File{}, nil, domain.Internal(fmt.Errorf("open media %s: %w", id, err))
	}
	return file, rc, nil
}

// SweepUnclaimed removes uploads no message ever claimed. A person who
// picks a photo and changes their mind leaves bytes behind; without this
// they stay forever.
func (s *Service) SweepUnclaimed(ctx context.Context, olderThan time.Duration) (int, error) {
	rows, err := s.store.DeleteUnclaimedMedia(ctx, time.Now().Add(-olderThan))
	if err != nil {
		return 0, domain.Internal(fmt.Errorf("delete unclaimed media: %w", err))
	}
	for _, row := range rows {
		if err := s.blobs.Delete(ctx, row.StorageKey); err != nil {
			s.log.WarnContext(ctx, "could not delete unclaimed file", "key", row.StorageKey, "error", err)
		}
		if row.ThumbKey != nil {
			if err := s.blobs.Delete(ctx, *row.ThumbKey); err != nil {
				s.log.WarnContext(ctx, "could not delete thumbnail", "key", *row.ThumbKey, "error", err)
			}
		}
	}
	return len(rows), nil
}

// RunSweeper removes abandoned uploads on a schedule, beside the server.
func (s *Service) RunSweeper(ctx context.Context, every, olderThan time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.SweepUnclaimed(ctx, olderThan)
			if err != nil {
				s.log.WarnContext(ctx, "could not sweep unclaimed uploads", "error", err)
				continue
			}
			if n > 0 {
				s.log.InfoContext(ctx, "removed unclaimed uploads", "count", n)
			}
		}
	}
}

func (s *Service) allow(ctx context.Context, owner Owner) error {
	policy := userUploadPolicy
	if owner.Kind == OwnerAgent {
		policy = agentUploadPolicy
	}
	wait, err := s.limiter.Allow(ctx, "upload:"+owner.Kind+":"+owner.ID.String(), policy)
	if err != nil {
		return domain.Internal(fmt.Errorf("rate limit upload: %w", err))
	}
	if wait > 0 {
		return domain.RateLimited("rate_limited", "You are uploading too quickly.", wait)
	}
	return nil
}

// discard removes bytes an upload wrote before it failed.
func (s *Service) discard(ctx context.Context, key string, thumbKey *string) {
	if err := s.blobs.Delete(ctx, key); err != nil {
		s.log.WarnContext(ctx, "could not remove failed upload", "key", key, "error", err)
	}
	if thumbKey != nil {
		if err := s.blobs.Delete(ctx, *thumbKey); err != nil {
			s.log.WarnContext(ctx, "could not remove failed thumbnail", "key", *thumbKey, "error", err)
		}
	}
}

func errEmpty() error {
	return domain.InvalidField("file", "empty_file", "That file is empty.")
}

func errNoFile() error {
	return domain.NotFound("media_not_found", "No such file.")
}

func errIncomplete(err error) error {
	return domain.Invalid("upload_incomplete", "The upload did not finish.").Wrap(err)
}

func errFileTooLarge(kind string) error {
	return domain.InvalidField("file", "file_too_large",
		fmt.Sprintf("That %s is larger than %d MB.", noun(kind), limitFor(kind)>>20))
}

func noun(kind string) string {
	switch kind {
	case KindImage:
		return "picture"
	case KindVideo:
		return "video"
	case KindAudio:
		return "recording"
	default:
		return "file"
	}
}
