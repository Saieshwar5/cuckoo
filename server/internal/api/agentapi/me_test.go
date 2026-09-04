package agentapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/auth"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type meEnvelope struct {
	Agent struct {
		ID          string `json:"id"`
		Handle      string `json:"handle"`
		DisplayName string `json:"display_name"`
		OwnerID     string `json:"owner_id"`
	} `json:"agent"`
}

func TestGetMe(t *testing.T) {
	db := testutil.NewStore(t)
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner, testutil.WithHandle("selfaware"))
	_, secret := testutil.BindAgent(t, db, agent)
	srv := testutil.NewServer(t, db)

	resp := srv.AsAgent(t, secret).Get("/v1/agent/me").ExpectStatus(http.StatusOK)

	var env meEnvelope
	resp.Decode(&env)
	if env.Agent.ID != domain.FormatID(domain.PrefixAgent, agent.ID) {
		t.Errorf("id = %q", env.Agent.ID)
	}
	if env.Agent.Handle != "selfaware" {
		t.Errorf("handle = %q", env.Agent.Handle)
	}
	if env.Agent.OwnerID != domain.FormatID(domain.PrefixUser, owner.ID) {
		t.Errorf("owner_id = %q", env.Agent.OwnerID)
	}
	if strings.Contains(string(resp.Body), secret) || strings.Contains(string(resp.Body), "secret") {
		t.Error("the response mentions the secret")
	}
}

// An agent's secret identifies exactly that agent. Presenting one agent's
// secret can never yield another agent's identity.
func TestGetMeIsBoundToTheSecret(t *testing.T) {
	db := testutil.NewStore(t)
	owner := testutil.CreateUser(t, db)
	a := testutil.CreateAgent(t, db, owner, testutil.WithHandle("agent-a"))
	b := testutil.CreateAgent(t, db, owner, testutil.WithHandle("agent-b"))
	_, secretA := testutil.BindAgent(t, db, a)
	_, secretB := testutil.BindAgent(t, db, b)
	srv := testutil.NewServer(t, db)

	var gotA, gotB meEnvelope
	srv.AsAgent(t, secretA).Get("/v1/agent/me").ExpectStatus(http.StatusOK).Decode(&gotA)
	srv.AsAgent(t, secretB).Get("/v1/agent/me").ExpectStatus(http.StatusOK).Decode(&gotB)

	if gotA.Agent.Handle != "agent-a" || gotB.Agent.Handle != "agent-b" {
		t.Errorf("secrets resolved to the wrong agents: %q, %q", gotA.Agent.Handle, gotB.Agent.Handle)
	}
}

func TestAgentAPIRejectsEverythingButABindingSecret(t *testing.T) {
	db := testutil.NewStore(t)
	owner := testutil.CreateUser(t, db)
	agent := testutil.CreateAgent(t, db, owner)
	srv := testutil.NewServer(t, db)

	cases := map[string]struct {
		client *testutil.Client
		code   string
	}{
		"nothing":           {srv.Anonymous(t), "missing_credentials"},
		"user header":       {srv.AsUser(t, owner), "missing_credentials"},
		"garbage bearer":    {srv.AsAgent(t, "bnd_sec_nope"), "invalid_credentials"},
		"basic scheme":      {srv.Anonymous(t).WithHeader("Authorization", "Basic abc"), "invalid_credentials"},
		"user id as bearer": {srv.AsAgent(t, domain.FormatID(domain.PrefixUser, owner.ID)), "invalid_credentials"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tc.client.Get("/v1/agent/me").ExpectError(http.StatusUnauthorized, tc.code)
		})
	}

	// And a secret that was valid stops being valid the moment the agent goes.
	_, secret := testutil.BindAgent(t, db, agent)
	srv.AsAgent(t, secret).Get("/v1/agent/me").ExpectStatus(http.StatusOK)
	srv.AsUser(t, owner).Delete("/v1/mgmt/agents/" + domain.FormatID(domain.PrefixAgent, agent.ID)).
		ExpectStatus(http.StatusNoContent)
	srv.AsAgent(t, secret).Get("/v1/agent/me").ExpectError(http.StatusUnauthorized, "invalid_credentials")
}

var _ = auth.DevHeader // keeps the import honest for the user-header case above
