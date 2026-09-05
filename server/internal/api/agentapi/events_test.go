package agentapi_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type eventJSON struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Data    struct {
		Message struct {
			Body struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

type eventsEnvelope struct {
	Events []eventJSON `json:"events"`
}

func TestListEvents(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner)
	_, secret := testutil.BindWebhook(t, db, agent, "https://example.com/cuckoo")
	dm := domain.FormatID(domain.PrefixConv, testutil.OwnerDM(t, db, agent).ID)

	_, otherSecret := testutil.BindAgent(t, db, testutil.CreateAgent(t, db, owner))

	for _, text := range []string{"one", "two"} {
		srv.AsUser(t, owner).Post("/v1/client/conversations/"+dm+"/messages", map[string]any{"text": text}).
			ExpectStatus(http.StatusCreated)
	}

	var env eventsEnvelope
	srv.AsAgent(t, secret).Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&env)
	if len(env.Events) != 2 {
		t.Fatalf("got %d events, want 2: %+v", len(env.Events), env.Events)
	}
	first := env.Events[0]
	if first.Type != "message.created" || first.Data.Message.Body.Text != "one" {
		t.Errorf("first event = %+v, want message.created for \"one\"", first)
	}
	if first.AgentID != domain.FormatID(domain.PrefixAgent, agent.ID) {
		t.Errorf("agent_id = %q", first.AgentID)
	}

	t.Run("since", func(t *testing.T) {
		var page eventsEnvelope
		srv.AsAgent(t, secret).Get("/v1/agent/events?since=" + first.ID).ExpectStatus(http.StatusOK).Decode(&page)
		if len(page.Events) != 1 || page.Events[0].Data.Message.Body.Text != "two" {
			t.Errorf("since first = %+v, want just \"two\"", page.Events)
		}
	})
	t.Run("bad since", func(t *testing.T) {
		resp := srv.AsAgent(t, secret).Get("/v1/agent/events?since="+dm).
			ExpectError(http.StatusUnprocessableEntity, "invalid_id")
		if resp.ErrorField() != "since" {
			t.Errorf("field = %q, want since", resp.ErrorField())
		}
	})
	t.Run("bad limit", func(t *testing.T) {
		srv.AsAgent(t, secret).Get("/v1/agent/events?limit=lots").
			ExpectError(http.StatusUnprocessableEntity, "invalid_limit")
		srv.AsAgent(t, secret).Get("/v1/agent/events?limit=101").
			ExpectError(http.StatusUnprocessableEntity, "invalid_limit")
	})
	t.Run("scoped to the agent", func(t *testing.T) {
		var page eventsEnvelope
		srv.AsAgent(t, otherSecret).Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&page)
		if len(page.Events) != 0 {
			t.Errorf("another agent sees %d events", len(page.Events))
		}
	})
	t.Run("anonymous", func(t *testing.T) {
		srv.Anonymous(t).Get("/v1/agent/events").ExpectStatus(http.StatusUnauthorized)
	})
	t.Run("a person cannot read agent events", func(t *testing.T) {
		srv.AsUser(t, owner).Get("/v1/agent/events").ExpectStatus(http.StatusUnauthorized)
	})
}
