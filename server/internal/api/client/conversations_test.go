package client_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

type participantJSON struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Handle      string `json:"handle"`
}

type messageJSON struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Sender         struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"sender"`
	Body struct {
		Text string `json:"text"`
	} `json:"body"`
	CreatedAt string `json:"created_at"`
}

type conversationJSON struct {
	ID           string            `json:"id"`
	Kind         string            `json:"kind"`
	Participants []participantJSON `json:"participants"`
	LastMessage  *messageJSON      `json:"last_message"`
}

type chatFixture struct {
	srv   *testutil.Server
	owner users.User
	other users.User
	agent agents.Agent
	dm    conversations.Conversation
	dmID  string
}

func setupChat(t *testing.T) *chatFixture {
	t.Helper()
	db := testutil.NewStore(t)
	f := &chatFixture{
		srv:   testutil.NewServer(t, db),
		owner: testutil.CreateUser(t, db, testutil.WithDisplayName("Priya")),
		other: testutil.CreateUser(t, db, testutil.WithDisplayName("Other")),
	}
	f.agent = testutil.CreateAgent(t, db, f.owner, testutil.WithHandle("sbi-support"), testutil.WithAgentName("SBI Support"))
	f.dm = testutil.OwnerDM(t, db, f.agent)
	f.dmID = domain.FormatID(domain.PrefixConv, f.dm.ID)
	return f
}

func TestListConversations(t *testing.T) {
	f := setupChat(t)

	var env struct {
		Conversations []conversationJSON `json:"conversations"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&env)

	if len(env.Conversations) != 1 {
		t.Fatalf("got %d conversations, want 1: %+v", len(env.Conversations), env.Conversations)
	}
	c := env.Conversations[0]
	if c.ID != f.dmID || c.Kind != "dm" || c.LastMessage != nil {
		t.Errorf("conversation = %+v", c)
	}
	if len(c.Participants) != 2 {
		t.Fatalf("participants = %+v, want 2", c.Participants)
	}
	for _, p := range c.Participants {
		switch p.Kind {
		case "user":
			if !strings.HasPrefix(p.ID, "usr_") || p.DisplayName != "Priya" || p.Handle != "" {
				t.Errorf("user participant = %+v", p)
			}
		case "agent":
			if !strings.HasPrefix(p.ID, "agt_") || p.DisplayName != "SBI Support" || p.Handle != "sbi-support" {
				t.Errorf("agent participant = %+v", p)
			}
		default:
			t.Errorf("unexpected participant kind %q", p.Kind)
		}
	}
}

func TestListConversationsIsEmptyForNewcomer(t *testing.T) {
	f := setupChat(t)
	var env struct {
		Conversations []conversationJSON `json:"conversations"`
	}
	f.srv.AsUser(t, f.other).Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&env)
	if len(env.Conversations) != 0 {
		t.Errorf("a user with no agents sees %+v", env.Conversations)
	}
}

func TestGetConversation(t *testing.T) {
	f := setupChat(t)

	t.Run("member", func(t *testing.T) {
		var env struct {
			Conversation conversationJSON `json:"conversation"`
		}
		f.srv.AsUser(t, f.owner).Get("/v1/client/conversations/" + f.dmID).
			ExpectStatus(http.StatusOK).Decode(&env)
		if env.Conversation.ID != f.dmID || len(env.Conversation.Participants) != 2 {
			t.Errorf("conversation = %+v", env.Conversation)
		}
	})
	t.Run("non-member", func(t *testing.T) {
		f.srv.AsUser(t, f.other).Get("/v1/client/conversations/"+f.dmID).
			ExpectError(http.StatusForbidden, "not_participant")
	})
	t.Run("missing", func(t *testing.T) {
		f.srv.AsUser(t, f.owner).Get("/v1/client/conversations/"+domain.FormatID(domain.PrefixConv, domain.NewID())).
			ExpectError(http.StatusNotFound, "conversation_not_found")
	})
	t.Run("wrong prefix", func(t *testing.T) {
		f.srv.AsUser(t, f.owner).Get("/v1/client/conversations/"+domain.FormatID(domain.PrefixAgent, f.dm.ID)).
			ExpectError(http.StatusUnprocessableEntity, "invalid_id")
	})
	t.Run("anonymous", func(t *testing.T) {
		f.srv.Anonymous(t).Get("/v1/client/conversations/" + f.dmID).
			ExpectStatus(http.StatusUnauthorized)
	})
}
