package blobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3 keeps blobs in a bucket.
//
// The object key is the blob key unchanged. That is what makes moving from
// Disk to S3 a copy rather than a migration: `aws s3 sync /data/media
// s3://bucket/`, change one setting, restart. Nothing in the database names
// a backend, only a key.
//
// Bytes still travel through the hub, as the package comment says; a bucket
// changes where they land, not who serves them.
//
// AWS marks the upload manager deprecated in favour of feature/s3/transfermanager,
// which is still a v0 module and free to break its API on any release. A
// deprecated v1 that will keep working is the safer of the two for something
// that has to run unattended; revisit when the successor reaches v1. Hence
// the staticcheck exemptions below, and nowhere else.
type S3 struct {
	api      s3API
	bucket   string
	uploader *manager.Uploader //nolint:staticcheck // SA1019, see above
}

// s3API is the part of the S3 client this package uses. Put needs the five
// methods of the upload manager because one call may become a multipart
// upload; the other two are the whole of read and delete.
type s3API interface {
	manager.UploadAPIClient
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// NewS3 prepares a bucket to hold blobs.
//
// Credentials are not arguments and are not in .env: the SDK finds them the
// way every AWS tool does, which on the hub's machine is the instance's own
// role. An empty region leaves that to the SDK too — AWS_REGION, then the
// instance's own region — and it is an error if neither answers, because a
// bucket in the wrong region fails one upload at a time rather than at start.
func NewS3(ctx context.Context, bucket, region string) (*S3, error) {
	if bucket == "" {
		return nil, errors.New("blobs: s3 bucket is empty")
	}
	var opts []func(*awsconfig.LoadOptions) error
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("blobs: load aws configuration: %w", err)
	}
	if cfg.Region == "" {
		return nil, errors.New("blobs: no aws region resolved; set CUCKOO_S3_REGION")
	}
	return newS3(s3.NewFromConfig(cfg), bucket), nil
}

// newS3 is NewS3 without the credential lookup, so a test can hand in its own
// client.
func newS3(api s3API, bucket string) *S3 {
	return &S3{
		api:    api,
		bucket: bucket,
		// The hub streams every upload through itself on a small machine, so
		// what is being bought here is bounded memory, not speed: at most two
		// parts of the smallest size S3 allows are held at once, whatever the
		// file. The largest upload the hub accepts is 64 MB.
		uploader: manager.NewUploader(api, func(u *manager.Uploader) { //nolint:staticcheck // SA1019, see the type
			u.PartSize = manager.MinUploadPartSize
			u.Concurrency = 2
		}),
	}
}

// Bucket is where blobs are written.
func (s *S3) Bucket() string { return s.bucket }

func (s *S3) Put(ctx context.Context, key string, r io.Reader) error {
	if err := checkKey(key); err != nil {
		return err
	}
	// The uploader reads r on its own goroutines and reports a read failure
	// in its own words. Callers match on the reader's error — media's size
	// guard is one, and an upload over the limit must still be "too large"
	// rather than a 500 — so the reader's error is kept and preferred.
	tracked := &trackedReader{r: r}
	if _, err := s.uploader.Upload(ctx, &s3.PutObjectInput{ //nolint:staticcheck // SA1019, see the type
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   tracked,
	}); err != nil {
		if failure := tracked.failure(); failure != nil {
			return fmt.Errorf("blobs: write %s: %w", key, failure)
		}
		return fmt.Errorf("blobs: write %s: %w", key, err)
	}
	return nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	out, err := s.api.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		// S3 only says NoSuchKey to a caller allowed to list the bucket;
		// without s3:ListBucket a missing object is an access denial
		// instead, and a deleted file would read as a server fault. The
		// hub's policy grants ListBucket for this reason.
		var missing *types.NoSuchKey
		if errors.As(err, &missing) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("blobs: open %s: %w", key, err)
	}
	return out.Body, nil
}

// Delete removes the object. S3 reports no error for a key holding nothing,
// which is the contract Store already asks for.
func (s *S3) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if _, err := s.api.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("blobs: delete %s: %w", key, err)
	}
	return nil
}

// trackedReader remembers the last error its reader gave, so a failure that
// began in the caller's reader can be reported as the caller's error rather
// than as whatever the upload made of it.
type trackedReader struct {
	r   io.Reader
	mu  sync.Mutex
	err error
}

func (t *trackedReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		t.mu.Lock()
		t.err = err
		t.mu.Unlock()
	}
	return n, err
}

func (t *trackedReader) failure() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}
