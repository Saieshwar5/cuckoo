package conversations_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// streaming returns a service with a stream store and a recorder, for the
// tests in this file.
func streaming(t *testing.T, f *fixture) (*conversations.Service, *recorder) {
	t.Helper()
	rec := &recorder{}
	return conversations.New(f.db,
		conversations.WithPublisher(rec),
		conversations.WithStreams(testutil.NewStreamStore(t)),
	), rec
}

func deltaText(t *testing.T, rec *recorder) string {
	t.Helper()
	var p conversations.MessageDeltaEvent
	if err := json.Unmarshal(rec.last(t, conversations.EventMessageDelta).Payload, &p); err != nil {
		t.Fatalf("delta payload: %v", err)
	}
	return p.Text
}

// A message is born empty, grows, is readable while growing, and is
// finished once.
func TestStreamLifecycle(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc, rec := streaming(t, f)

	started, err := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{})
	if err != nil {
		t.Fatalf("StartStream: %v", err)
	}
	if started.Status != conversations.MessageStreaming || started.Body.Text != "" || started.Sender.ID != f.agent.ID {
		t.Fatalf("started = %+v, want an empty streaming message from the agent", started.Message)
	}
	rec.last(t, conversations.EventMessageStarted)

	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, "I can see "); err != nil {
		t.Fatalf("AppendStream: %v", err)
	}
	if got := deltaText(t, rec); got != "I can see " {
		t.Errorf("delta announced %q", got)
	}
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, "the deduction."); err != nil {
		t.Fatalf("AppendStream: %v", err)
	}

	// A reader arriving mid-stream sees the text so far, everywhere a
	// message is shown.
	page, _ := svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if len(page.Messages) != 1 || page.Messages[0].Status != conversations.MessageStreaming ||
		page.Messages[0].Body.Text != "I can see the deduction." {
		t.Errorf("mid-stream history = %+v, want the text so far", page.Messages)
	}
	list, _ := svc.ListMine(ctx, f.owner.ID)
	if list[0].LastMessage == nil || list[0].LastMessage.Body.Text != "I can see the deduction." {
		t.Errorf("mid-stream preview = %+v", list[0].LastMessage)
	}
	theirs, _ := svc.ListMessagesForAgent(ctx, f.agent.ID, f.dm.ID, conversations.ListMessagesInput{})
	if len(theirs.Messages) != 1 || theirs.Messages[0].Body.Text != "I can see the deduction." {
		t.Errorf("agent's mid-stream history = %+v", theirs.Messages)
	}

	done, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{})
	if err != nil {
		t.Fatalf("FinishStream: %v", err)
	}
	if done.Status != conversations.MessageComplete || done.Truncated || done.Body.Text != "I can see the deduction." {
		t.Errorf("finished = %+v", done)
	}
	rec.last(t, conversations.EventMessageCompleted)
	page, _ = svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if page.Messages[0].Status != conversations.MessageComplete || page.Messages[0].Body.Text != "I can see the deduction." {
		t.Errorf("history after finish = %+v, want the text in the row", page.Messages[0])
	}

	again, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{})
	if err != nil || again.ID != done.ID || again.Status != conversations.MessageComplete {
		t.Errorf("second finish = %+v, %v; want the finished message again", again, err)
	}
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, "too late"); domain.CodeOf(err) != "not_streaming" {
		t.Errorf("append after finish got %v, want not_streaming", err)
	}
}

func TestStreamBelongsToItsAgent(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc, _ := streaming(t, f)
	stranger := testutil.CreateAgent(t, f.db, f.other)
	started, _ := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{})

	if err := svc.AppendStream(ctx, stranger.ID, started.ID, "mine now"); domain.CodeOf(err) != "not_streaming" {
		t.Errorf("stranger append got %v, want not_streaming", err)
	}
	if _, err := svc.FinishStream(ctx, stranger.ID, started.ID, conversations.FinishInput{}); domain.CodeOf(err) != "stream_not_found" {
		t.Errorf("stranger finish got %v, want stream_not_found", err)
	}
	if _, err := svc.StartStream(ctx, stranger.ID, f.dm.ID, conversations.SendInput{}); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger start got %v, want not_participant", err)
	}
}

