package mgmt_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

type bindingEnvelope struct {
	Binding struct {
		ID     string  `json:"id"`
		Mode   string  `json:"mode"`
		Status string  `json:"status"`
		URL    *string `json:"webhook_url"`
	} `json:"binding"`
	Secret string `json:"secret"`
}

func TestSetBindingReturnsSecretOnce(t *testing.T) {
	srv, owner, _ := setup(t)
	c := srv.AsUser(t, owner)
	a := create(t, c, "bindme")

	var env bindingEnvelope
	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{"mode": "socket"}).
		ExpectStatus(http.StatusCreated).Decode(&env)

	if !strings.HasPrefix(env.Secret, "bnd_sec_") {
		t.Errorf("secret = %q", env.Secret)
	}
	if !strings.HasPrefix(env.Binding.ID, "bnd_") || env.Binding.Mode != "socket" || env.Binding.Status != "idle" {
		t.Errorf("binding = %+v", env.Binding)
	}

	// Reading the agent back shows the binding but never the secret.
	resp := c.Get("/v1/mgmt/agents/" + a.ID).ExpectStatus(http.StatusOK)
	var raw map[string]any
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(resp.Body), env.Secret) {
		t.Fatal("the secret appears in a GET response")
	}
	if strings.Contains(string(resp.Body), `"secret"`) {
		t.Fatal("a secret field appears in a GET response")
	}
	var got agentEnvelope
	resp.Decode(&got)
	if got.Agent.Binding == nil || got.Agent.Binding.ID != env.Binding.ID {
		t.Errorf("GET did not show the binding: %+v", got.Agent.Binding)
	}
}

// The secret works on the agent API — that is its only purpose — and stops
// working the instant a new binding replaces it.
func TestBindingSecretLifecycle(t *testing.T) {
	srv, owner, _ := setup(t)
	c := srv.AsUser(t, owner)
	a := create(t, c, "lifecycle")

	var first bindingEnvelope
	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{"mode": "socket"}).
		ExpectStatus(http.StatusCreated).Decode(&first)
	srv.AsAgent(t, first.Secret).Get("/v1/agent/me").ExpectStatus(http.StatusOK)

	var second bindingEnvelope
	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{"mode": "socket"}).
		ExpectStatus(http.StatusCreated).Decode(&second)
	if second.Secret == first.Secret {
		t.Fatal("re-binding reused the secret")
	}

	srv.AsAgent(t, first.Secret).Get("/v1/agent/me").ExpectError(http.StatusUnauthorized, "invalid_credentials")
	srv.AsAgent(t, second.Secret).Get("/v1/agent/me").ExpectStatus(http.StatusOK)

	c.Delete("/v1/mgmt/agents/" + a.ID + "/binding").ExpectStatus(http.StatusNoContent)
	srv.AsAgent(t, second.Secret).Get("/v1/agent/me").ExpectError(http.StatusUnauthorized, "invalid_credentials")

	var after agentEnvelope
	c.Get("/v1/mgmt/agents/" + a.ID).ExpectStatus(http.StatusOK).Decode(&after)
	if after.Agent.Binding != nil {
		t.Errorf("binding still shown after revoke: %+v", after.Agent.Binding)
	}
	c.Delete("/v1/mgmt/agents/"+a.ID+"/binding").ExpectError(http.StatusNotFound, "no_binding")
}

func TestSetBindingWebhook(t *testing.T) {
	srv, owner, _ := setup(t)
	c := srv.AsUser(t, owner)
	a := create(t, c, "hooked")

	var env bindingEnvelope
	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{
		"mode": "webhook", "webhook_url": "https://example.com/cuckoo",
	}).ExpectStatus(http.StatusCreated).Decode(&env)
	if env.Binding.URL == nil || *env.Binding.URL != "https://example.com/cuckoo" {
		t.Errorf("webhook_url = %v", env.Binding.URL)
	}

	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{
		"mode": "webhook", "webhook_url": "http://example.com/cuckoo",
	}).ExpectError(http.StatusUnprocessableEntity, "invalid_webhook_url")
	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{"mode": "webhook"}).
		ExpectError(http.StatusUnprocessableEntity, "invalid_webhook_url")
	c.Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{"mode": "telepathy"}).
		ExpectError(http.StatusUnprocessableEntity, "invalid_mode")
}

func TestBindingRequiresOwner(t *testing.T) {
	srv, owner, other := setup(t)
	a := create(t, srv.AsUser(t, owner), "guarded")

	srv.AsUser(t, other).Post("/v1/mgmt/agents/"+a.ID+"/binding", map[string]any{"mode": "socket"}).
		ExpectError(http.StatusForbidden, "not_owner")
	srv.AsUser(t, other).Delete("/v1/mgmt/agents/"+a.ID+"/binding").
		ExpectError(http.StatusForbidden, "not_owner")
}
