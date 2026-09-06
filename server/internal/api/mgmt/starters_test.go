package mgmt_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// Starters ride on the agent from the management API to the card and to
// the participant in a chat, and are checked like button labels.
func TestStarters(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db)
	owner := testutil.CreateUser(t, db)
	me := srv.AsUser(t, owner)

	var created struct {
		Agent struct {
			ID       string   `json:"id"`
			Starters []string `json:"starters"`
		} `json:"agent"`
	}
	me.Post("/v1/mgmt/agents", map[string]any{
		"handle": "swiggy", "display_name": "Swiggy",
		"starters": []string{" Track my order ", "Refund status"},
	}).ExpectStatus(http.StatusCreated).Decode(&created)
	if len(created.Agent.Starters) != 2 || created.Agent.Starters[0] != "Track my order" {
		t.Fatalf("starters = %v, want two, trimmed", created.Agent.Starters)
	}

	// The person's chat carries them on the agent's participant entry.
	var list struct {
		Conversations []struct {
			Participants []struct {
				Kind     string   `json:"kind"`
				Starters []string `json:"starters"`
			} `json:"participants"`
		} `json:"conversations"`
	}
	me.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&list)
	found := false
	for _, c := range list.Conversations {
		for _, p := range c.Participants {
			if p.Kind == "agent" && len(p.Starters) == 2 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("the chat's participant did not carry the starters: %+v", list)
	}

	// Replace, then clear.
	path := "/v1/mgmt/agents/" + created.Agent.ID
	me.Patch(path, map[string]any{"starters": []string{"Hi"}}).ExpectStatus(http.StatusOK).Decode(&created)
	if len(created.Agent.Starters) != 1 || created.Agent.Starters[0] != "Hi" {
		t.Errorf("after replace = %v", created.Agent.Starters)
	}
	me.Patch(path, map[string]any{"display_name": "Swiggy Food"}).ExpectStatus(http.StatusOK).Decode(&created)
	if len(created.Agent.Starters) != 1 {
		t.Errorf("an update without starters changed them: %v", created.Agent.Starters)
	}
	me.Patch(path, map[string]any{"starters": []string{}}).ExpectStatus(http.StatusOK).Decode(&created)
	if len(created.Agent.Starters) != 0 {
		t.Errorf("after clearing = %v", created.Agent.Starters)
	}

	for name, bad := range map[string][]string{
		"five":       {"a", "b", "c", "d", "e"},
		"empty one":  {"a", "  "},
		"too long":   {"0123456789012345678901234567890123456789x"},
		"line break": {"a\nb"},
		"repeat":     {"a", "a"},
	} {
		me.Patch(path, map[string]any{"starters": bad}).ExpectError(http.StatusUnprocessableEntity, "invalid_starters")
		_ = name
	}
}

// A hub with a welcome agent gives it to every new account, and only to
// new ones; the agent hears that they joined.
func TestWelcomeAgent(t *testing.T) {
	db := testutil.NewStore(t)
	srv := testutil.NewServer(t, db, testutil.WithWelcomeHandle("welcome"))
	operator := testutil.CreateUser(t, db)
	greeter := testutil.CreateAgent(t, db, operator, testutil.WithHandle("welcome"), testutil.WithAgentName("Cuckoo"))
	_, secret := testutil.BindAgent(t, db, greeter)

	// A stranger signs up by email code.
	anon := srv.Anonymous(t)
	anon.Post("/v1/auth/email/start", map[string]any{"email": "new@example.com"}).ExpectStatus(http.StatusNoContent)
	var verified struct {
		Token string `json:"token"`
		IsNew bool   `json:"is_new"`
	}
	anon.Post("/v1/auth/email/verify", map[string]any{
		"email": "new@example.com", "code": srv.LastCode(t, "new@example.com"), "device_name": "test",
	}).ExpectStatus(http.StatusOK).Decode(&verified)
	if !verified.IsNew {
		t.Fatalf("expected a new account")
	}

	var list struct {
		Contacts []struct {
			Agent struct {
				ID string `json:"id"`
			} `json:"agent"`
			AddedVia string `json:"added_via"`
		} `json:"contacts"`
	}
	srv.AsSession(t, verified.Token).Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&list)
	if len(list.Contacts) != 1 || list.Contacts[0].Agent.ID != domain.FormatID(domain.PrefixAgent, greeter.ID) || list.Contacts[0].AddedVia != "hub" {
		t.Fatalf("a new account's list = %+v, want the greeter, added by the hub", list.Contacts)
	}

	var events struct {
		Events []struct {
			Type string `json:"type"`
		} `json:"events"`
	}
	srv.AsAgent(t, secret).Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&events)
	joined := 0
	for _, e := range events.Events {
		if e.Type == "conversation.joined" {
			joined++
		}
	}
	if joined != 1 {
		t.Errorf("the greeter heard %d joins, want 1", joined)
	}

	// Signing in again is not arriving again.
	anon.Post("/v1/auth/email/start", map[string]any{"email": "new@example.com"}).ExpectStatus(http.StatusNoContent)
	anon.Post("/v1/auth/email/verify", map[string]any{
		"email": "new@example.com", "code": srv.LastCode(t, "new@example.com"), "device_name": "test",
	}).ExpectStatus(http.StatusOK).Decode(&verified)
	srv.AsSession(t, verified.Token).Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&list)
	if len(list.Contacts) != 1 {
		t.Errorf("after a second sign-in the list has %d contacts", len(list.Contacts))
	}
}
