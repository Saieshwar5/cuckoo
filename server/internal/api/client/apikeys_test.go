package client_test

import (
	"net/http"
	"strings"
	"testing"
)

type apiKeyJSON struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	LastUsedAt *string `json:"last_used_at"`
	RevokedAt  *string `json:"revoked_at"`
}

// mintKey issues a key for a person and returns its record and the key.
func mintKey(t *testing.T, f *chatFixture, name string) (apiKeyJSON, string) {
	t.Helper()
	var env struct {
		APIKey apiKeyJSON `json:"api_key"`
		Key    string     `json:"key"`
	}
	f.srv.AsUser(t, f.owner).Post("/v1/client/api-keys", map[string]string{"name": name}).
		ExpectStatus(http.StatusCreated).Decode(&env)
	return env.APIKey, env.Key
}

// The whole point: a server holding a key does everything an owner does in
// the management API, with no phone in the loop.
func TestAPIKeyDrivesTheManagementAPI(t *testing.T) {
	f := setupChat(t)
	record, key := mintKey(t, f, "SBI website")
	if !strings.HasPrefix(key, "mgt_tok_") || !strings.HasPrefix(record.ID, "key_") {
		t.Fatalf("minted %+v with key %q", record, key)
	}
	server := f.srv.AsKey(t, key)

	// Create an agent, connect its backend, and mint a personalised code:
	// the whole of a company's setup.
	var created struct {
		Agent struct {
			ID     string `json:"id"`
			Handle string `json:"handle"`
		} `json:"agent"`
	}
	server.Post("/v1/mgmt/agents", map[string]string{"handle": "sbi-cards", "display_name": "SBI Cards"}).
		ExpectStatus(http.StatusCreated).Decode(&created)

	var bound struct {
		Secret string `json:"secret"`
	}
	server.Post("/v1/mgmt/agents/"+created.Agent.ID+"/binding", map[string]string{"mode": "socket"}).
		ExpectStatus(http.StatusCreated).Decode(&bound)
	if !strings.HasPrefix(bound.Secret, "bnd_sec_") {
		t.Errorf("binding secret = %q", bound.Secret)
	}

	var minted struct {
		Code string `json:"code"`
		URL  string `json:"url"`
	}
	server.Post("/v1/mgmt/agents/"+created.Agent.ID+"/pair-tokens", map[string]any{
		"payload": map[string]string{"customer_ref": "SBI-8812"}, "max_uses": 1,
	}).ExpectStatus(http.StatusCreated).Decode(&minted)
	if !strings.Contains(minted.URL, minted.Code) {
		t.Errorf("minted %+v", minted)
	}

	// A person adds the agent with that code, and the agent is theirs.
	f.srv.AsUser(t, f.other).Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(http.StatusCreated)

	// The key sees the account's agents, the app's and its own alike.
	var list struct {
		Agents []struct {
			Handle string `json:"handle"`
		} `json:"agents"`
	}
	server.Get("/v1/mgmt/agents").ExpectStatus(http.StatusOK).Decode(&list)
	handles := map[string]bool{}
	for _, a := range list.Agents {
		handles[a.Handle] = true
	}
	if !handles["sbi-cards"] || !handles["sbi-support"] {
		t.Errorf("agents = %+v, want both the key's and the app's", list.Agents)
	}
}

// A key opens the management API and nothing else.
func TestAPIKeyIsRefusedElsewhere(t *testing.T) {
	f := setupChat(t)
	_, key := mintKey(t, f, "scoped")
	server := f.srv.AsKey(t, key)

	// Not the app's own API: it cannot read chats or become another key.
	server.Get("/v1/client/me").ExpectStatus(http.StatusUnauthorized)
	server.Get("/v1/client/conversations").ExpectStatus(http.StatusUnauthorized)
	server.Post("/v1/client/api-keys", map[string]string{"name": "another"}).ExpectStatus(http.StatusUnauthorized)
	server.Get("/v1/client/api-keys").ExpectStatus(http.StatusUnauthorized)
	// Nor the agent protocol: it cannot speak for a bot.
	server.Get("/v1/agent/events").ExpectStatus(http.StatusUnauthorized)
	// Nor the session it is not.
	server.Post("/v1/auth/logout", nil).ExpectStatus(http.StatusUnauthorized)
}

// Keys are listed and ended by their owner, in the app.
func TestAPIKeyListAndRevokeOverHTTP(t *testing.T) {
	f := setupChat(t)
	record, key := mintKey(t, f, "deploy script")

	var listed struct {
		APIKeys []apiKeyJSON `json:"api_keys"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/client/api-keys").ExpectStatus(http.StatusOK).Decode(&listed)
	if len(listed.APIKeys) != 1 || listed.APIKeys[0].Name != "deploy script" || listed.APIKeys[0].RevokedAt != nil {
		t.Fatalf("keys = %+v", listed.APIKeys)
	}
	// Somebody else's account has none of it.
	f.srv.AsUser(t, f.other).Get("/v1/client/api-keys").ExpectStatus(http.StatusOK).Decode(&listed)
	if len(listed.APIKeys) != 0 {
		t.Errorf("other sees %+v", listed.APIKeys)
	}
	f.srv.AsUser(t, f.other).Delete("/v1/client/api-keys/" + record.ID).ExpectStatus(http.StatusNotFound)

	f.srv.AsKey(t, key).Get("/v1/mgmt/agents").ExpectStatus(http.StatusOK)
	f.srv.AsUser(t, f.owner).Delete("/v1/client/api-keys/" + record.ID).ExpectStatus(http.StatusNoContent)
	f.srv.AsKey(t, key).Get("/v1/mgmt/agents").ExpectError(http.StatusUnauthorized, "invalid_credentials")
	f.srv.AsUser(t, f.owner).Delete("/v1/client/api-keys/" + record.ID).ExpectStatus(http.StatusNotFound)

	// An unnamed key is refused with the field named.
	r := f.srv.AsUser(t, f.owner).Post("/v1/client/api-keys", map[string]string{"name": " "}).
		ExpectError(http.StatusUnprocessableEntity, "invalid_name")
	if r.ErrorField() != "name" {
		t.Errorf("field = %q, want name", r.ErrorField())
	}
	// The app still works after all that.
	f.srv.AsUser(t, f.owner).Get("/v1/mgmt/agents").ExpectStatus(http.StatusOK)
}

// A key belongs to the account, not to a device. Signing in elsewhere ends
// the person's session — one device at a time — and the key carries on,
// which is the whole reason a server holds one.
func TestAPIKeyOutlivesSessions(t *testing.T) {
	f := setupChat(t)
	_, key := mintKey(t, f, "outlives sessions")

	email := "keys@example.com"
	first := f.srv.SignIn(t, email)
	f.srv.AsSession(t, first).Get("/v1/client/me").ExpectStatus(http.StatusOK)
	second := f.srv.SignIn(t, email)
	f.srv.AsSession(t, first).Get("/v1/client/me").ExpectStatus(http.StatusUnauthorized)
	f.srv.AsSession(t, second).Get("/v1/client/me").ExpectStatus(http.StatusOK)

	f.srv.AsKey(t, key).Get("/v1/mgmt/agents").ExpectStatus(http.StatusOK)
}
