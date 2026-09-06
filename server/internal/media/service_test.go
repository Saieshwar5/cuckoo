package media_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// pngBytes makes a picture of a given size: a real one, so the decoder,
// the measurer and the thumbnailer all see what they would in life.
func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func newService(t *testing.T) (*media.Service, *store.Store, media.Owner) {
	t.Helper()
	db := testutil.NewStore(t)
	bs, err := blobs.NewDisk(t.TempDir())
	if err != nil {
		t.Fatalf("blobs: %v", err)
	}
	owner := testutil.CreateUser(t, db)
	return media.New(db, bs), db, media.Owner{Kind: media.OwnerUser, ID: owner.ID}
}

func code(t *testing.T, err error) string {
	t.Helper()
	derr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("not a classified error: %v", err)
	}
	return derr.Code
}

func readAll(t *testing.T, rc io.ReadCloser) []byte {
	t.Helper()
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return data
}

// What a file is comes from its bytes. A caller that renames a document to
// .jpg has not made it a picture, and one that calls a picture .txt has not
// stopped it being one.
func TestUploadClassifiesByContentNotName(t *testing.T) {
	ctx := context.Background()
	svc, _, owner := newService(t)

	picture, err := svc.Upload(ctx, owner, "notes.txt", bytes.NewReader(pngBytes(t, 40, 30)))
	if err != nil {
		t.Fatalf("upload picture: %v", err)
	}
	if picture.Kind != media.KindImage || picture.MimeType != "image/png" {
		t.Errorf("picture = %s/%s, want image and image/png", picture.Kind, picture.MimeType)
	}
	if picture.FileName != "notes.txt" {
		t.Errorf("file name = %q, want the name kept as given", picture.FileName)
	}

	document, err := svc.Upload(ctx, owner, "photo.jpg", strings.NewReader("dear sir, please find attached"))
	if err != nil {
		t.Fatalf("upload document: %v", err)
	}
	if document.Kind != media.KindFile {
		t.Errorf("document kind = %s, want file", document.Kind)
	}
}

