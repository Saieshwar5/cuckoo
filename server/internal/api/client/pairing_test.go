package client_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type pairJSON struct {
	Kind  string `json:"kind"`
	Agent struct {
		ID          string `json:"id"`
		Handle      string `json:"handle"`
		DisplayName string `json:"display_name"`
		Owner       struct {
			DisplayName string `json:"display_name"`
		} `json:"owner"`
		Verified bool `json:"verified"`
	} `json:"agent"`
	AlreadyAdded   bool    `json:"already_added"`
	Blocked        bool    `json:"blocked"`
	ConversationID *string `json:"conversation_id"`
}

// A stranger scans the owner's code, sees the card, adds the agent, and can
// talk to it; the owner's list of codes shows the use.
func TestPairOverHTTP(t *testing.T) {
	f := setupChat(t)
	_, secret := testutil.BindAgent(t, f.db, f.agent)
	agentID := domain.FormatID(domain.PrefixAgent, f.agent.ID)

	var minted struct {
		Token struct {
			ID      string `json:"id"`
			MaxUses *int   `json:"max_uses"`
		} `json:"token"`
		Code  string `json:"code"`
		URL   string `json:"url"`
		QRPNG string `json:"qr_png"`
	}
	f.srv.AsUser(t, f.owner).Post("/v1/mgmt/agents/"+agentID+"/pair-tokens", map[string]any{
		"payload": map[string]string{"customer_ref": "SBI-8812"}, "max_uses": 1, "expires_in": 3600,
	}).ExpectStatus(http.StatusCreated).Decode(&minted)
	if !strings.HasPrefix(minted.URL, "https://hub.test/p/pair_") || !strings.HasPrefix(minted.QRPNG, "data:image/png;base64,") {
		t.Errorf("minted = %+v", minted)
	}
	// The link page works without an account.
	page := f.srv.Anonymous(t).Get("/p/" + minted.Code).ExpectStatus(http.StatusOK)
	if body := page.Text(); !strings.Contains(body, "SBI Support") || !strings.Contains(body, "cuckoo://p/"+minted.Code) {
		t.Errorf("page = %s", body)
	}
	f.srv.Anonymous(t).Get("/p/pair_nope").ExpectStatus(http.StatusNotFound)

	var card pairJSON
	f.srv.AsUser(t, f.other).Get("/v1/client/pair/" + minted.Code).ExpectStatus(http.StatusOK).Decode(&card)
	if card.Kind != "add_agent" || card.Agent.ID != agentID || card.Agent.Owner.DisplayName != "Priya" || card.AlreadyAdded || card.Agent.Verified {
		t.Errorf("card = %+v", card)
	}

	var accepted struct {
		Conversation conversationJSON `json:"conversation"`
		New          bool             `json:"new"`
	}
	f.srv.AsUser(t, f.other).Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(http.StatusCreated).Decode(&accepted)
	if !accepted.New || accepted.Conversation.Kind != "dm" || len(accepted.Conversation.Participants) != 2 {
		t.Errorf("accepted = %+v", accepted)
	}
	f.srv.AsUser(t, f.other).Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(http.StatusOK)

	// The backend sees the join first, then the message.
	send(t, f.srv.AsUser(t, f.other), accepted.Conversation.ID, "my payment failed")
	var evs struct {
		Events []struct {
			Type string `json:"type"`
			Data struct {
				PairToken *struct {
					ID      string          `json:"id"`
					Payload json.RawMessage `json:"payload"`
				} `json:"pair_token"`
			} `json:"data"`
		} `json:"events"`
	}
	f.srv.AsAgent(t, secret).Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&evs)
	if len(evs.Events) != 2 || evs.Events[0].Type != "conversation.joined" || evs.Events[1].Type != "message.created" {
		t.Fatalf("events = %+v", evs.Events)
	}
	if pt := evs.Events[0].Data.PairToken; pt == nil || pt.ID != minted.Token.ID || string(pt.Payload) != `{"customer_ref":"SBI-8812"}` {
		t.Errorf("join payload = %+v", evs.Events[0].Data.PairToken)
	}

	var contacts struct {
		Contacts []struct {
			Agent struct {
				ID string `json:"id"`
			} `json:"agent"`
			AddedVia       string `json:"added_via"`
			ConversationID string `json:"conversation_id"`
		} `json:"contacts"`
	}
	f.srv.AsUser(t, f.other).Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&contacts)
	if len(contacts.Contacts) != 1 || contacts.Contacts[0].AddedVia != "pair_token" || contacts.Contacts[0].ConversationID != accepted.Conversation.ID {
		t.Errorf("contacts = %+v", contacts.Contacts)
	}
	// The owner is a contact of their own agent, added as the owner.
	f.srv.AsUser(t, f.owner).Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&contacts)
	if len(contacts.Contacts) != 1 || contacts.Contacts[0].AddedVia != "owner" {
		t.Errorf("owner's contacts = %+v", contacts.Contacts)
	}

	var tokens struct {
		Tokens []struct {
			ID       string `json:"id"`
			UseCount int    `json:"use_count"`
		} `json:"tokens"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/mgmt/agents/" + agentID + "/pair-tokens").ExpectStatus(http.StatusOK).Decode(&tokens)
	if len(tokens.Tokens) != 1 || tokens.Tokens[0].UseCount != 1 {
		t.Errorf("tokens = %+v", tokens.Tokens)
	}
	f.srv.AsUser(t, f.other).Get("/v1/mgmt/agents/" + agentID + "/pair-tokens").ExpectStatus(http.StatusForbidden)
	// The picture again, for the code this device kept; never for a wrong code.
	var pic struct {
		URL   string `json:"url"`
		QRPNG string `json:"qr_png"`
	}
	f.srv.AsUser(t, f.owner).Get("/v1/mgmt/agents/" + agentID + "/pair-tokens/" + minted.Token.ID + "/qr?code=" + minted.Code).
		ExpectStatus(http.StatusOK).Decode(&pic)
	if pic.URL != minted.URL || !strings.HasPrefix(pic.QRPNG, "data:image/png;base64,") {
		t.Errorf("picture = %+v", pic)
	}
	f.srv.AsUser(t, f.owner).Get("/v1/mgmt/agents/" + agentID + "/pair-tokens/" + minted.Token.ID + "/qr?code=pair_wrong").ExpectStatus(http.StatusNotFound)
	f.srv.AsUser(t, f.owner).Delete("/v1/mgmt/agents/" + agentID + "/pair-tokens/" + minted.Token.ID).ExpectStatus(http.StatusNoContent)
	f.srv.AsUser(t, f.other).Get("/v1/client/pair/" + minted.Code).ExpectStatus(http.StatusNotFound)
}

// Blocking closes the chat both ways over HTTP and the backend is told.
// contactSettingsJSON is the part of a contact row this test reads.
type contactSettingsJSON struct {
	Agent struct {
		ID string `json:"id"`
	} `json:"agent"`
	ConversationID string  `json:"conversation_id"`
	MutedUntil     *string `json:"muted_until"`
	Pinned         bool    `json:"pinned"`
	Archived       bool    `json:"archived"`
}

// Over HTTP: what a person decides about an agent in their list — mute,
// pin, archive, remove — is theirs alone, survives in the list, is capped
// where a cap makes sense, and is wiped when the agent is removed and comes
// back.
func TestContactSettingsOverHTTP(t *testing.T) {
	f := setupChat(t)
	other := f.srv.AsUser(t, f.other)
	owner := f.srv.AsUser(t, f.owner)

	// add hands f.other an agent of f.owner's by code and returns its id.
	// 201 the first time; 200 when the contact already existed, which is
	// what coming back after a removal looks like.
	add := func(agent agents.Agent, status int) string {
		agentID := domain.FormatID(domain.PrefixAgent, agent.ID)
		var minted struct {
			Code string `json:"code"`
		}
		owner.Post("/v1/mgmt/agents/"+agentID+"/pair-tokens", map[string]any{}).ExpectStatus(http.StatusCreated).Decode(&minted)
		other.Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(status)
		return agentID
	}
	contact := func(agentID string) contactSettingsJSON {
		var list struct {
			Contacts []contactSettingsJSON `json:"contacts"`
		}
		other.Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&list)
		for _, c := range list.Contacts {
			if c.Agent.ID == agentID {
				return c
			}
		}
		t.Fatalf("%s is not in the list", agentID)
		return contactSettingsJSON{}
	}
	listed := func(conversationID string) bool {
		var list struct {
			Conversations []conversationJSON `json:"conversations"`
		}
		other.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&list)
		for _, c := range list.Conversations {
			if c.ID == conversationID {
				return true
			}
		}
		return false
	}
	patch := func(agentID string, body map[string]any) *testutil.Response {
		return other.Patch("/v1/client/contacts/"+agentID, body)
	}

	agentID := add(f.agent, http.StatusCreated)
	if c := contact(agentID); c.MutedUntil != nil || c.Pinned || c.Archived {
		t.Fatalf("fresh contact = %+v, want nothing decided", c)
	}

	t.Run("mute", func(t *testing.T) {
		until := time.Now().Add(8 * time.Hour).UTC().Format(time.RFC3339)
		patch(agentID, map[string]any{"muted_until": until}).ExpectStatus(http.StatusNoContent)
		if c := contact(agentID); c.MutedUntil == nil {
			t.Errorf("muted_until not recorded")
		}
		patch(agentID, map[string]any{"muted_until": nil}).ExpectStatus(http.StatusNoContent)
		if c := contact(agentID); c.MutedUntil != nil {
			t.Errorf("null did not unmute: %v", *c.MutedUntil)
		}
		past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		patch(agentID, map[string]any{"muted_until": past}).ExpectError(http.StatusUnprocessableEntity, "invalid_muted_until")
		patch(agentID, map[string]any{"muted_until": "soon"}).ExpectError(http.StatusUnprocessableEntity, "invalid_muted_until")
	})

	t.Run("pin, ten at most", func(t *testing.T) {
		var others []string
		for range 10 {
			others = append(others, add(testutil.CreateAgent(t, f.db, f.owner), http.StatusCreated))
		}
		for _, id := range others {
			patch(id, map[string]any{"pinned": true}).ExpectStatus(http.StatusNoContent)
		}
		patch(agentID, map[string]any{"pinned": true}).ExpectError(http.StatusConflict, "too_many_pins")
		// Pinning a pinned chat again is not an eleventh pin.
		patch(others[0], map[string]any{"pinned": true}).ExpectStatus(http.StatusNoContent)
		patch(others[0], map[string]any{"pinned": false}).ExpectStatus(http.StatusNoContent)
		patch(agentID, map[string]any{"pinned": true}).ExpectStatus(http.StatusNoContent)
		if !contact(agentID).Pinned || contact(others[0]).Pinned {
			t.Errorf("pins did not move")
		}
	})

	t.Run("archive", func(t *testing.T) {
		patch(agentID, map[string]any{"archived": true}).ExpectStatus(http.StatusNoContent)
		if !contact(agentID).Archived {
			t.Errorf("not archived")
		}
	})

	t.Run("remove, and come back clean", func(t *testing.T) {
		conv := contact(agentID).ConversationID
		other.Delete("/v1/client/contacts/" + agentID).ExpectStatus(http.StatusNoContent)
		var list struct {
			Contacts []contactSettingsJSON `json:"contacts"`
		}
		other.Get("/v1/client/contacts").ExpectStatus(http.StatusOK).Decode(&list)
		for _, c := range list.Contacts {
			if c.Agent.ID == agentID {
				t.Fatalf("removed agent still listed")
			}
		}
		if listed(conv) {
			t.Errorf("removed agent's chat still in the list")
		}
		// The history is still theirs.
		other.Get("/v1/client/conversations/" + conv + "/messages").ExpectStatus(http.StatusOK)
		// Nothing can be decided about an agent that is not in the list.
		patch(agentID, map[string]any{"pinned": true}).ExpectError(http.StatusNotFound, "not_a_contact")
		other.Delete("/v1/client/contacts/"+agentID).ExpectError(http.StatusNotFound, "not_a_contact")

		// The card a fresh code opens offers Add again, not "already added".
		var minted struct {
			Code string `json:"code"`
		}
		owner.Post("/v1/mgmt/agents/"+agentID+"/pair-tokens", map[string]any{}).ExpectStatus(http.StatusCreated).Decode(&minted)
		var card struct {
			AlreadyAdded bool `json:"already_added"`
		}
		other.Get("/v1/client/pair/" + minted.Code).ExpectStatus(http.StatusOK).Decode(&card)
		if card.AlreadyAdded {
			t.Errorf("a removed agent's card says already added")
		}

		// Scanning again restores it, with nothing decided.
		add(f.agent, http.StatusOK)
		other.Get("/v1/client/pair/" + minted.Code).ExpectStatus(http.StatusOK).Decode(&card)
		if !card.AlreadyAdded {
			t.Errorf("after coming back the card should say already added")
		}
		c := contact(agentID)
		if c.Pinned || c.Archived || c.MutedUntil != nil || c.ConversationID != conv {
			t.Errorf("restored contact = %+v, want the same chat and nothing decided", c)
		}
		if !listed(conv) {
			t.Errorf("restored agent's chat not back in the list")
		}
	})

	t.Run("an owner deletes, not removes", func(t *testing.T) {
		mine := domain.FormatID(domain.PrefixAgent, f.agent.ID)
		owner.Delete("/v1/client/contacts/"+mine).ExpectError(http.StatusConflict, "own_agent")
		// But may pin and mute their own.
		owner.Patch("/v1/client/contacts/"+mine, map[string]any{"pinned": true}).ExpectStatus(http.StatusNoContent)
	})

	stranger := domain.FormatID(domain.PrefixAgent, testutil.CreateAgent(t, f.db, f.owner).ID)
	patch(stranger, map[string]any{"archived": true}).ExpectError(http.StatusNotFound, "not_a_contact")
}

func TestBlockOverHTTP(t *testing.T) {
	f := setupChat(t)
	_, secret := testutil.BindAgent(t, f.db, f.agent)
	agentID := domain.FormatID(domain.PrefixAgent, f.agent.ID)
	var minted struct {
		Code string `json:"code"`
	}
	f.srv.AsUser(t, f.owner).Post("/v1/mgmt/agents/"+agentID+"/pair-tokens", map[string]any{}).ExpectStatus(http.StatusCreated).Decode(&minted)
	var accepted struct {
		Conversation conversationJSON `json:"conversation"`
	}
	f.srv.AsUser(t, f.other).Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(http.StatusCreated).Decode(&accepted)

	f.srv.AsUser(t, f.other).Post("/v1/client/agents/"+agentID+"/block", nil).ExpectStatus(http.StatusNoContent)
	f.srv.AsAgent(t, secret).Post("/v1/agent/conversations/"+accepted.Conversation.ID+"/messages",
		map[string]string{"text": "still there?"}).ExpectStatus(http.StatusForbidden)
	f.srv.AsUser(t, f.other).Post("/v1/client/conversations/"+accepted.Conversation.ID+"/messages",
		map[string]string{"text": "no"}).ExpectStatus(http.StatusForbidden)
	// History stays readable.
	f.srv.AsUser(t, f.other).Get("/v1/client/conversations/" + accepted.Conversation.ID + "/messages").ExpectStatus(http.StatusOK)

	f.srv.AsUser(t, f.other).Delete("/v1/client/agents/" + agentID + "/block").ExpectStatus(http.StatusNoContent)
	f.srv.AsAgent(t, secret).Post("/v1/agent/conversations/"+accepted.Conversation.ID+"/messages",
		map[string]string{"text": "welcome back"}).ExpectStatus(http.StatusCreated)
	stranger := domain.FormatID(domain.PrefixAgent, testutil.CreateAgent(t, f.db, f.owner).ID)
	f.srv.AsUser(t, f.other).Post("/v1/client/agents/"+stranger+"/block", nil).ExpectStatus(http.StatusNotFound)
}
