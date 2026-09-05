package client_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type messagePageJSON struct {
	Messages   []messageJSON `json:"messages"`
	NextBefore *string       `json:"next_before"`
}

func send(t *testing.T, c *testutil.Client, convID, text string) messageJSON {
	t.Helper()
	var env struct {
		Message messageJSON `json:"message"`
	}
	c.Post("/v1/client/conversations/"+convID+"/messages", map[string]any{"text": text}).
		ExpectStatus(http.StatusCreated).Decode(&env)
	return env.Message
}

func TestSendMessage(t *testing.T) {
	f := setupChat(t)
	m := send(t, f.srv.AsUser(t, f.owner), f.dmID, "  hello  ")

	if !strings.HasPrefix(m.ID, "msg_") {
		t.Errorf("id = %q, want msg_ prefix", m.ID)
	}
	if m.ConversationID != f.dmID {
		t.Errorf("conversation_id = %q, want %q", m.ConversationID, f.dmID)
	}
	if m.Sender.Kind != "user" || m.Sender.ID != domain.FormatID(domain.PrefixUser, f.owner.ID) {
		t.Errorf("sender = %+v, want the caller", m.Sender)
	}
	if m.Body.Text != "hello" {
		t.Errorf("text = %q, want trimmed", m.Body.Text)
	}
	if m.CreatedAt == "" {
		t.Error("created_at missing")
	}
	// The fixture agent has no backend, so the tick is "failed" at once.
	if m.DeliveryStatus == nil || *m.DeliveryStatus != "failed" {
		t.Errorf("delivery_status = %v, want failed for an agent with no backend", m.DeliveryStatus)
	}

	// The chat list now previews it.
	var env struct {
		Conversations []conversationJSON `json:"conversations"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&env)
	if lm := env.Conversations[0].LastMessage; lm == nil || lm.ID != m.ID {
		t.Errorf("last_message = %+v, want the message just sent", lm)
	}
}

// With a backend connected the message waits for the worker, and the chat
// list preview carries the same tick.
func TestSendMessageIsPendingWhenBackendConnected(t *testing.T) {
	f := setupChat(t)
	testutil.BindWebhook(t, f.db, f.agent, "https://example.com/cuckoo")

	m := send(t, f.srv.AsUser(t, f.owner), f.dmID, "hello")
	if m.DeliveryStatus == nil || *m.DeliveryStatus != "pending" {
		t.Errorf("delivery_status = %v, want pending", m.DeliveryStatus)
	}

	var env struct {
		Conversations []conversationJSON `json:"conversations"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&env)
	if lm := env.Conversations[0].LastMessage; lm == nil || lm.DeliveryStatus == nil || *lm.DeliveryStatus != "pending" {
		t.Errorf("last_message = %+v, want delivery_status pending", lm)
	}
}

func TestSendMessageRejections(t *testing.T) {
	f := setupChat(t)
	c := f.srv.AsUser(t, f.owner)
	path := "/v1/client/conversations/" + f.dmID + "/messages"

	t.Run("empty text", func(t *testing.T) {
		resp := c.Post(path, map[string]any{"text": "   "}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_text")
		if resp.ErrorField() != "text" {
			t.Errorf("field = %q, want text", resp.ErrorField())
		}
	})
	t.Run("too long", func(t *testing.T) {
		c.Post(path, map[string]any{"text": strings.Repeat("x", 8001)}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_text")
	})
	t.Run("unknown field", func(t *testing.T) {
		c.Post(path, map[string]any{"text": "hi", "buttons": []string{}}).
			ExpectError(http.StatusUnprocessableEntity, "unknown_field")
	})
	t.Run("non-member", func(t *testing.T) {
		f.srv.AsUser(t, f.other).Post(path, map[string]any{"text": "hi"}).
			ExpectError(http.StatusForbidden, "not_participant")
	})
	t.Run("missing conversation", func(t *testing.T) {
		c.Post("/v1/client/conversations/"+domain.FormatID(domain.PrefixConv, domain.NewID())+"/messages",
			map[string]any{"text": "hi"}).
			ExpectError(http.StatusNotFound, "conversation_not_found")
	})
	t.Run("anonymous", func(t *testing.T) {
		f.srv.Anonymous(t).Post(path, map[string]any{"text": "hi"}).ExpectStatus(http.StatusUnauthorized)
	})
}

func TestListMessagesPaginates(t *testing.T) {
	f := setupChat(t)
	c := f.srv.AsUser(t, f.owner)
	for _, text := range []string{"one", "two", "three"} {
		send(t, c, f.dmID, text)
	}
	path := "/v1/client/conversations/" + f.dmID + "/messages"

	var first messagePageJSON
	c.Get(path + "?limit=2").ExpectStatus(http.StatusOK).Decode(&first)
	if len(first.Messages) != 2 || first.Messages[0].Body.Text != "three" || first.Messages[1].Body.Text != "two" {
		t.Fatalf("first page = %+v, want three then two", first.Messages)
	}
	if first.NextBefore == nil || *first.NextBefore != first.Messages[1].ID {
		t.Fatalf("next_before = %v, want the oldest id on the page", first.NextBefore)
	}

	var second messagePageJSON
	c.Get(path + "?limit=2&before=" + *first.NextBefore).ExpectStatus(http.StatusOK).Decode(&second)
	if len(second.Messages) != 1 || second.Messages[0].Body.Text != "one" {
		t.Errorf("second page = %+v, want just one", second.Messages)
	}
	if second.NextBefore != nil {
		t.Errorf("next_before = %q on the last page, want null", *second.NextBefore)
	}

	var all messagePageJSON
	c.Get(path).ExpectStatus(http.StatusOK).Decode(&all)
	if len(all.Messages) != 3 || all.NextBefore != nil {
		t.Errorf("default page = %d messages, next_before %v; want 3 and null", len(all.Messages), all.NextBefore)
	}
}

func TestListMessagesRejections(t *testing.T) {
	f := setupChat(t)
	c := f.srv.AsUser(t, f.owner)
	path := "/v1/client/conversations/" + f.dmID + "/messages"

	t.Run("bad cursor", func(t *testing.T) {
		resp := c.Get(path+"?before="+domain.FormatID(domain.PrefixConv, domain.NewID())).
			ExpectError(http.StatusUnprocessableEntity, "invalid_id")
		if resp.ErrorField() != "before" {
			t.Errorf("field = %q, want before", resp.ErrorField())
		}
	})
	t.Run("limit not a number", func(t *testing.T) {
		resp := c.Get(path+"?limit=ten").ExpectError(http.StatusUnprocessableEntity, "invalid_limit")
		if resp.ErrorField() != "limit" {
			t.Errorf("field = %q, want limit", resp.ErrorField())
		}
	})
	t.Run("limit too large", func(t *testing.T) {
		c.Get(path+"?limit=101").ExpectError(http.StatusUnprocessableEntity, "invalid_limit")
	})
	t.Run("non-member", func(t *testing.T) {
		f.srv.AsUser(t, f.other).Get(path).ExpectError(http.StatusForbidden, "not_participant")
	})
}
