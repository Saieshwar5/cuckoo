package testutil

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
)

// defaultTestRedisURL matches `make up`, on a database number the running
// server never uses, so a test run cannot touch development counters.
const defaultTestRedisURL = "redis://localhost:6380/1"

var (
	redisOnce   sync.Once
	redisClient *redis.Client
	redisErr    error
)

// NewLimiter returns a rate limiter over the test Redis whose keys carry a
// prefix unique to this test. Buckets expire on their own, so there is
// nothing to clean up and tests may run in parallel.
func NewLimiter(t *testing.T) *ratelimit.Redis {
	t.Helper()

	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		t.Fatalf("testutil: random prefix: %v", err)
	}
	return ratelimit.NewRedis(sharedRedis(t), "test:"+hex.EncodeToString(raw[:])+":")
}

// sharedRedis opens the test Redis once per test binary.
func sharedRedis(t *testing.T) *redis.Client {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping Redis test in -short mode")
	}

	redisOnce.Do(func() {
		url := os.Getenv("CUCKOO_TEST_REDIS_URL")
		if url == "" {
			url = defaultTestRedisURL
		}
		opts, err := redis.ParseURL(url)
		if err != nil {
			redisErr = err
			return
		}
		redisClient = redis.NewClient(opts)
		redisErr = redisClient.Ping(context.Background()).Err()
	})

	if redisErr != nil {
		// Same rule as the database: in CI, unreachable is a broken build,
		// not a quiet pass.
		if os.Getenv("CI") != "" {
			t.Fatalf("testutil: test Redis unavailable in CI: %v", redisErr)
		}
		t.Skipf("testutil: test Redis unavailable (run `make up`): %v", redisErr)
	}
	return redisClient
}
