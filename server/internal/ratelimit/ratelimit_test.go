package ratelimit_test

import (
	"context"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func TestTokenBucket(t *testing.T) {
	ctx := context.Background()
	limiter := testutil.NewLimiter(t)
	policy := ratelimit.Policy{Rate: 10, Burst: 3}

	for i := range 3 {
		if wait, err := limiter.Allow(ctx, "k", policy); err != nil || wait != 0 {
			t.Fatalf("burst request %d refused: wait %v, err %v", i+1, wait, err)
		}
	}

	wait, err := limiter.Allow(ctx, "k", policy)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if wait <= 0 || wait > 100*time.Millisecond {
		t.Errorf("fourth request: wait = %v, want about one token's worth (100ms)", wait)
	}

	// Another key is another bucket.
	if wait, _ := limiter.Allow(ctx, "other", policy); wait != 0 {
		t.Errorf("a different key shared the exhausted bucket")
	}

	// Waiting refills.
	time.Sleep(120 * time.Millisecond)
	if wait, _ := limiter.Allow(ctx, "k", policy); wait != 0 {
		t.Errorf("after waiting for a refill, still refused for %v", wait)
	}
}

// Two limiters with different prefixes never see each other's buckets. This
// is what lets every test share one Redis.
func TestPrefixesIsolate(t *testing.T) {
	ctx := context.Background()
	a, b := testutil.NewLimiter(t), testutil.NewLimiter(t)
	policy := ratelimit.Policy{Rate: 1, Burst: 1}

	if wait, _ := a.Allow(ctx, "k", policy); wait != 0 {
		t.Fatal("first request refused")
	}
	if wait, _ := b.Allow(ctx, "k", policy); wait != 0 {
		t.Error("the other prefix inherited an exhausted bucket")
	}
}

func TestUnlimited(t *testing.T) {
	var l ratelimit.Unlimited
	for range 1000 {
		if wait, err := l.Allow(context.Background(), "k", ratelimit.Policy{Rate: 1, Burst: 1}); wait != 0 || err != nil {
			t.Fatal("Unlimited limited something")
		}
	}
}