func TestStreamRejections(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc, _ := streaming(t, f)

	if _, err := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "hello"}); domain.CodeOf(err) != "stream_with_text" {
		t.Errorf("start with text got %v, want stream_with_text", err)
	}
	started, _ := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{})
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, ""); domain.CodeOf(err) != "invalid_text" {
		t.Errorf("empty delta got %v, want invalid_text", err)
	}
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, strings.Repeat("x", 33000)); domain.CodeOf(err) != "stream_too_long" {
		t.Errorf("oversized delta got %v, want stream_too_long", err)
	}

	// Over the character limit but under the byte cap: finished, clamped,
	// and marked truncated.
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, strings.Repeat("y", 8001)); err != nil {
		t.Fatalf("append 8001: %v", err)
	}
	done, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{})
	if err != nil || len(done.Body.Text) != 8000 || !done.Truncated {
		t.Errorf("finished = len %d truncated %v, %v; want 8000 and truncated", len(done.Body.Text), done.Truncated, err)
	}

	plain := conversations.New(f.db)
	if _, err := plain.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{}); domain.KindOf(err) != domain.KindInternal {
		t.Errorf("start without a stream store got %v, want internal", err)
	}
}

// A stream that goes quiet is finished for the agent, with what it has.
func TestSweepStreamsCutsOffIdleStreams(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc, rec := streaming(t, f)
	started, _ := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{})
	if err := svc.AppendStream(ctx, f.agent.ID, started.ID, "half a"); err != nil {
		t.Fatal(err)
	}

	if n, err := svc.SweepStreams(ctx, time.Hour); err != nil || n != 0 {
		t.Fatalf("sweep of an active stream: %d, %v; want nothing swept", n, err)
	}
	n, err := svc.SweepStreams(ctx, 0)
	if err != nil || n != 1 {
		t.Fatalf("sweep with no grace: %d, %v; want 1", n, err)
	}
	page, _ := svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	m := page.Messages[0]
	if m.Status != conversations.MessageComplete || !m.Truncated || m.Body.Text != "half a" {
		t.Errorf("swept message = %+v, want complete, truncated, with the text so far", m)
	}
	rec.last(t, conversations.EventMessageCompleted)

	if n, _ := svc.SweepStreams(ctx, 0); n != 0 {
		t.Errorf("second sweep finished %d again", n)
	}
	if _, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{}); err != nil {
		t.Errorf("agent finishing after the sweep: %v, want the finished message", err)
	}
}

// If the buffer's bookkeeping is lost, a row stuck streaming is still
// finished once it is old enough.
func TestSweepStreamsFinishesStuckRows(t *testing.T) {
	ctx := context.Background()
	db, tx := testutil.NewStoreTx(t)
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner)
	dm := testutil.OwnerDM(t, db, agent)
	svc := conversations.New(db, conversations.WithStreams(testutil.NewStreamStore(t)))

	started, _ := svc.StartStream(ctx, agent.ID, dm.ID, conversations.SendInput{})
	if _, err := tx.Exec(ctx, "UPDATE messages SET created_at = now() - interval '6 minutes' WHERE id = $1", started.ID); err != nil {
		t.Fatal(err)
	}
	n, err := svc.SweepStreams(ctx, time.Hour)
	if err != nil || n != 1 {
		t.Fatalf("stuck sweep: %d, %v; want 1", n, err)
	}
	page, _ := svc.ListMessages(ctx, owner.ID, dm.ID, conversations.ListMessagesInput{})
	if page.Messages[0].Status != conversations.MessageComplete || !page.Messages[0].Truncated {
		t.Errorf("stuck row = %+v, want complete and truncated", page.Messages[0])
	}
}
