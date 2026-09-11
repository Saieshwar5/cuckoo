package conversations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func unreadOf(t *testing.T, svc *conversations.Service, f *fixture) int {
	t.Helper()
	c, err := svc.GetMine(context.Background(), f.owner.ID, f.dm.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c.Unread
}

// The badge counts what the agent said since the person last read, never
// what they said themselves, and only ever goes down by reading.
func TestUnreadAndMarkRead(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))

	say := func(text string) conversations.Message {
		t.Helper()
		m, err := svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: text})
		if err != nil {
			t.Fatal(err)
		}
		return m.Message
	}
	first := say("one")
	say("two")
	if _, err := svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "mine"}); err != nil {
		t.Fatal(err)
	}
	third := say("three")
	if n := unreadOf(t, svc, f); n != 3 {
		t.Fatalf("unread = %d, want 3 (the person's own message does not count)", n)
	}

	if err := svc.MarkRead(ctx, f.owner.ID, f.dm.ID, third.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if n := unreadOf(t, svc, f); n != 0 {
		t.Errorf("unread after reading = %d, want 0", n)
	}
	var p conversations.ReadEvent
	ev := rec.last(t, conversations.EventConversationRead)
	_ = json.Unmarshal(ev.Payload, &p)
	if p.ReadUpTo != third.ID || len(ev.Recipients) != 1 || ev.Recipients[0] != f.owner.ID {
		t.Errorf("read event = %+v to %v", p, ev.Recipients)
	}

	// A device that was behind cannot unread anything, and says nothing.
	before := rec.count()
	if err := svc.MarkRead(ctx, f.owner.ID, f.dm.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	if n := unreadOf(t, svc, f); n != 0 || rec.count() != before {
		t.Errorf("reading backwards: unread %d, %d new events; want 0 and none", n, rec.count()-before)
	}

	// Hidden and cleared messages are out of sight, and out of the count.
	hidden := say("four")
	say("five")
	if err := svc.Hide(ctx, f.owner.ID, f.dm.ID, hidden.ID); err != nil {
		t.Fatal(err)
	}
	if n := unreadOf(t, svc, f); n != 1 {
		t.Errorf("unread with one hidden = %d, want 1", n)
	}
	if err := svc.Clear(ctx, f.owner.ID, f.dm.ID); err != nil {
		t.Fatal(err)
	}
	if n := unreadOf(t, svc, f); n != 0 {
		t.Errorf("unread after clearing = %d, want 0", n)
	}
}

func TestMarkReadRejections(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	second := testutil.CreateAgent(t, f.db, f.owner, testutil.WithHandle("second"))
	otherDM := testutil.OwnerDM(t, f.db, second)
	m, _ := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "hi"})
	elsewhere, _ := f.svc.SendAsAgent(ctx, second.ID, otherDM.ID, conversations.SendInput{Text: "hi"})

	if err := f.svc.MarkRead(ctx, f.other.ID, f.dm.ID, m.ID); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger got %v", err)
	}
	if err := f.svc.MarkRead(ctx, f.owner.ID, f.dm.ID, elsewhere.ID); domain.CodeOf(err) != "message_not_found" {
		t.Errorf("message from another chat got %v", err)
	}
	if err := f.svc.MarkRead(ctx, f.owner.ID, f.dm.ID, domain.NewID()); domain.CodeOf(err) != "message_not_found" {
		t.Errorf("missing message got %v", err)
	}
}
