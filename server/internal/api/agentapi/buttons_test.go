package agentapi_test

import (
	"net/http"
	"testing"
)

type richMessageJSON struct {
	ID   string `json:"id"`
	Body struct {
		Text    string `json:"text"`
		Buttons [][]struct {
			ID    string `json:"id"`
			URL   string `json:"url"`
			Label string `json:"label"`
			Style string `json:"style"`
		} `json:"buttons"`
		QuickReplies []struct {
			Label string `json:"label"`
		} `json:"quick_replies"`
		Action *struct {
			ButtonID        string `json:"button_id"`
			SourceMessageID string `json:"source_message_id"`
		} `json:"action"`
		SelectedButtonID string `json:"selected_button_id"`
	} `json:"body"`
	ReplyTo *struct {
		ID          string `json:"id"`
		SenderKind  string `json:"sender_kind"`
		TextPreview string `json:"text_preview"`
	} `json:"reply_to"`
}

// Over HTTP: the agent asks with buttons, the person taps through the app's
// API, the agent sees the tap and answers quoting the original.
func TestButtonsOverHTTP(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	person := f.srv.AsUser(t, f.owner)
	agentMessages := "/v1/agent/conversations/" + f.dmID + "/messages"
	personMessages := "/v1/client/conversations/" + f.dmID + "/messages"

	var asked struct {
		Message richMessageJSON `json:"message"`
	}
	person.Post(personMessages, map[string]any{"text": "my payment failed"}).ExpectStatus(http.StatusCreated).Decode(&asked)

	var q struct {
		Message richMessageJSON `json:"message"`
	}
	agent.Post(agentMessages, map[string]any{
		"text":     "Which account?",
		"reply_to": asked.Message.ID,
		"buttons": [][]map[string]any{{
			{"id": "acc-salary", "label": "Salary account", "style": "primary"},
			{"id": "acc-savings", "label": "Savings"},
		}},
		"quick_replies": []map[string]any{{"label": "Neither"}},
	}).ExpectStatus(http.StatusCreated).Decode(&q)
	if len(q.Message.Body.Buttons) != 1 || q.Message.Body.Buttons[0][1].Style != "default" || len(q.Message.Body.QuickReplies) != 1 {
		t.Fatalf("question = %+v", q.Message.Body)
	}
	if q.Message.ReplyTo == nil || q.Message.ReplyTo.ID != asked.Message.ID || q.Message.ReplyTo.SenderKind != "user" ||
		q.Message.ReplyTo.TextPreview != "my payment failed" {
		t.Errorf("reply_to = %+v", q.Message.ReplyTo)
	}

	var tap struct {
		Message richMessageJSON `json:"message"`
	}
	person.Post(personMessages, map[string]any{
		"action": map[string]any{"button_id": "acc-salary", "source_message_id": q.Message.ID},
	}).ExpectStatus(http.StatusCreated).Decode(&tap)
	if tap.Message.Body.Text != "Salary account" || tap.Message.Body.Action == nil ||
		tap.Message.Body.Action.ButtonID != "acc-salary" || tap.Message.Body.Action.SourceMessageID != q.Message.ID {
		t.Errorf("tap = %+v", tap.Message.Body)
	}

	var history struct {
		Messages []richMessageJSON `json:"messages"`
	}
	agent.Get(agentMessages).ExpectStatus(http.StatusOK).Decode(&history)
	if len(history.Messages) != 3 || history.Messages[0].Body.Action == nil || history.Messages[1].Body.SelectedButtonID != "acc-salary" {
		t.Errorf("agent's history = %+v, want the tap newest and the question marked", history.Messages)
	}

	t.Run("rejections", func(t *testing.T) {
		person.Post(personMessages, map[string]any{"text": "x", "buttons": [][]map[string]any{{{"id": "a", "label": "a"}}}}).
			ExpectError(http.StatusUnprocessableEntity, "unknown_field")
		person.Post(personMessages, map[string]any{"action": map[string]any{"button_id": "nope", "source_message_id": q.Message.ID}}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_action")
		person.Post(personMessages, map[string]any{"action": map[string]any{"button_id": "acc-salary", "source_message_id": "bogus"}}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_id")
		agent.Post(agentMessages, map[string]any{"text": "x", "buttons": [][]map[string]any{{{"id": "bad id", "label": "a"}}}}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_buttons")
		agent.Post(agentMessages, map[string]any{"text": "x", "reply_to": "msg_nope"}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_id")
	})
}

// Over HTTP: a url button reaches the app with its link and no id, sits
// beside an ordinary one, and can never come back as a tap.
func TestURLButtonsOverHTTP(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	person := f.srv.AsUser(t, f.owner)
	agentMessages := "/v1/agent/conversations/" + f.dmID + "/messages"
	personMessages := "/v1/client/conversations/" + f.dmID + "/messages"

	var sent struct {
		Message richMessageJSON `json:"message"`
	}
	agent.Post(agentMessages, map[string]any{
		"text": "Order 4412 is on its way.",
		"buttons": [][]map[string]any{{
			{"url": "https://swiggy.com/track/4412", "label": "Track order", "style": "primary"},
			{"id": "help", "label": "Something is wrong"},
		}},
	}).ExpectStatus(http.StatusCreated).Decode(&sent)

	link := sent.Message.Body.Buttons[0][0]
	if link.URL != "https://swiggy.com/track/4412" || link.ID != "" || link.Style != "primary" {
		t.Fatalf("url button = %+v, want the link and no id", link)
	}

	// The app sees the same thing the agent sent.
	var seen struct {
		Messages []richMessageJSON `json:"messages"`
	}
	person.Get(personMessages).ExpectStatus(http.StatusOK).Decode(&seen)
	if got := seen.Messages[0].Body.Buttons[0][0]; got.URL != link.URL || got.ID != "" {
		t.Errorf("as the app sees it = %+v, want the link and no id", got)
	}

	// Nothing a person sends can turn a link into a choice.
	for _, id := range []string{"", "https://swiggy.com/track/4412"} {
		person.Post(personMessages, map[string]any{
			"action": map[string]any{"button_id": id, "source_message_id": sent.Message.ID},
		}).ExpectError(http.StatusUnprocessableEntity, "invalid_action")
	}

	agent.Post(agentMessages, map[string]any{"text": "x", "buttons": [][]map[string]any{
		{{"url": "javascript:alert(1)", "label": "Tap"}},
	}}).ExpectError(http.StatusUnprocessableEntity, "invalid_buttons")
	agent.Post(agentMessages, map[string]any{"text": "x", "buttons": [][]map[string]any{
		{{"id": "a", "url": "https://swiggy.in", "label": "Tap"}},
	}}).ExpectError(http.StatusUnprocessableEntity, "invalid_buttons")
}

func TestTypingOverHTTP(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	path := "/v1/agent/conversations/" + f.dmID + "/typing"

	agent.Post(path, map[string]any{"state": "start"}).ExpectStatus(http.StatusNoContent)
	agent.Post(path, map[string]any{"state": "stop"}).ExpectStatus(http.StatusNoContent)
	agent.Post(path, map[string]any{"state": "thinking"}).ExpectError(http.StatusUnprocessableEntity, "invalid_state")
	f.srv.AsAgent(t, f.otherSecret).Post(path, map[string]any{"state": "start"}).ExpectError(http.StatusForbidden, "not_participant")
}

func TestActivityOverHTTP(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	path := "/v1/agent/conversations/" + f.dmID + "/activity"

	agent.Post(path, map[string]any{"state": "thinking"}).ExpectStatus(http.StatusNoContent)
	agent.Post(path, map[string]any{"state": "working", "label": "Searching flights"}).ExpectStatus(http.StatusNoContent)
	agent.Post(path, map[string]any{"state": "idle"}).ExpectStatus(http.StatusNoContent)
	agent.Post(path, map[string]any{"state": "start"}).ExpectError(http.StatusUnprocessableEntity, "invalid_state")
	agent.Post(path, map[string]any{"state": "working", "label": "www.example.com"}).ExpectError(http.StatusUnprocessableEntity, "invalid_label")
}

func TestFinishWithButtonsOverHTTP(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	var env struct {
		Message richMessageJSON `json:"message"`
	}
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{"stream": true}).ExpectStatus(http.StatusCreated).Decode(&env)
	id := env.Message.ID
	agent.Post("/v1/agent/messages/"+id+"/append", map[string]any{"text": "Raise a complaint?"}).ExpectStatus(http.StatusNoContent)
	agent.Post("/v1/agent/messages/"+id+"/finish", map[string]any{
		"buttons": [][]map[string]any{{{"id": "yes", "label": "Yes", "style": "primary"}, {"id": "no", "label": "No", "style": "danger"}}},
	}).ExpectStatus(http.StatusOK).Decode(&env)
	if env.Message.Body.Text != "Raise a complaint?" || len(env.Message.Body.Buttons) != 1 || len(env.Message.Body.Buttons[0]) != 2 {
		t.Errorf("finished = %+v", env.Message.Body)
	}
}
