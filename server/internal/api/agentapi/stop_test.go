package agentapi_test

import (
	"net/http"
	"testing"
)

// A person presses stop in the app: the reply ends where it stands, the
// agent's next word is refused as stopped, and the agent finds the stop in
// its events naming the reply.
func TestStopAcrossTheAPIs(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	person := f.srv.AsUser(t, f.owner)

	var started struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{"stream": true}).
		ExpectStatus(http.StatusCreated).Decode(&started)
	id := started.Message.ID
	agent.Post("/v1/agent/messages/"+id+"/append", map[string]any{"text": "Searching for"}).ExpectStatus(http.StatusNoContent)

	var stopped struct {
		Stopped []struct {
			ID      string `json:"id"`
			Stopped bool   `json:"stopped"`
			Body    struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"stopped"`
	}
	person.Post("/v1/client/conversations/"+f.dmID+"/stop", nil).ExpectStatus(http.StatusOK).Decode(&stopped)
	if len(stopped.Stopped) != 1 || stopped.Stopped[0].ID != id || !stopped.Stopped[0].Stopped || stopped.Stopped[0].Body.Text != "Searching for" {
		t.Fatalf("stop = %+v, want the open reply, stopped", stopped)
	}

	agent.Post("/v1/agent/messages/"+id+"/append", map[string]any{"text": " flights"}).ExpectError(http.StatusConflict, "stopped")

	var events struct {
		Events []struct {
			Type string `json:"type"`
			Data struct {
				Conversation struct {
					ID string `json:"id"`
				} `json:"conversation"`
				MessageID *string `json:"message_id"`
			} `json:"data"`
		} `json:"events"`
	}
	agent.Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&events)
	var found bool
	for _, ev := range events.Events {
		if ev.Type == "stop.requested" {
			found = true
			if ev.Data.Conversation.ID != f.dmID || ev.Data.MessageID == nil || *ev.Data.MessageID != id {
				t.Errorf("stop event = %+v, want this chat and the ended reply", ev.Data)
			}
		}
	}
	if !found {
		t.Errorf("events = %+v, want a stop.requested", events.Events)
	}
}

// The chat list carries the badge, and reading clears it.
func TestUnreadAcrossTheAPIs(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	person := f.srv.AsUser(t, f.owner)

	var sent struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{"text": "one"}).ExpectStatus(http.StatusCreated)
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{"text": "two"}).
		ExpectStatus(http.StatusCreated).Decode(&sent)

	unread := func() int {
		t.Helper()
		var env struct {
			Conversations []struct {
				ID          string `json:"id"`
				UnreadCount int    `json:"unread_count"`
			} `json:"conversations"`
		}
		person.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&env)
		for _, c := range env.Conversations {
			if c.ID == f.dmID {
				return c.UnreadCount
			}
		}
		t.Fatal("chat not listed")
		return -1
	}
	if n := unread(); n != 2 {
		t.Errorf("unread = %d, want 2", n)
	}
	person.Post("/v1/client/conversations/"+f.dmID+"/read", map[string]any{"message_id": sent.Message.ID}).
		ExpectStatus(http.StatusNoContent)
	if n := unread(); n != 0 {
		t.Errorf("unread after reading = %d, want 0", n)
	}
	person.Post("/v1/client/conversations/"+f.dmID+"/read", map[string]any{"message_id": "nope"}).
		ExpectError(http.StatusUnprocessableEntity, "invalid_id")
}
