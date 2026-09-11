package conversations_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// stopEvents is every stop.requested row the agent has in its outbox.
func stopEvents(t *testing.T, f *fixture) []gen.MessageDelivery {
	t.Helper()
	rows, err := f.db.ListDeliveriesSince(context.Background(), gen.ListDeliveriesSinceParams{
		AgentID: f.agent.ID, PageSize: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var out []gen.MessageDelivery
	for _, r := range rows {
		if r.EventType == conversations.EventStopRequested {
			out = append(out, r)
		}
	}
	return out
}

// Stop ends the reply where it stands, whatever the agent does next, and
// tells the agent.
func TestStopEndsTheReplyAndTellsTheAgent(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	testutil.BindAgent(t, f.db, f.agent)
	svc, rec := streaming(t, f)

	started, err := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, "It is 31 degrees and"); err != nil {
		t.Fatal(err)
	}

	ended, err := svc.Stop(ctx, f.owner.ID, f.dm.ID)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(ended) != 1 || ended[0].ID != started.ID {
		t.Fatalf("ended = %+v, want the open reply", ended)
	}
	m := ended[0]
	if m.Status != conversations.MessageComplete || !m.Stopped || !m.Truncated || m.Body.Text != "It is 31 degrees and" {
		t.Errorf("stopped reply = %+v, want complete, stopped, truncated, text kept", m)
	}

	// Every screen hears the reply is final and the indicator is gone.
	var done conversations.MessageCreatedEvent
	_ = json.Unmarshal(rec.latest(t, conversations.EventMessageCompleted).Payload, &done)
	if done.Message.ID != started.ID || !done.Message.Stopped {
		t.Errorf("completed = %+v, want the stopped reply", done.Message)
	}
	if p := activityOf(t, rec.latest(t, conversations.EventActivity)); p.State != "idle" || p.AgentID != f.agent.ID {
		t.Errorf("activity after stop = %+v, want idle", p)
	}

	// The agent's next word is refused, with a code that says why.
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, " sunny"); domain.CodeOf(err) != "stopped" {
		t.Errorf("append after stop got %v, want stopped", err)
	}
	// Finishing it is harmless: the reply as the person left it.
	again, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{})
	if err != nil || !again.Stopped || again.Body.Text != "It is 31 degrees and" {
		t.Errorf("finish after stop = %+v, %v", again, err)
	}

	stops := stopEvents(t, f)
	if len(stops) != 1 || stops[0].Status != "pending" {
		t.Fatalf("stop events = %+v, want one pending", stops)
	}
	var p conversations.StopPayload
	_ = json.Unmarshal(stops[0].Payload, &p)
	if p.MessageID == nil || *p.MessageID != started.ID {
		t.Errorf("stop payload = %+v, want the ended reply", p)
	}
}

// An agent still thinking has no words to cut, and is told all the same.
func TestStopWhileThinking(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	testutil.BindAgent(t, f.db, f.agent)
	svc, _ := streaming(t, f)

	ended, err := svc.Stop(ctx, f.owner.ID, f.dm.ID)
	if err != nil || len(ended) != 0 {
		t.Fatalf("Stop = %v, %v; want nothing ended", ended, err)
	}
	stops := stopEvents(t, f)
	if len(stops) != 1 {
		t.Fatalf("stop events = %d, want 1", len(stops))
	}
	var p conversations.StopPayload
	_ = json.Unmarshal(stops[0].Payload, &p)
	if p.MessageID != nil {
		t.Errorf("stop payload names %v, want no message", p.MessageID)
	}
}

func TestStopRejections(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc, _ := streaming(t, f)

	if _, err := svc.Stop(ctx, f.other.ID, f.dm.ID); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger stop got %v", err)
	}
	limited := conversations.New(f.db, conversations.WithLimiter(refusing{wait: time.Second}),
		conversations.WithStreams(testutil.NewStreamStore(t)))
	if _, err := limited.Stop(ctx, f.owner.ID, f.dm.ID); domain.CodeOf(err) != "rate_limited" {
		t.Errorf("limited stop got %v", err)
	}
}
