package client_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// Closing an account: the token dies with it, the agents are retired for
// the people who added them, the name in their history becomes "Deleted
// account", and the same address can start again as someone new.
func TestDeleteAccount(t *testing.T) {
	f := setupChat(t)
	agentID := domain.FormatID(domain.PrefixAgent, f.agent.ID)
	owner := f.srv.AsUser(t, f.owner)
	other := f.srv.AsUser(t, f.other)

	// The other person adds the owner's agent and hears from the owner-side
	// history through it.
	var minted struct {
		Code string `json:"code"`
	}
	owner.Post("/v1/mgmt/agents/"+agentID+"/pair-tokens", map[string]any{}).ExpectStatus(http.StatusCreated).Decode(&minted)
	other.Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(http.StatusCreated)

	// The owner signs in by code, so there is a real session to lose.
	anon := f.srv.Anonymous(t)
	anon.Post("/v1/auth/email/start", map[string]any{"email": "owner@example.com"}).ExpectStatus(http.StatusNoContent)
	var v struct {
		Token string `json:"token"`
		IsNew bool   `json:"is_new"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	anon.Post("/v1/auth/email/verify", map[string]any{
		"email": "owner@example.com", "code": f.srv.LastCode(t, "owner@example.com"), "device_name": "t",
	}).ExpectStatus(http.StatusOK).Decode(&v)
	me := f.srv.AsSession(t, v.Token)
	firstID := v.User.ID
	me.Post("/v1/mgmt/agents", map[string]any{"handle": "mine-to-lose", "display_name": "Mine"}).ExpectStatus(http.StatusCreated)

	me.Delete("/v1/client/me").ExpectStatus(http.StatusNoContent)
	me.Get("/v1/client/me").ExpectStatus(http.StatusUnauthorized)

	// The same address opens a fresh account, with nothing in it.
	anon.Post("/v1/auth/email/start", map[string]any{"email": "owner@example.com"}).ExpectStatus(http.StatusNoContent)
	anon.Post("/v1/auth/email/verify", map[string]any{
		"email": "owner@example.com", "code": f.srv.LastCode(t, "owner@example.com"), "device_name": "t",
	}).ExpectStatus(http.StatusOK).Decode(&v)
	if !v.IsNew || v.User.ID == firstID {
		t.Fatalf("after deletion the address should be new: is_new=%v same id=%v", v.IsNew, v.User.ID == firstID)
	}
	var agents struct {
		Agents []any `json:"agents"`
	}
	f.srv.AsSession(t, v.Token).Get("/v1/mgmt/agents").ExpectStatus(http.StatusOK).Decode(&agents)
	if len(agents.Agents) != 0 {
		t.Errorf("a fresh account owns %d agents", len(agents.Agents))
	}
	// And the handle stays retired.
	f.srv.AsSession(t, v.Token).Post("/v1/mgmt/agents", map[string]any{"handle": "mine-to-lose", "display_name": "Again"}).
		ExpectError(http.StatusConflict, "handle_taken")

	// Now the original owner's own account, whose agent the other person has.
	owner.Delete("/v1/client/me").ExpectStatus(http.StatusNoContent)
	var contacts struct {
		Contacts []struct {
			Agent struct {
				ID string `json:"id"`
			} `json:"agent"`
			AgentDeleted bool `json:"agent_deleted"`
		} `json:"contacts"`
	}
	other.Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&contacts)
	for _, c := range contacts.Contacts {
		if c.Agent.ID == agentID && !c.AgentDeleted {
			t.Errorf("the deleted owner's agent still reads as live to the person who added it")
		}
	}
	// The owner's own chat with it was never the other person's to open.
	other.Get("/v1/client/conversations/" + f.dmID).ExpectStatus(http.StatusForbidden)
	var list struct {
		Conversations []conversationJSON `json:"conversations"`
	}
	other.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&list)
	for _, c := range list.Conversations {
		for _, p := range c.Participants {
			if p.Kind == "user" && strings.Contains(p.DisplayName, "Priya") {
				t.Errorf("a deleted person's name still shows: %+v", p)
			}
		}
	}
}
