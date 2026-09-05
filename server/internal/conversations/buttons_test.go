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

var twoButtons = [][]conversations.Button{{
	{ID: "acc-salary", Label: " Salary account ", Style: "primary"},
	{ID: "acc-savings", Label: "Savings"},
}}

// An agent asks with buttons; the person's tap is an ordinary message whose
// text is the label and whose body names the choice; the question records
// which button was taken.
func TestButtonsAndTaps(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	question, err := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{
		Text: "Which account?", Buttons: twoButtons,
		QuickReplies: []conversations.QuickReply{{Label: "Neither"}},
	})
	if err != nil {
		t.Fatalf("SendAsAgent: %v", err)
	}
	b := question.Body.Buttons
	if len(b) != 1 || len(b[0]) != 2 || b[0][0].Label != "Salary account" || b[0][0].Style != "primary" || b[0][1].Style != "default" {
		t.Errorf("buttons stored = %+v, want trimmed labels and a default style", b)
	}
	if len(question.Body.QuickReplies) != 1 || question.Body.QuickReplies[0].Label != "Neither" {
		t.Errorf("quick replies = %+v", question.Body.QuickReplies)
	}

	tapped, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{
		Action: &conversations.Action{ButtonID: "acc-salary", SourceMessageID: question.ID},
	})
	if err != nil {
		t.Fatalf("tap: %v", err)
	}
	if tapped.Body.Text != "Salary account" || tapped.Body.Action == nil ||
		tapped.Body.Action.ButtonID != "acc-salary" || tapped.Body.Action.SourceMessageID != question.ID {
		t.Errorf("tap message = %+v, want the label as text and the action recorded", tapped.Body)
	}
	if rows, _ := f.db.ListDeliveriesByMessage(ctx, tapped.ID); len(rows) != 1 {
		t.Errorf("a tap was queued for %d agents, want 1: it is a message like any other", len(rows))
	}

	page, _ := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if len(page.Messages) != 2 || page.Messages[1].Body.SelectedButtonID != "acc-salary" {
		t.Errorf("history = %+v, want the question to record the button taken", page.Messages)
	}

	// A second tap is allowed and recorded; the latest wins on the question.
	if _, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{
		Action: &conversations.Action{ButtonID: "acc-savings", SourceMessageID: question.ID},
	}); err != nil {
		t.Fatalf("second tap: %v", err)
	}
	page, _ = f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if page.Messages[2].Body.SelectedButtonID != "acc-savings" {
		t.Errorf("after a second tap the question records %q", page.Messages[2].Body.SelectedButtonID)
	}
}

func TestTapRejections(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	question, _ := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "Which?", Buttons: twoButtons})
	plain, _ := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "hello"})
	elsewhere := testutil.OwnerDM(t, f.db, testutil.CreateAgent(t, f.db, f.owner))

	cases := map[string]conversations.SendInput{
		"tap with text":         {Text: "also this", Action: &conversations.Action{ButtonID: "acc-salary", SourceMessageID: question.ID}},
		"unknown button":        {Action: &conversations.Action{ButtonID: "nope", SourceMessageID: question.ID}},
		"message without ones":  {Action: &conversations.Action{ButtonID: "acc-salary", SourceMessageID: plain.ID}},
		"message that is gone":  {Action: &conversations.Action{ButtonID: "acc-salary", SourceMessageID: domain.NewID()}},
		"user offering buttons": {Text: "pick", Buttons: twoButtons},
	}
	want := map[string]string{
		"tap with text": "invalid_action", "unknown button": "invalid_action", "message without ones": "invalid_action",
		"message that is gone": "invalid_action", "user offering buttons": "buttons_not_allowed",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, in)
			if domain.CodeOf(err) != want[name] {
				t.Errorf("got %v, want %s", err, want[name])
			}
		})
	}
	t.Run("tap from another conversation", func(t *testing.T) {
		_, err := f.svc.SendAsUser(ctx, f.owner.ID, elsewhere.ID, conversations.SendInput{
			Action: &conversations.Action{ButtonID: "acc-salary", SourceMessageID: question.ID},
		})
		if domain.CodeOf(err) != "invalid_action" {
			t.Errorf("got %v, want invalid_action", err)
		}
	})
	t.Run("agent sending an action", func(t *testing.T) {
		_, err := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{
			Action: &conversations.Action{ButtonID: "acc-salary", SourceMessageID: question.ID},
		})
		if domain.CodeOf(err) != "action_not_allowed" {
			t.Errorf("got %v, want action_not_allowed", err)
		}
	})
}

func TestButtonValidation(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	row := func(ids ...string) []conversations.Button {
		out := make([]conversations.Button, 0, len(ids))
		for _, id := range ids {
			out = append(out, conversations.Button{ID: id, Label: id})
		}
		return out
	}
	cases := map[string]conversations.SendInput{
		"four rows":       {Text: "x", Buttons: [][]conversations.Button{row("a"), row("b"), row("c"), row("d")}},
		"four in a row":   {Text: "x", Buttons: [][]conversations.Button{row("a", "b", "c", "d")}},
		"empty row":       {Text: "x", Buttons: [][]conversations.Button{{}}},
		"bad id":          {Text: "x", Buttons: [][]conversations.Button{{{ID: "has space", Label: "ok"}}}},
		"duplicate id":    {Text: "x", Buttons: [][]conversations.Button{row("a"), row("a")}},
		"long label":      {Text: "x", Buttons: [][]conversations.Button{{{ID: "a", Label: strings.Repeat("l", 41)}}}},
		"empty label":     {Text: "x", Buttons: [][]conversations.Button{{{ID: "a", Label: "  "}}}},
		"bad style":       {Text: "x", Buttons: [][]conversations.Button{{{ID: "a", Label: "ok", Style: "loud"}}}},
		"seven chips":     {Text: "x", QuickReplies: make([]conversations.QuickReply, 7)},
		"chip with break": {Text: "x", QuickReplies: []conversations.QuickReply{{Label: "a\nb"}}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, in)
			if domain.KindOf(err) != domain.KindInvalid {
				t.Errorf("got %v, want invalid", err)
			}
		})
	}
}

