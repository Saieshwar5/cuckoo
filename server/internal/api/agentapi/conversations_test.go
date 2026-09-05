package agentapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

type messageJSON struct {
	ID     string `json:"id"`
	Sender struct {
		Kind        string `json:"kind"`
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"sender"`
	Body struct {
		Text string `json:"text"`
	} `json:"body"`
}

type chatFixture struct {
	db          *store.Store
	tx          pgx.Tx
	srv         *testutil.Server
	owner       users.User
	agent       agents.Agent
	secret      string
	otherSecret string
	dmID        string
}

func setupChat(t *testing.T) *chatFixture {
	t.Helper()
	db, tx := testutil.NewStoreTx(t)
	f := &chatFixture{db: db, tx: tx, srv: testutil.NewServer(t, db)}
	f.owner = testutil.CreateUser(t, db, testutil.WithDisplayName("Priya"))
	f.agent = testutil.CreateAgent(t, db, f.owner, testutil.WithHandle("sbi-support"), testutil.WithAgentName("SBI Support"))
	_, f.secret = testutil.BindWebhook(t, db, f.agent, "https://example.com/cuckoo")
	_, f.otherSecret = testutil.BindAgent(t, db, testutil.CreateAgent(t, db, f.owner))
	f.dmID = domain.FormatID(domain.PrefixConv, testutil.OwnerDM(t, db, f.agent).ID)
	return f
}

func (f *chatFixture) userSends(t *testing.T, text string) {
	t.Helper()
	f.srv.AsUser(t, f.owner).Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{"text": text}).
		ExpectStatus(http.StatusCreated)
}

func TestGetConversation(t *testing.T) {
	f := setupChat(t)

	var env struct {
		Conversation struct {
			ID           string `json:"id"`
			Kind         string `json:"kind"`
			Participants []struct {
				Kind        string `json:"kind"`
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				Handle      string `json:"handle"`
				IsMe        bool   `json:"is_me"`
			} `json:"participants"`
		} `json:"conversation"`
	}
	f.srv.AsAgent(t, f.secret).Get("/v1/agent/conversations/" + f.dmID).ExpectStatus(http.StatusOK).Decode(&env)
	c := env.Conversation
	if c.ID != f.dmID || c.Kind != "dm" || len(c.Participants) != 2 {
		t.Fatalf("conversation = %+v", c)
	}
	for _, p := range c.Participants {
		switch p.Kind {
		case "agent":
			if !p.IsMe || p.Handle != "sbi-support" || p.DisplayName != "SBI Support" {
				t.Errorf("agent participant = %+v, want is_me with handle and name", p)
			}
		case "user":
			if p.IsMe || p.DisplayName != "Priya" || !strings.HasPrefix(p.ID, "usr_") {
				t.Errorf("user participant = %+v", p)
			}
		}
	}

	t.Run("another agent", func(t *testing.T) {
		f.srv.AsAgent(t, f.otherSecret).Get("/v1/agent/conversations/"+f.dmID).
			ExpectError(http.StatusForbidden, "not_participant")
	})
	t.Run("missing", func(t *testing.T) {
		f.srv.AsAgent(t, f.secret).Get("/v1/agent/conversations/"+domain.FormatID(domain.PrefixConv, domain.NewID())).
			ExpectError(http.StatusNotFound, "conversation_not_found")
	})
	t.Run("anonymous", func(t *testing.T) {
		f.srv.Anonymous(t).Get("/v1/agent/conversations/" + f.dmID).ExpectStatus(http.StatusUnauthorized)
	})
}

// The round trip the whole product is for: a person sends, the agent reads
// it and replies, the person sees the reply.
func TestReply(t *testing.T) {
	f := setupChat(t)
	f.userSends(t, "my UPI payment failed")
	agentClient := f.srv.AsAgent(t, f.secret)
	path := "/v1/agent/conversations/" + f.dmID + "/messages"

	var history struct {
		Messages   []messageJSON `json:"messages"`
		NextBefore *string       `json:"next_before"`
	}
	agentClient.Get(path).ExpectStatus(http.StatusOK).Decode(&history)
	if len(history.Messages) != 1 || history.Messages[0].Body.Text != "my UPI payment failed" ||
		history.Messages[0].Sender.Kind != "user" || history.Messages[0].Sender.DisplayName != "Priya" {
		t.Fatalf("agent's history = %+v", history.Messages)
	}

	var env struct {
		Message messageJSON `json:"message"`
	}
	agentClient.Post(path, map[string]any{
		"text": "I can see the deduction.", "idempotency_key": "reply-1",
	}).ExpectStatus(http.StatusCreated).Decode(&env)
	reply := env.Message
	if !strings.HasPrefix(reply.ID, "msg_") || reply.Sender.Kind != "agent" ||
		reply.Sender.ID != domain.FormatID(domain.PrefixAgent, f.agent.ID) || reply.Sender.DisplayName != "SBI Support" {
		t.Errorf("reply = %+v", reply)
	}

	t.Run("retry replays", func(t *testing.T) {
		var again struct {
			Message messageJSON `json:"message"`
		}
		agentClient.Post(path, map[string]any{"text": "I can see the deduction.", "idempotency_key": "reply-1"}).
			ExpectStatus(http.StatusOK).Decode(&again)
		if again.Message.ID != reply.ID {
			t.Errorf("retry created %s, want %s again", again.Message.ID, reply.ID)
		}
	})

	t.Run("the person sees it", func(t *testing.T) {
		var page struct {
			Messages []struct {
				ID     string `json:"id"`
				Sender struct {
					Kind string `json:"kind"`
				} `json:"sender"`
				DeliveryStatus *string `json:"delivery_status"`
			} `json:"messages"`
		}
		f.srv.AsUser(t, f.owner).Get("/v1/client/conversations/" + f.dmID + "/messages").
			ExpectStatus(http.StatusOK).Decode(&page)
		if len(page.Messages) != 2 || page.Messages[0].ID != reply.ID || page.Messages[0].Sender.Kind != "agent" {
			t.Fatalf("owner's history = %+v, want the reply newest", page.Messages)
		}
		if page.Messages[0].DeliveryStatus != nil {
			t.Errorf("agent reply has delivery_status %q, want null", *page.Messages[0].DeliveryStatus)
		}
	})

	t.Run("agent history pages newest first", func(t *testing.T) {
		var page struct {
			Messages   []messageJSON `json:"messages"`
			NextBefore *string       `json:"next_before"`
		}
		agentClient.Get(path + "?limit=1").ExpectStatus(http.StatusOK).Decode(&page)
		if len(page.Messages) != 1 || page.Messages[0].ID != reply.ID || page.NextBefore == nil {
			t.Fatalf("page = %+v", page)
		}
		agentClient.Get(path + "?limit=1&before=" + *page.NextBefore).ExpectStatus(http.StatusOK).Decode(&page)
		if len(page.Messages) != 1 || page.Messages[0].Sender.Kind != "user" || page.NextBefore != nil {
			t.Errorf("older page = %+v", page)
		}
	})
}

func TestReplyRejections(t *testing.T) {
	f := setupChat(t)
	path := "/v1/agent/conversations/" + f.dmID + "/messages"

	t.Run("empty text", func(t *testing.T) {
		f.srv.AsAgent(t, f.secret).Post(path, map[string]any{"text": " "}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_text")
	})
	t.Run("unknown field", func(t *testing.T) {
		f.srv.AsAgent(t, f.secret).Post(path, map[string]any{"text": "hi", "priority": "high"}).
			ExpectError(http.StatusUnprocessableEntity, "unknown_field")
	})
	t.Run("another agent", func(t *testing.T) {
		f.srv.AsAgent(t, f.otherSecret).Post(path, map[string]any{"text": "hi"}).
			ExpectError(http.StatusForbidden, "not_participant")
	})
	t.Run("a person on the agent API", func(t *testing.T) {
		f.srv.AsUser(t, f.owner).Post(path, map[string]any{"text": "hi"}).ExpectStatus(http.StatusUnauthorized)
	})
	t.Run("anonymous", func(t *testing.T) {
		f.srv.Anonymous(t).Post(path, map[string]any{"text": "hi"}).ExpectStatus(http.StatusUnauthorized)
	})
}