// A picture arrives measured and with a small copy, so a bubble is the
// right shape before any bytes have loaded.
func TestUploadMeasuresAndShrinksPictures(t *testing.T) {
	ctx := context.Background()
	svc, _, owner := newService(t)

	original := pngBytes(t, 1200, 800)
	file, err := svc.Upload(ctx, owner, "holiday.png", bytes.NewReader(original))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if file.Width != 1200 || file.Height != 800 {
		t.Errorf("size = %dx%d, want 1200x800", file.Width, file.Height)
	}
	if !file.HasThumbnail {
		t.Fatal("no thumbnail was made")
	}
	if file.ByteSize != int64(len(original)) {
		t.Errorf("byte size = %d, want %d", file.ByteSize, len(original))
	}

	_, body, err := svc.OpenForUser(ctx, owner.ID, file.ID, media.VariantOriginal)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	if got := readAll(t, body); !bytes.Equal(got, original) {
		t.Error("the original came back changed")
	}

	_, thumbBody, err := svc.OpenForUser(ctx, owner.ID, file.ID, media.VariantThumb)
	if err != nil {
		t.Fatalf("open thumbnail: %v", err)
	}
	thumb := readAll(t, thumbBody)
	if len(thumb) >= len(original) {
		t.Errorf("thumbnail is %d bytes, no smaller than the original's %d", len(thumb), len(original))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	if format != "jpeg" {
		t.Errorf("thumbnail format = %s, want jpeg", format)
	}
	// Scaled to fit, and still the shape of the original.
	if cfg.Width != 480 || cfg.Height != 320 {
		t.Errorf("thumbnail = %dx%d, want 480x320", cfg.Width, cfg.Height)
	}
}

// A picture too small to shrink is stored as it is rather than enlarged.
func TestSmallPicturesKeepTheirSize(t *testing.T) {
	ctx := context.Background()
	svc, _, owner := newService(t)

	file, err := svc.Upload(ctx, owner, "tiny.png", bytes.NewReader(pngBytes(t, 64, 64)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	_, body, err := svc.OpenForUser(ctx, owner.ID, file.ID, media.VariantThumb)
	if err != nil {
		t.Fatalf("open thumbnail: %v", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(readAll(t, body)))
	if err != nil {
		t.Fatalf("decode thumbnail: %v", err)
	}
	if cfg.Width != 64 || cfg.Height != 64 {
		t.Errorf("thumbnail = %dx%d, want the original 64x64", cfg.Width, cfg.Height)
	}
}

func TestUploadRefusesEmptyAndOversized(t *testing.T) {
	ctx := context.Background()
	svc, _, owner := newService(t)

	if _, err := svc.Upload(ctx, owner, "nothing.txt", strings.NewReader("")); code(t, err) != "empty_file" {
		t.Errorf("empty upload = %v, want empty_file", err)
	}

	// A picture header followed by more bytes than a picture may be. The
	// reader is generated, so the test does not hold the whole thing.
	header := pngBytes(t, 8, 8)
	oversized := io.MultiReader(bytes.NewReader(header), io.LimitReader(zeros{}, media.MaxImageBytes))
	if _, err := svc.Upload(ctx, owner, "huge.png", oversized); code(t, err) != "file_too_large" {
		t.Errorf("oversized upload = %v, want file_too_large", err)
	}
}

// Bytes are readable by whoever uploaded them, and by nobody else until a
// message carries them into a conversation.
func TestUploadsArePrivateUntilTheyAreSent(t *testing.T) {
	ctx := context.Background()
	svc, db, owner := newService(t)
	stranger := testutil.CreateUser(t, db)

	file, err := svc.Upload(ctx, owner, "receipt.pdf", strings.NewReader("%PDF-1.7 a receipt"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if _, _, err := svc.OpenForUser(ctx, stranger.ID, file.ID, media.VariantOriginal); code(t, err) != "media_not_found" {
		t.Errorf("a stranger read it: %v", err)
	}
	// Not knowing about a file and not being allowed it answer alike, so
	// ids cannot be probed for.
	if _, _, err := svc.OpenForUser(ctx, owner.ID, domain.NewID(), media.VariantOriginal); code(t, err) != "media_not_found" {
		t.Errorf("unknown id = %v, want media_not_found", err)
	}
}

func TestThereIsNoThumbnailOfADocument(t *testing.T) {
	ctx := context.Background()
	svc, _, owner := newService(t)

	file, err := svc.Upload(ctx, owner, "report.pdf", strings.NewReader("%PDF-1.7 the report"))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if file.HasThumbnail {
		t.Error("a document was given a thumbnail")
	}
	if _, _, err := svc.OpenForUser(ctx, owner.ID, file.ID, media.VariantThumb); code(t, err) != "no_thumbnail" {
		t.Errorf("thumbnail of a document = %v, want no_thumbnail", err)
	}
}

// Someone picks a photo, changes their mind, and closes the app. The bytes
// do not stay on the disk forever.
func TestSweepRemovesUploadsNobodySent(t *testing.T) {
	ctx := context.Background()
	svc, _, owner := newService(t)

	file, err := svc.Upload(ctx, owner, "abandoned.png", bytes.NewReader(pngBytes(t, 20, 20)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	n, err := svc.SweepUnclaimed(ctx, 0)
	if err != nil {
		t.Fatalf("SweepUnclaimed: %v", err)
	}
	if n != 1 {
		t.Errorf("swept %d, want 1", n)
	}
	if _, _, err := svc.OpenForUser(ctx, owner.ID, file.ID, media.VariantOriginal); code(t, err) != "media_not_found" {
		t.Errorf("after the sweep, open = %v, want media_not_found", err)
	}
}

// zeros is an endless reader, for testing a limit without holding what it
// refuses.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
