package mgmt_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

type agentJSON struct {
	ID          string `json:"id"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	HasAvatar   bool   `json:"has_avatar"`
	Binding     *struct {
		ID     string  `json:"id"`
		Mode   string  `json:"mode"`
		Status string  `json:"status"`
		URL    *string `json:"webhook_url"`
	} `json:"binding"`
}

type agentEnvelope struct {
	Agent agentJSON `json:"agent"`
}

func setup(t *testing.T) (*testutil.Server, users.User, users.User) {
	t.Helper()
	db := testutil.NewStore(t)
	owner := testutil.CreateUser(t, db, testutil.WithDisplayName("Owner"))
	other := testutil.CreateUser(t, db, testutil.WithDisplayName("Other"))
	return testutil.NewServer(t, db), owner, other
}

func create(t *testing.T, c *testutil.Client, handle string) agentJSON {
	t.Helper()
	var env agentEnvelope
	c.Post("/v1/mgmt/agents", map[string]any{
		"handle": handle, "display_name": "Agent " + handle, "description": "desc",
	}).ExpectStatus(http.StatusCreated).Decode(&env)
	return env.Agent
}

func TestCreateAgent(t *testing.T) {
	srv, owner, _ := setup(t)
	a := create(t, srv.AsUser(t, owner), "My-Helper")

	if !strings.HasPrefix(a.ID, "agt_") {
		t.Errorf("id = %q, want agt_ prefix", a.ID)
	}
	if a.Handle != "my-helper" {
		t.Errorf("handle = %q, want lowercased", a.Handle)
	}
	if a.Binding != nil {
		t.Errorf("a new agent has a binding: %+v", a.Binding)
	}
}

func TestCreateAgentRejections(t *testing.T) {
	srv, owner, other := setup(t)
	c := srv.AsUser(t, owner)
	create(t, c, "taken")

	t.Run("duplicate handle", func(t *testing.T) {
		c.Post("/v1/mgmt/agents", map[string]any{"handle": "TAKEN", "display_name": "x"}).
			ExpectError(http.StatusConflict, "handle_taken")
	})
	t.Run("duplicate by another owner", func(t *testing.T) {
		srv.AsUser(t, other).Post("/v1/mgmt/agents", map[string]any{"handle": "taken", "display_name": "x"}).
			ExpectError(http.StatusConflict, "handle_taken")
	})
	t.Run("bad handle", func(t *testing.T) {
		resp := c.Post("/v1/mgmt/agents", map[string]any{"handle": "no spaces", "display_name": "x"}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_handle")
		if resp.ErrorField() != "handle" {
			t.Errorf("field = %q", resp.ErrorField())
		}
	})
	t.Run("reserved handle", func(t *testing.T) {
		c.Post("/v1/mgmt/agents", map[string]any{"handle": "admin", "display_name": "x"}).
			ExpectError(http.StatusUnprocessableEntity, "handle_reserved")
	})
	t.Run("missing name", func(t *testing.T) {
		c.Post("/v1/mgmt/agents", map[string]any{"handle": "fine"}).
			ExpectError(http.StatusUnprocessableEntity, "invalid_display_name")
	})
	t.Run("unknown field", func(t *testing.T) {
		c.Post("/v1/mgmt/agents", map[string]any{"handle": "fine", "display_name": "x", "owner": "me"}).
			ExpectError(http.StatusUnprocessableEntity, "unknown_field")
	})
}

func TestListAgentsIsScopedToOwner(t *testing.T) {
	srv, owner, other := setup(t)
	create(t, srv.AsUser(t, owner), "mine-1")
	create(t, srv.AsUser(t, owner), "mine-2")
	create(t, srv.AsUser(t, other), "theirs")

	var env struct {
		Agents []agentJSON `json:"agents"`
	}
	srv.AsUser(t, owner).Get("/v1/mgmt/agents").ExpectStatus(http.StatusOK).Decode(&env)

	if len(env.Agents) != 2 {
		t.Fatalf("got %d agents, want 2: %+v", len(env.Agents), env.Agents)
	}
	if env.Agents[0].Handle != "mine-1" || env.Agents[1].Handle != "mine-2" {
		t.Errorf("wrong agents or order: %+v", env.Agents)
	}
}

func TestGetAgent(t *testing.T) {
	srv, owner, other := setup(t)
	a := create(t, srv.AsUser(t, owner), "readable")

	t.Run("owner", func(t *testing.T) {
		var env agentEnvelope
		srv.AsUser(t, owner).Get("/v1/mgmt/agents/" + a.ID).ExpectStatus(http.StatusOK).Decode(&env)
		if env.Agent.ID != a.ID {
			t.Errorf("got %q, want %q", env.Agent.ID, a.ID)
		}
	})
	t.Run("someone else", func(t *testing.T) {
		srv.AsUser(t, other).Get("/v1/mgmt/agents/"+a.ID).ExpectError(http.StatusForbidden, "not_owner")
	})
	t.Run("missing", func(t *testing.T) {
		srv.AsUser(t, owner).Get("/v1/mgmt/agents/"+domain.FormatID(domain.PrefixAgent, domain.NewID())).
			ExpectError(http.StatusNotFound, "agent_not_found")
	})
	t.Run("malformed id", func(t *testing.T) {
		srv.AsUser(t, owner).Get("/v1/mgmt/agents/not-an-id").ExpectError(http.StatusUnprocessableEntity, "invalid_id")
	})
	t.Run("wrong id type", func(t *testing.T) {
		srv.AsUser(t, owner).Get("/v1/mgmt/agents/"+domain.FormatID(domain.PrefixUser, owner.ID)).
			ExpectError(http.StatusUnprocessableEntity, "invalid_id")
	})
}

func TestUpdateAgent(t *testing.T) {
	srv, owner, other := setup(t)
	c := srv.AsUser(t, owner)
	a := create(t, c, "editable")

	var env agentEnvelope
	c.Patch("/v1/mgmt/agents/"+a.ID, map[string]any{"display_name": "Renamed"}).
		ExpectStatus(http.StatusOK).Decode(&env)
	if env.Agent.DisplayName != "Renamed" || env.Agent.Description != "desc" {
		t.Errorf("partial update wrong: %+v", env.Agent)
	}

	c.Patch("/v1/mgmt/agents/"+a.ID, map[string]any{"handle": "new"}).
		ExpectError(http.StatusUnprocessableEntity, "unknown_field") // handle is not editable
	c.Patch("/v1/mgmt/agents/"+a.ID, map[string]any{}).
		ExpectError(http.StatusUnprocessableEntity, "no_changes")
	srv.AsUser(t, other).Patch("/v1/mgmt/agents/"+a.ID, map[string]any{"display_name": "Hijacked"}).
		ExpectError(http.StatusForbidden, "not_owner")
}

func TestDeleteAgent(t *testing.T) {
	srv, owner, other := setup(t)
	c := srv.AsUser(t, owner)
	a := create(t, c, "deletable")

	srv.AsUser(t, other).Delete("/v1/mgmt/agents/"+a.ID).ExpectError(http.StatusForbidden, "not_owner")
	c.Delete("/v1/mgmt/agents/" + a.ID).ExpectStatus(http.StatusNoContent)
	c.Get("/v1/mgmt/agents/"+a.ID).ExpectError(http.StatusNotFound, "agent_not_found")
	c.Delete("/v1/mgmt/agents/"+a.ID).ExpectError(http.StatusNotFound, "agent_not_found")
}

// The management API accepts people, never agents. A binding secret presented
// here is a bearer token that is not a session, and is refused as such.
func TestManagementAPIRejectsAgentCredentials(t *testing.T) {
	srv, owner, _ := setup(t)
	agent := testutil.CreateAgent(t, srv.Store, owner)
	_, secret := testutil.BindAgent(t, srv.Store, agent)

	srv.AsAgent(t, secret).Get("/v1/mgmt/agents").ExpectError(http.StatusUnauthorized, "invalid_credentials")
	srv.Anonymous(t).Get("/v1/mgmt/agents").ExpectError(http.StatusUnauthorized, "missing_credentials")
}
