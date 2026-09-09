package blobs

// A helper the S3 test needs to run against a bucket it holds in memory. It
// lives here rather than in s3.go so nothing outside a test can build a store
// that never looked for credentials: this file is only compiled for tests.

// NewS3ForTesting builds an S3 store over a client the test supplies.
func NewS3ForTesting(api s3API, bucket string) *S3 { return newS3(api, bucket) }
