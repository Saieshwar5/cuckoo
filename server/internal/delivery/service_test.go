package delivery_test

import (
	"context"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func messageIDs(t *testing.T, list []events.Envelope) []string {
	t.Helper()
	out := make([]string, 0, len(list))
	for _, e := range list {
		data, ok := e.Data.(events.MessageCreated)
		if !ok {
			t.Fatalf("event %s carries %T, want MessageCreated", e.ID, e.Data)
		}
		out = append(out, data.Message.ID)
	}
	return out
}

// Catch-up is the same rows a webhook would have carried, in order, for this
// agent only, and regardless of whether anything was delivered.
func TestListEvents(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	var want []string
	for _, text := range []string{"one", "two", "three"} {
		want = append(want, domain.FormatID(domain.PrefixMessage, f.send(t, f.dm, text).ID))
	}

	// Another owner's agent has its own events, which must not leak.
	other := testutil.CreateUser(t, f.db)
	otherAgent := testutil.CreateAgent(t, f.db, other)
	if _, err := f.convs.SendText(ctx, other.ID, testutil.OwnerDM(t, f.db, otherAgent).ID, "theirs"); err != nil {
		t.Fatalf("other SendText: %v", err)
	}

	all, err := f.svc.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if got := messageIDs(t, all); len(got) != 3 || got[0] != want[0] || got[2] != want[2] {
		t.Errorf("events carry messages %v, want %v oldest first", got, want)
	}
	if all[0].Type != events.TypeMessageCreated || all[0].AgentID != domain.FormatID(domain.PrefixAgent, f.agent.ID) {
		t.Errorf("envelope = %+v", all[0])
	}

	since, err := domain.ParseID(domain.PrefixEvent, all[0].ID)
	if err != nil {
		t.Fatalf("event id %q does not parse: %v", all[0].ID, err)
	}
	rest, err := f.svc.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{Since: &since})
	if err != nil {
		t.Fatalf("ListEvents since: %v", err)
	}
	if got := messageIDs(t, rest); len(got) != 2 || got[0] != want[1] {
		t.Errorf("events since the first = %v, want the last two", got)
	}

	one, err := f.svc.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{Limit: 1})
	if err != nil || len(one) != 1 {
		t.Errorf("limit 1 returned %d events, %v", len(one), err)
	}
	if _, err := f.svc.ListEvents(ctx, f.agent.ID, delivery.ListEventsInput{Limit: 101}); domain.CodeOf(err) != "invalid_limit" {
		t.Errorf("limit 101 got %v, want invalid_limit", err)
	}

	theirs, _ := f.svc.ListEvents(ctx, otherAgent.ID, delivery.ListEventsInput{})
	if len(theirs) != 1 {
		t.Errorf("other agent sees %d events, want only its own 1", len(theirs))
	}
}
