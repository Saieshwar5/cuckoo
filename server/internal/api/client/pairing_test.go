package client_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

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
	f.srv.AsUser(t, f.owner).Delete("/v1/mgmt/agents/" + agentID + "/pair-tokens/" + minted.Token.ID).ExpectStatus(http.StatusNoContent)
	f.srv.AsUser(t, f.other).Get("/v1/client/pair/" + minted.Code).ExpectStatus(http.StatusNotFound)
}

// Blocking closes the chat both ways over HTTP and the backend is told.
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
