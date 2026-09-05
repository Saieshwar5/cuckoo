package delivery

import "time"

// retryDelays is the wait after each failed attempt, in order. It starts
// fast, because most failures are a deploy or a blip, and slows to hourly,
// because a backend that has been down for an hour is not coming back in the
// next minute.
var retryDelays = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	30 * time.Second,
	2 * time.Minute,
	10 * time.Minute,
	1 * time.Hour,
}

// retryFor is how long after creation an event is still worth attempting.
// Past it, the backend is expected to catch up from its event history.
const retryFor = 24 * time.Hour

// nextAttempt returns when to try again after the given (1-based) failed
// attempt, or false when the day allowed for retries has run out.
func nextAttempt(attempt int, createdAt, now time.Time) (time.Time, bool) {
	if attempt < 1 {
		attempt = 1
	}
	delay := retryDelays[len(retryDelays)-1]
	if attempt <= len(retryDelays) {
		delay = retryDelays[attempt-1]
	}

	next := now.Add(delay)
	if next.After(createdAt.Add(retryFor)) {
		return time.Time{}, false
	}
	return next, true
}
