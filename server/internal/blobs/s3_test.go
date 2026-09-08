package blobs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/Saieshwar5/cuckoo/server/internal/blobs"
)

// fakeS3 is a bucket as a map. It answers the five methods the upload manager
// needs and the two the package calls itself; a body small enough to fit in
// one part only ever reaches PutObject, and every upload the hub accepts is.
type fakeS3 struct {
	objects map[string][]byte
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string][]byte{}} }

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.objects[aws.ToString(in.Key)] = body
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	body, ok := f.objects[aws.ToString(in.Key)]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(body))}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(f.objects, aws.ToString(in.Key))
	return &s3.DeleteObjectOutput{}, nil
}

// The multipart methods complete the interface. Reaching one means a body
// larger than a part, which the hub's own limits already refuse.
var errMultipart = errors.New("fakeS3: multipart upload is not used")

func (f *fakeS3) UploadPart(context.Context, *s3.UploadPartInput, ...func(*s3.Options)) (*s3.UploadPartOutput, error) {
	return nil, errMultipart
}

func (f *fakeS3) CreateMultipartUpload(context.Context, *s3.CreateMultipartUploadInput, ...func(*s3.Options)) (*s3.CreateMultipartUploadOutput, error) {
	return nil, errMultipart
}

func (f *fakeS3) CompleteMultipartUpload(context.Context, *s3.CompleteMultipartUploadInput, ...func(*s3.Options)) (*s3.CompleteMultipartUploadOutput, error) {
	return nil, errMultipart
}

func (f *fakeS3) AbortMultipartUpload(context.Context, *s3.AbortMultipartUploadInput, ...func(*s3.Options)) (*s3.AbortMultipartUploadOutput, error) {
	return nil, errMultipart
}

func TestS3RoundTrip(t *testing.T) {
	ctx := context.Background()
	store := blobs.NewS3ForTesting(newFakeS3(), "cuckoo-media")

	if err := store.Put(ctx, "ab/cd/abcd", bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if got := read(t, store, "ab/cd/abcd"); got != "hello" {
		t.Errorf("read back %q", got)
	}

	// Writing again replaces, so a retry of an upload is not two objects.
	if err := store.Put(ctx, "ab/cd/abcd", bytes.NewReader([]byte("again"))); err != nil {
		t.Fatalf("Put again: %v", err)
	}
	if got := read(t, store, "ab/cd/abcd"); got != "again" {
		t.Errorf("after replace %q", got)
	}
}

func TestS3OpenMissingIsNotFound(t *testing.T) {
	store := blobs.NewS3ForTesting(newFakeS3(), "cuckoo-media")

	_, err := store.Open(context.Background(), "ab/cd/nothing")
	if !errors.Is(err, blobs.ErrNotFound) {
		t.Fatalf("Open of a missing key = %v, want ErrNotFound", err)
	}
}

func TestS3DeleteMissingIsNotAnError(t *testing.T) {
	store := blobs.NewS3ForTesting(newFakeS3(), "cuckoo-media")

	if err := store.Delete(context.Background(), "ab/cd/nothing"); err != nil {
		t.Fatalf("Delete of a missing key: %v", err)
	}
}

func TestS3RejectsAKeyThatEscapes(t *testing.T) {
	ctx := context.Background()
	store := blobs.NewS3ForTesting(newFakeS3(), "cuckoo-media")

	for _, key := range []string{"", "/etc/passwd", "../../etc/passwd", "ab//cd"} {
		if err := store.Put(ctx, key, bytes.NewReader([]byte("x"))); err == nil {
			t.Errorf("Put(%q) was allowed", key)
		}
		if _, err := store.Open(ctx, key); err == nil {
			t.Errorf("Open(%q) was allowed", key)
		}
		if err := store.Delete(ctx, key); err == nil {
			t.Errorf("Delete(%q) was allowed", key)
		}
	}
}

// errRefused stands in for media's size guard: an error the caller matches on
// after Put returns, which must survive the upload machinery in between.
var errRefused = errors.New("refused")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errRefused }

func TestS3PutKeepsTheReadersError(t *testing.T) {
	store := blobs.NewS3ForTesting(newFakeS3(), "cuckoo-media")

	err := store.Put(context.Background(), "ab/cd/abcd", failingReader{})
	if !errors.Is(err, errRefused) {
		t.Fatalf("Put with a failing reader = %v, want it to wrap errRefused", err)
	}
}
