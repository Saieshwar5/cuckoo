package agents_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// recorder keeps every event published, so a test can say what was
// announced and to whom.
type recorder struct {
	events []realtime.Event
}

func (r *recorder) Publish(_ context.Context, ev realtime.Event) error {
	r.events = append(r.events, ev)
	return nil
}

func (r *recorder) statuses(t *testing.T) []string {
	t.Helper()
	out := make([]string, 0, len(r.events))
	for _, ev := range r.events {
		if ev.Type != agents.EventAgentStatus {
			t.Fatalf("unexpected event %q", ev.Type)
		}
		var p agents.AgentStatusEvent
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			t.Fatalf("decode: %v", err)
		}
		out = append(out, p.Status)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Every real change in an agent's connection is announced to the people who
// share a conversation with it, and nothing else is: a backend confirming
// for the thousandth time that it is alive is not news.
func TestStatusChangesAreAnnouncedOnce(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewStore(t)
	rec := &recorder{}
	svc := agents.New(db, agents.WithPublisher(rec))
	owner := testutil.CreateUser(t, db)
	agent, err := svc.Create(ctx, owner.ID, agents.CreateInput{Handle: "live", DisplayName: "Live"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	binding, _, err := svc.SetBinding(ctx, owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket})
	if err != nil {
		t.Fatalf("SetBinding: %v", err)
	}
	for _, step := range []struct {
		name string
		do   func() error
	}{
		{"connect", func() error { return svc.SocketConnected(ctx, binding.ID) }},
		{"connect again", func() error { return svc.SocketConnected(ctx, binding.ID) }},
		{"deliver", func() error { return svc.RecordDeliverySuccess(ctx, binding.ID) }},
		{"fail once", func() error { return svc.RecordDeliveryFailure(ctx, binding.ID) }},
		{"close", func() error { return svc.SocketClosed(ctx, binding.ID) }},
		{"close again", func() error { return svc.SocketClosed(ctx, binding.ID) }},
		{"revoke", func() error { return svc.RevokeBinding(ctx, owner.ID, agent.ID) }},
	} {
		if err := step.do(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}

	want := []string{"idle", "connected", "idle", "none"}
	if got := rec.statuses(t); !equal(got, want) {
		t.Errorf("announced %v, want %v", got, want)
	}
	for _, ev := range rec.events {
		if len(ev.Recipients) != 1 || ev.Recipients[0] != owner.ID {
			t.Errorf("recipients = %v, want just the owner %s", ev.Recipients, owner.ID)
		}
	}
}

// Deleting an agent ends its binding and says so.
func TestDeleteAnnouncesNone(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewStore(t)
	rec := &recorder{}
	svc := agents.New(db, agents.WithPublisher(rec))
	owner := testutil.CreateUser(t, db)
	agent, _ := svc.Create(ctx, owner.ID, agents.CreateInput{Handle: "gone", DisplayName: "Gone"})
	if _, _, err := svc.SetBinding(ctx, owner.ID, agent.ID, agents.SetBindingInput{Mode: agents.ModeSocket}); err != nil {
		t.Fatalf("SetBinding: %v", err)
	}
	if err := svc.Delete(ctx, owner.ID, agent.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := rec.statuses(t); !equal(got, []string{"idle", "none"}) {
		t.Errorf("announced %v, want [idle none]", got)
	}
	if _, err := svc.Get(ctx, agent.ID); err == nil {
		t.Error("deleted agent still found")
	}
	_ = uuid.Nil
}
