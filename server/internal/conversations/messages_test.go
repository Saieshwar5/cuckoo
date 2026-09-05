package conversations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func TestSendText(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	msg, err := f.svc.SendText(ctx, f.owner.ID, f.dm.ID, "  my UPI payment failed  ")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if msg.ConversationID != f.dm.ID {
		t.Errorf("ConversationID = %v, want %v", msg.ConversationID, f.dm.ID)
	}
	if msg.Sender.Kind != conversations.ParticipantUser || msg.Sender.ID != f.owner.ID {
		t.Errorf("Sender = %+v, want the calling user", msg.Sender)
	}
	if msg.Body.Text != "my UPI payment failed" {
		t.Errorf("Text = %q, want it trimmed", msg.Body.Text)
	}

	page, err := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(page.Messages) != 1 || page.Messages[0] != msg {
		t.Errorf("history = %+v, want exactly the sent message", page.Messages)
	}
	if page.NextBefore != nil {
		t.Errorf("NextBefore set on a single-page history")
	}
}

// Sending records who must hear the message in the same transaction. An
// agent with no backend gets a delivery that has already failed, so the
// sender is told now; an agent with one gets a pending delivery.
func TestSendTextRecordsDeliveriesForAgents(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	unheard, err := f.svc.SendText(ctx, f.owner.ID, f.dm.ID, "anyone?")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if unheard.DeliveryStatus != conversations.DeliveryFailed {
		t.Errorf("status with no backend = %q, want failed", unheard.DeliveryStatus)
	}
	rows, err := f.db.ListDeliveriesByMessage(ctx, unheard.ID)
	if err != nil || len(rows) != 1 || rows[0].AgentID != f.agent.ID || rows[0].Status != "failed" {
		t.Errorf("deliveries = %+v (%v), want one failed row for the agent", rows, err)
	}

	testutil.BindWebhook(t, f.db, f.agent, "https://example.com/cuckoo")
	heard, err := f.svc.SendText(ctx, f.owner.ID, f.dm.ID, "hello")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if heard.DeliveryStatus != conversations.DeliveryPending {
		t.Errorf("status with a backend = %q, want pending", heard.DeliveryStatus)
	}
	list, _ := f.svc.ListMine(ctx, f.owner.ID)
	if list[0].LastMessage == nil || list[0].LastMessage.DeliveryStatus != conversations.DeliveryPending {
		t.Errorf("chat list preview = %+v, want pending status", list[0].LastMessage)
	}
}

func TestSendTextRejectsInvalidText(t *testing.T) {
	f := setup(t)
	cases := map[string]string{
		"empty":        "",
		"only spaces":  "   \n\t ",
		"too long":     strings.Repeat("x", 8001),
		"nul byte":     "hello\x00world",
		"invalid utf8": "hi \xff there",
	}
	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.svc.SendText(context.Background(), f.owner.ID, f.dm.ID, text)
			if domain.CodeOf(err) != "invalid_text" {
				t.Errorf("got %v, want invalid_text", err)
			}
		})
	}

	// The limit is in characters, not bytes: 8000 Devanagari letters is a
	// legal message even though it is 24 000 bytes.
	if _, err := f.svc.SendText(context.Background(), f.owner.ID, f.dm.ID, strings.Repeat("क", 8000)); err != nil {
		t.Errorf("8000 non-ASCII characters rejected: %v", err)
	}
}

// Membership is checked before the text is looked at, so a stranger learns
// nothing about what the conversation would have accepted.
func TestSendTextRequiresMembership(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	_, err := f.svc.SendText(ctx, f.other.ID, f.dm.ID, "")
	if domain.CodeOf(err) != "not_participant" {
		t.Errorf("non-member got %v, want not_participant", err)
	}
	_, err = f.svc.SendText(ctx, f.owner.ID, domain.NewID(), "hi")
	if domain.CodeOf(err) != "conversation_not_found" {
		t.Errorf("missing conversation got %v, want conversation_not_found", err)
	}
}

func TestListMessagesPaginatesNewestFirst(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	texts := []string{"one", "two", "three", "four", "five"}
	for _, text := range texts {
		if _, err := f.svc.SendText(ctx, f.owner.ID, f.dm.ID, text); err != nil {
			t.Fatalf("SendText %q: %v", text, err)
		}
	}

	var got []string
	in := conversations.ListMessagesInput{Limit: 2}
	for pages := 0; ; pages++ {
		page, err := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, in)
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		for _, m := range page.Messages {
			got = append(got, m.Body.Text)
		}
		if page.NextBefore == nil {
			if pages != 2 {
				t.Errorf("history ended after %d pages, want 3", pages+1)
			}
			break
		}
		if len(page.Messages) != 2 {
			t.Errorf("page %d has %d messages, want a full page of 2", pages, len(page.Messages))
		}
		in.Before = page.NextBefore
		if pages > 5 {
			t.Fatal("pagination never terminates")
		}
	}

	want := "five,four,three,two,one"
	if strings.Join(got, ",") != want {
		t.Errorf("paged history = %q, want %q", strings.Join(got, ","), want)
	}
}

func TestListMessagesLimitBounds(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	for _, limit := range []int{-1, 101} {
		_, err := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{Limit: limit})
		if domain.CodeOf(err) != "invalid_limit" {
			t.Errorf("limit %d: got %v, want invalid_limit", limit, err)
		}
	}
	for _, limit := range []int{0, 1, 100} {
		if _, err := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{Limit: limit}); err != nil {
			t.Errorf("limit %d rejected: %v", limit, err)
		}
	}
}

func TestListMessagesRequiresMembership(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	_, err := f.svc.ListMessages(ctx, f.other.ID, f.dm.ID, conversations.ListMessagesInput{})
	if domain.CodeOf(err) != "not_participant" {
		t.Errorf("non-member got %v, want not_participant", err)
	}
}
