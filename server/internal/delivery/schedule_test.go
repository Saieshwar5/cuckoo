package delivery

import (
	"testing"
	"time"
)

func TestNextAttemptLadder(t *testing.T) {
	created := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
	now := created.Add(time.Minute)

	want := []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, time.Hour, time.Hour}
	for i, delay := range want {
		attempt := i + 1
		next, ok := nextAttempt(attempt, created, now)
		if !ok {
			t.Fatalf("attempt %d: exhausted within the first minute", attempt)
		}
		if got := next.Sub(now); got != delay {
			t.Errorf("attempt %d: delay = %v, want %v", attempt, got, delay)
		}
	}
}

// A retry is only scheduled if it would still fall inside the day allowed;
// the attempt that would land after it is the last.
func TestNextAttemptExhaustsAfterADay(t *testing.T) {
	created := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)

	if _, ok := nextAttempt(6, created, created.Add(23*time.Hour+30*time.Minute)); ok {
		t.Error("an hourly retry at 23h30 lands past the deadline but was scheduled")
	}
	if _, ok := nextAttempt(6, created, created.Add(22*time.Hour)); !ok {
		t.Error("an hourly retry at 22h fits inside the day but was refused")
	}
	if _, ok := nextAttempt(0, created, created); !ok {
		t.Error("attempt 0 should be treated as the first")
	}
}
