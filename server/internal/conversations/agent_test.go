package conversations_test

import (
	"context"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// The loop closes: an agent answers into the conversation it was told about,
// and the person finds the reply in their history.
func TestSendAsAgent(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "my UPI payment failed"}) //nolint:errcheck

	res, err := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "I can see the deduction."})
	if err != nil {
		t.Fatalf("SendAsAgent: %v", err)
	}
	if !res.Created {
		t.Error("a fresh send reports Created = false")
	}
	if res.Sender.Kind != conversations.ParticipantAgent || res.Sender.ID != f.agent.ID {
		t.Errorf("Sender = %+v, want the agent", res.Sender)
	}
	// Nobody but the person to deliver to, and their device is not tracked
	// here, so no delivery status.
	if res.DeliveryStatus != "" {
		t.Errorf("DeliveryStatus = %q, want none for an agent message in a DM", res.DeliveryStatus)
	}
	rows, _ := f.db.ListDeliveriesByMessage(ctx, res.ID)
	if len(rows) != 0 {
		t.Errorf("an agent's own message was queued for delivery to %d agents", len(rows))
	}

	page, _ := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if len(page.Messages) != 2 || page.Messages[0].ID != res.ID || page.Messages[1].Sender.Kind != conversations.ParticipantUser {
		t.Errorf("owner's history = %+v, want the reply newest", page.Messages)
	}
	list, _ := f.svc.ListMine(ctx, f.owner.ID)
	if list[0].LastMessage == nil || list[0].LastMessage.ID != res.ID {
		t.Errorf("chat list preview = %+v, want the agent's reply", list[0].LastMessage)
	}
}

func TestSendAsAgentRequiresMembership(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	stranger := testutil.CreateAgent(t, f.db, f.other)

	_, err := f.svc.SendAsAgent(ctx, stranger.ID, f.dm.ID, conversations.SendInput{Text: "let me in"})
	if domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger agent got %v, want not_participant", err)
	}
	_, err = f.svc.SendAsAgent(ctx, f.agent.ID, domain.NewID(), conversations.SendInput{Text: "hello"})
	if domain.CodeOf(err) != "conversation_not_found" {
		t.Errorf("missing conversation got %v, want conversation_not_found", err)
	}
}

// A retry with the same key gets the same message, for people and agents
// alike. A key reused for a different conversation is a confused client and
// is refused rather than answered with a message from elsewhere.
func TestIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	in := conversations.SendInput{Text: "once", IdempotencyKey: "app-42"}

	first, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, in)
	if err != nil || !first.Created {
		t.Fatalf("first send: %+v, %v", first, err)
	}
	again, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, in)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if again.Created || again.ID != first.ID {
		t.Errorf("retry = %+v, want the first message with Created = false", again)
	}
	page, _ := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if len(page.Messages) != 1 {
		t.Errorf("history has %d messages after a retry, want 1", len(page.Messages))
	}

	// Keys are per sender: the agent may use the same string.
	reply, err := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, in)
	if err != nil || !reply.Created || reply.ID == first.ID {
		t.Errorf("agent send with the user's key = %+v, %v; want its own message", reply, err)
	}

	elsewhere := testutil.OwnerDM(t, f.db, testutil.CreateAgent(t, f.db, f.owner))
	_, err = f.svc.SendAsUser(ctx, f.owner.ID, elsewhere.ID, in)
	if domain.CodeOf(err) != "idempotency_key_reused" {
		t.Errorf("key reused elsewhere got %v, want idempotency_key_reused", err)
	}

	_, err = f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID,
		conversations.SendInput{Text: "x", IdempotencyKey: string(make([]byte, 201))})
	if domain.CodeOf(err) != "invalid_idempotency_key" {
		t.Errorf("201-byte key got %v, want invalid_idempotency_key", err)
	}
}

// refusing is a Limiter that always says wait.
type refusing struct{ wait time.Duration }

func (r refusing) Allow(context.Context, string, ratelimit.Policy) (time.Duration, error) {
	return r.wait, nil
}

func TestSendIsRateLimited(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	limited := conversations.New(f.db, conversations.WithLimiter(refusing{wait: 3 * time.Second}))

	_, err := limited.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hi"})
	e, ok := domain.AsError(err)
	if !ok || e.Kind != domain.KindRateLimited || e.Code != "rate_limited" || e.RetryAfter != 3*time.Second {
		t.Errorf("got %v, want rate_limited with RetryAfter 3s", err)
	}

	// A retry of something already sent is not new traffic.
	sent, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hi", IdempotencyKey: "k"})
	if err != nil {
		t.Fatalf("SendAsUser: %v", err)
	}
	replay, err := limited.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hi", IdempotencyKey: "k"})
	if err != nil || replay.ID != sent.ID {
		t.Errorf("replay under limit = %+v, %v; want the existing message", replay, err)
	}
}

// An agent sees history only from when it joined. The owner DM is created
// with the agent, so the rule is provoked by backdating a message.
func TestListMessagesForAgentStartsAtJoin(t *testing.T) {
	ctx := context.Background()
	db, tx := testutil.NewStoreTx(t)
	svc := conversations.New(db)
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner)
	dm := testutil.OwnerDM(t, db, agent)

	before, _ := svc.SendAsUser(ctx, owner.ID, dm.ID, conversations.SendInput{Text: "before"})
	svc.SendAsUser(ctx, owner.ID, dm.ID, conversations.SendInput{Text: "after"}) //nolint:errcheck
	if _, err := tx.Exec(ctx, "UPDATE messages SET created_at = now() - interval '1 hour' WHERE id = $1", before.ID); err != nil {
		t.Fatalf("backdate: %v", err)
	}

	theirs, err := svc.ListMessagesForAgent(ctx, agent.ID, dm.ID, conversations.ListMessagesInput{})
	if err != nil {
		t.Fatalf("ListMessagesForAgent: %v", err)
	}
	if len(theirs.Messages) != 1 || theirs.Messages[0].Body.Text != "after" {
		t.Errorf("agent sees %+v, want only what was said since it joined", theirs.Messages)
	}
	mine, _ := svc.ListMessages(ctx, owner.ID, dm.ID, conversations.ListMessagesInput{})
	if len(mine.Messages) != 2 {
		t.Errorf("owner sees %d messages, want both", len(mine.Messages))
	}

	stranger := testutil.CreateAgent(t, db, owner)
	if _, err := svc.ListMessagesForAgent(ctx, stranger.ID, dm.ID, conversations.ListMessagesInput{}); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger agent got %v, want not_participant", err)
	}
}

func TestGetForAgent(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	conv, err := f.svc.GetForAgent(ctx, f.agent.ID, f.dm.ID)
	if err != nil || conv.ID != f.dm.ID || len(conv.Participants) != 2 {
		t.Errorf("GetForAgent = %+v, %v", conv, err)
	}
	stranger := testutil.CreateAgent(t, f.db, f.other)
	if _, err := f.svc.GetForAgent(ctx, stranger.ID, f.dm.ID); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger got %v, want not_participant", err)
	}
}
