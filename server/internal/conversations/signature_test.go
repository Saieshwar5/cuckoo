package conversations_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/signing"
)

func signer(t *testing.T, seed byte) *signing.Signer {
	t.Helper()
	s, err := signing.New(bytes.Repeat([]byte{seed}, signing.KeyLen))
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	return s
}

// A sent message carries the hub's mark, and the mark is over what was said:
// the same words under another key, or other words under the same key, do
// not verify.
func TestSentMessagesAreSigned(t *testing.T) {
	f := setup(t)
	svc := conversations.New(f.db, conversations.WithSigner(signer(t, 1)))
	ctx := context.Background()

	res, err := svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hello"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(res.Signature) == 0 {
		t.Fatal("a sent message has no signature")
	}
	if !svc.Verify(res.Message) {
		t.Error("the message does not verify with the key that signed it")
	}

	// It is on the row, not just in the reply.
	page, err := svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if err != nil || len(page.Messages) != 1 {
		t.Fatalf("list: %v, %d messages", err, len(page.Messages))
	}
	stored := page.Messages[0]
	if !bytes.Equal(stored.Signature, res.Signature) {
		t.Error("the stored signature differs from the one returned")
	}

	tampered := stored
	tampered.Body.Text = "hullo"
	if svc.Verify(tampered) {
		t.Error("changed words verify")
	}
	other := conversations.New(f.db, conversations.WithSigner(signer(t, 2)))
	if other.Verify(stored) {
		t.Error("another key verifies this hub's signature")
	}
	if conversations.New(f.db).Verify(stored) {
		t.Error("a service with no signer verifies")
	}
}

// Which button was tapped is recorded on the offering message afterwards;
// that must not break its signature.
func TestSignatureSurvivesAButtonBeingTaken(t *testing.T) {
	f := setup(t)
	svc := conversations.New(f.db, conversations.WithSigner(signer(t, 3)))
	ctx := context.Background()

	offer, err := svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{
		Text:    "Which one?",
		Buttons: [][]conversations.Button{{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}},
	})
	if err != nil {
		t.Fatalf("offer: %v", err)
	}
	if _, err := svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{
		Action: &conversations.Action{ButtonID: "a", SourceMessageID: offer.ID},
	}); err != nil {
		t.Fatalf("tap: %v", err)
	}
	page, err := svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, m := range page.Messages {
		if m.ID == offer.ID {
			if m.Body.SelectedButtonID != "a" {
				t.Fatalf("the tap was not recorded on the offer: %+v", m.Body)
			}
			if !svc.Verify(m) {
				t.Error("the offer no longer verifies after its button was taken")
			}
		}
	}
}

// Without a signer nothing is signed, which is what every other test of
// this package relies on.
func TestNoSignerMeansNoSignature(t *testing.T) {
	f := setup(t)
	res, err := f.svc.SendAsUser(context.Background(), f.owner.ID, f.dm.ID, conversations.SendInput{Text: "plain"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if res.Signature != nil {
		t.Errorf("an unsigned service produced a signature: %x", res.Signature)
	}
}