// A reply names the message it answers; readers get a preview of the
// original as it stands, never a copy.
func TestReplyTo(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	long := strings.Repeat("word ", 40)
	asked, _ := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "  my\n\npayment " + long})

	reply, err := f.svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "I see it.", ReplyTo: &asked.ID})
	if err != nil {
		t.Fatalf("SendAsAgent with reply: %v", err)
	}
	if reply.ReplyTo == nil || reply.ReplyTo.ID != asked.ID || reply.ReplyTo.SenderKind != conversations.ParticipantUser {
		t.Fatalf("ReplyTo = %+v, want the asked message by the user", reply.ReplyTo)
	}
	if !strings.HasPrefix(reply.ReplyTo.TextPreview, "my payment word") || !strings.HasSuffix(reply.ReplyTo.TextPreview, "…") {
		t.Errorf("preview = %q, want whitespace collapsed and cut with an ellipsis", reply.ReplyTo.TextPreview)
	}
	page, _ := f.svc.ListMessages(ctx, f.owner.ID, f.dm.ID, conversations.ListMessagesInput{})
	if page.Messages[0].ReplyTo == nil || page.Messages[0].ReplyTo.TextPreview == "" {
		t.Errorf("history reply = %+v, want the preview filled in", page.Messages[0].ReplyTo)
	}
	theirs, _ := f.svc.ListMessagesForAgent(ctx, f.agent.ID, f.dm.ID, conversations.ListMessagesInput{})
	if theirs.Messages[0].ReplyTo == nil || theirs.Messages[0].ReplyTo.TextPreview == "" {
		t.Errorf("agent's history reply = %+v, want the preview filled in", theirs.Messages[0].ReplyTo)
	}

	elsewhere := testutil.OwnerDM(t, f.db, testutil.CreateAgent(t, f.db, f.owner))
	if _, err := f.svc.SendAsUser(ctx, f.owner.ID, elsewhere.ID, conversations.SendInput{Text: "x", ReplyTo: &asked.ID}); domain.CodeOf(err) != "invalid_reply_to" {
		t.Errorf("reply across conversations got %v, want invalid_reply_to", err)
	}
	missing := domain.NewID()
	if _, err := f.svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "x", ReplyTo: &missing}); domain.CodeOf(err) != "invalid_reply_to" {
		t.Errorf("reply to nothing got %v, want invalid_reply_to", err)
	}
}

func TestTyping(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))

	if err := svc.Typing(ctx, f.agent.ID, f.dm.ID, "start"); err != nil {
		t.Fatalf("Typing start: %v", err)
	}
	ev := rec.last(t, conversations.EventTyping)
	var p conversations.TypingEvent
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatal(err)
	}
	if len(ev.Recipients) != 1 || ev.Recipients[0] != f.owner.ID || p.State != "start" || p.AgentID != f.agent.ID {
		t.Errorf("typing event = %+v to %v", p, ev.Recipients)
	}
	if until := time.Until(p.ExpiresAt); until < 8*time.Second || until > 11*time.Second {
		t.Errorf("start expires in %v, want about ten seconds", until)
	}

	if err := svc.Typing(ctx, f.agent.ID, f.dm.ID, "stop"); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(rec.last(t, conversations.EventTyping).Payload, &p)
	if p.State != "stop" || p.ExpiresAt.After(time.Now()) {
		t.Errorf("stop event = %+v, want expired at once", p)
	}

	if err := svc.Typing(ctx, f.agent.ID, f.dm.ID, "thinking"); domain.CodeOf(err) != "invalid_state" {
		t.Errorf("bad state got %v", err)
	}
	stranger := testutil.CreateAgent(t, f.db, f.other)
	if err := svc.Typing(ctx, stranger.ID, f.dm.ID, "start"); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger typing got %v", err)
	}
}

func TestFinishStreamWithButtons(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc, _ := streaming(t, f)

	if _, err := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Buttons: twoButtons}); domain.CodeOf(err) != "stream_with_buttons" {
		t.Errorf("start with buttons got %v", err)
	}
	started, _ := svc.StartStream(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{})
	_ = svc.AppendStream(ctx, f.agent.ID, started.ID, "Raise a complaint?")
	done, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{Buttons: twoButtons})
	if err != nil {
		t.Fatalf("FinishStream: %v", err)
	}
	if len(done.Body.Buttons) != 1 || done.Body.Text != "Raise a complaint?" {
		t.Errorf("finished = %+v, want text and buttons", done.Body)
	}
	if _, err := svc.FinishStream(ctx, f.agent.ID, started.ID, conversations.FinishInput{
		Buttons: [][]conversations.Button{{{ID: "bad id!", Label: "x"}}},
	}); domain.CodeOf(err) != "invalid_buttons" {
		t.Errorf("finish with bad buttons got %v", err)
	}
}
