package conversations_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// latest is the newest event of one type, however many others came after.
func (r *recorder) latest(t *testing.T, want string) realtime.Event {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.events) - 1; i >= 0; i-- {
		if r.events[i].Type == want {
			return r.events[i]
		}
	}
	t.Fatalf("no %s published", want)
	return realtime.Event{}
}

func activityOf(t *testing.T, ev realtime.Event) conversations.ActivityEvent {
	t.Helper()
	var p conversations.ActivityEvent
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestActivity(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))

	if err := svc.Activity(ctx, f.agent.ID, f.dm.ID, "working", "  Checking the weather "); err != nil {
		t.Fatalf("Activity working: %v", err)
	}
	ev := rec.last(t, conversations.EventActivity)
	p := activityOf(t, ev)
	if len(ev.Recipients) != 1 || ev.Recipients[0] != f.owner.ID || p.AgentID != f.agent.ID {
		t.Errorf("activity event = %+v to %v", p, ev.Recipients)
	}
	if p.State != "working" || p.Label != "Checking the weather" {
		t.Errorf("activity = %q %q, want working with the label trimmed", p.State, p.Label)
	}
	if until := time.Until(p.ExpiresAt); until < 8*time.Second || until > 11*time.Second {
		t.Errorf("working expires in %v, want about ten seconds", until)
	}

	if err := svc.Activity(ctx, f.agent.ID, f.dm.ID, "idle", ""); err != nil {
		t.Fatal(err)
	}
	if p := activityOf(t, rec.last(t, conversations.EventActivity)); p.State != "idle" || p.ExpiresAt.After(time.Now()) {
		t.Errorf("idle = %+v, want expired at once", p)
	}

	for _, bad := range []struct{ state, label, code string }{
		{"typing", "", "invalid_state"},
		{"thinking", "Hmm", "invalid_label"},
		{"idle", "done", "invalid_label"},
		{"working", strings.Repeat("a", 41), "invalid_label"},
		{"working", "two\nlines", "invalid_label"},
		{"working", "Visit https://spam.example", "invalid_label"},
		{"working", "go to www.spam.example", "invalid_label"},
	} {
		if err := svc.Activity(ctx, f.agent.ID, f.dm.ID, bad.state, bad.label); domain.CodeOf(err) != bad.code {
			t.Errorf("Activity(%q, %q) got %v, want %s", bad.state, bad.label, err, bad.code)
		}
	}
	stranger := testutil.CreateAgent(t, f.db, f.other)
	if err := svc.Activity(ctx, stranger.ID, f.dm.ID, "thinking", ""); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger got %v", err)
	}
}

// The first version of the protocol keeps working: start is thinking and
// stop is idle.
func TestTypingIsActivity(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))

	if err := svc.Typing(ctx, f.agent.ID, f.dm.ID, "start"); err != nil {
		t.Fatal(err)
	}
	if p := activityOf(t, rec.last(t, conversations.EventActivity)); p.State != "thinking" {
		t.Errorf("start = %q, want thinking", p.State)
	}
	if err := svc.Typing(ctx, f.agent.ID, f.dm.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	if p := activityOf(t, rec.last(t, conversations.EventActivity)); p.State != "idle" {
		t.Errorf("stop = %q, want idle", p.State)
	}
	if err := svc.Typing(ctx, f.agent.ID, f.dm.ID, "thinking"); domain.CodeOf(err) != "invalid_state" {
		t.Errorf("typing with an activity state got %v", err)
	}
}
