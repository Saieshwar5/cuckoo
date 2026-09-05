package conversations_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// recorder is a Publisher that keeps what it was given.
type recorder struct {
	mu     sync.Mutex
	events []realtime.Event
}

func (r *recorder) Publish(_ context.Context, ev realtime.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *recorder) last(t *testing.T, want string) realtime.Event {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.events) == 0 {
		t.Fatalf("nothing published, want %s", want)
	}
	ev := r.events[len(r.events)-1]
	if ev.Type != want {
		t.Fatalf("last event is %s, want %s", ev.Type, want)
	}
	return ev
}

// Sending announces the message to the people in the conversation, and only
// them, after it is stored.
func TestSendAnnouncesToParticipants(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))

	res, err := svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hello"})
	if err != nil {
		t.Fatalf("SendAsUser: %v", err)
	}
	ev := rec.last(t, conversations.EventMessageCreated)
	if len(ev.UserIDs) != 1 || ev.UserIDs[0] != f.owner.ID {
		t.Errorf("recipients = %v, want just the owner", ev.UserIDs)
	}
	var p conversations.MessageCreatedEvent
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.ConversationID != f.dm.ID || p.Message.ID != res.ID || p.Message.Body.Text != "hello" ||
		p.Message.DeliveryStatus != conversations.DeliveryFailed {
		t.Errorf("payload = %+v, want the stored message with its status", p)
	}

	// The agent's reply reaches the person too.
	reply, _ := svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "hi"})
	ev = rec.last(t, conversations.EventMessageCreated)
	if len(ev.UserIDs) != 1 || ev.UserIDs[0] != f.owner.ID {
		t.Errorf("reply recipients = %v, want the owner", ev.UserIDs)
	}
	_ = json.Unmarshal(ev.Payload, &p)
	if p.Message.ID != reply.ID || p.Message.Sender.Kind != conversations.ParticipantAgent {
		t.Errorf("reply payload = %+v", p)
	}

	// A replayed send announces nothing: the device already heard it.
	before := rec.count()
	svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "once", IdempotencyKey: "k"}) //nolint:errcheck
	svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "once", IdempotencyKey: "k"}) //nolint:errcheck
	if got := rec.count() - before; got != 1 {
		t.Errorf("two sends under one key published %d events, want 1", got)
	}
}

func TestDeliveryChangedAnnouncesTheTick(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))
	testutil.BindWebhook(t, f.db, f.agent, "https://example.com/cuckoo")
	res, _ := svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hello"})

	if err := svc.DeliveryChanged(ctx, res.ID); err != nil {
		t.Fatalf("DeliveryChanged: %v", err)
	}
	ev := rec.last(t, conversations.EventDeliveryUpdated)
	var p conversations.DeliveryUpdatedEvent
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if p.MessageID != res.ID || p.ConversationID != f.dm.ID || p.DeliveryStatus != conversations.DeliveryPending {
		t.Errorf("payload = %+v, want the message's current status", p)
	}
	if err := svc.DeliveryChanged(ctx, domain.NewID()); err != nil {
		t.Errorf("DeliveryChanged for a missing message: %v, want nothing", err)
	}
}

// Paging forward from a message the caller has: oldest first, with a cursor
// that continues, so a device fills its gap in order.
func TestListMessagesAfter(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	var oldest = domain.NewID()
	for i, text := range []string{"one", "two", "three", "four", "five"} {
		res, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: text})
		if err != nil {
			t.Fatalf("send %q: %v", text, err)
		}
		if i == 0 {
			oldest = res.ID
		}
	}

	page, err := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{After: &oldest, Limit: 2})
	if err != nil {
		t.Fatalf("ListMessages after: %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Body.Text != "two" || page.Messages[1].Body.Text != "three" {
		t.Fatalf("first forward page = %+v, want two then three", page.Messages)
	}
	if page.NextAfter == nil || page.NextBefore != nil {
		t.Fatalf("cursors = after %v before %v, want only next_after", page.NextAfter, page.NextBefore)
	}
	page, _ = f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{After: page.NextAfter, Limit: 2})
	if len(page.Messages) != 2 || page.Messages[1].Body.Text != "five" || page.NextAfter != nil {
		t.Errorf("last forward page = %+v (next %v), want four, five and no cursor", page.Messages, page.NextAfter)
	}

	_, err = f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{After: &oldest, Before: &oldest})
	if domain.CodeOf(err) != "invalid_cursor" {
		t.Errorf("both cursors got %v, want invalid_cursor", err)
	}
}
