package agentapi_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

type streamMessageJSON struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Body   struct {
		Text string `json:"text"`
	} `json:"body"`
	Truncated bool `json:"truncated"`
}

// The HTTP form of a stream: start, append, finish.
func TestStreamOverHTTP(t *testing.T) {
	f := setupChat(t)
	c := f.srv.AsAgent(t, f.secret)
	messages := "/v1/agent/conversations/" + f.dmID + "/messages"

	var env struct {
		Message streamMessageJSON `json:"message"`
	}
	c.Post(messages, map[string]any{"stream": true}).ExpectStatus(http.StatusCreated).Decode(&env)
	if env.Message.Status != "streaming" || env.Message.Body.Text != "" {
		t.Fatalf("started = %+v, want an empty streaming message", env.Message)
	}
	id := env.Message.ID

	c.Post("/v1/agent/messages/"+id+"/append", map[string]any{"text": "hello "}).ExpectStatus(http.StatusNoContent)
	c.Post("/v1/agent/messages/"+id+"/append", map[string]any{"text": "world"}).ExpectStatus(http.StatusNoContent)

	var page struct {
		Messages []streamMessageJSON `json:"messages"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/client/conversations/" + f.dmID + "/messages").ExpectStatus(http.StatusOK).Decode(&page)
	if len(page.Messages) != 1 || page.Messages[0].Status != "streaming" || page.Messages[0].Body.Text != "hello world" {
		t.Errorf("mid-stream history = %+v, want the text so far", page.Messages)
	}

	c.Post("/v1/agent/messages/"+id+"/finish", nil).ExpectStatus(http.StatusOK).Decode(&env)
	if env.Message.Status != "complete" || env.Message.Body.Text != "hello world" || env.Message.Truncated {
		t.Errorf("finished = %+v", env.Message)
	}

	t.Run("append after finish", func(t *testing.T) {
		c.Post("/v1/agent/messages/"+id+"/append", map[string]any{"text": "late"}).
			ExpectError(http.StatusConflict, "not_streaming")
	})
	t.Run("finish again", func(t *testing.T) {
		c.Post("/v1/agent/messages/"+id+"/finish", nil).ExpectStatus(http.StatusOK)
	})
	t.Run("another agent", func(t *testing.T) {
		f.srv.AsAgent(t, f.otherSecret).Post("/v1/agent/messages/"+id+"/finish", nil).
			ExpectError(http.StatusNotFound, "stream_not_found")
	})
	t.Run("stream with text", func(t *testing.T) {
		c.Post(messages, map[string]any{"stream": true, "text": "no"}).
			ExpectError(http.StatusUnprocessableEntity, "stream_with_text")
	})
	t.Run("empty delta", func(t *testing.T) {
		c.Post(messages, map[string]any{"stream": true}).ExpectStatus(http.StatusCreated).Decode(&env)
		c.Post("/v1/agent/messages/"+env.Message.ID+"/append", map[string]any{"text": ""}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_text")
	})
	t.Run("unknown message", func(t *testing.T) {
		c.Post("/v1/agent/messages/"+domain.FormatID(domain.PrefixMessage, domain.NewID())+"/append",
			map[string]any{"text": "x"}).ExpectError(http.StatusConflict, "not_streaming")
	})
}
