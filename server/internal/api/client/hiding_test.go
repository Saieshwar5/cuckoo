package client_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// Over HTTP: what a person puts out of their own sight — one message, or
// everything so far — leaves their history and their chat-list preview,
// and nobody else's.
func TestClearAndDeleteForMe(t *testing.T) {
	f := setupChat(t)
	_, secret := testutil.BindAgent(t, f.db, f.agent)
	owner := f.srv.AsUser(t, f.owner)
	agent := f.srv.AsAgent(t, secret)
	messages := "/v1/client/conversations/" + f.dmID + "/messages"

	var ids []string
	for _, text := range []string{"one", "two", "three"} {
		var sent struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		}
		owner.Post(messages, map[string]any{"text": text}).ExpectStatus(http.StatusCreated).Decode(&sent)
		ids = append(ids, sent.Message.ID)
	}
	mine := func() []string {
		var page struct {
			Messages []messageJSON `json:"messages"`
		}
		owner.Get(messages).ExpectStatus(http.StatusOK).Decode(&page)
		out := make([]string, 0, len(page.Messages))
		for _, m := range page.Messages {
			out = append(out, m.Body.Text)
		}
		return out
	}
	preview := func() string {
		var list struct {
			Conversations []conversationJSON `json:"conversations"`
		}
		owner.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&list)
		for _, c := range list.Conversations {
			if c.ID == f.dmID {
				if c.LastMessage == nil {
					return ""
				}
				return c.LastMessage.Body.Text
			}
		}
		t.Fatalf("chat %s not listed", f.dmID)
		return ""
	}
	theirs := func() int {
		var page struct {
			Messages []messageJSON `json:"messages"`
		}
		agent.Get("/v1/agent/conversations/" + f.dmID + "/messages").ExpectStatus(http.StatusOK).Decode(&page)
		return len(page.Messages)
	}

	// Delete the middle one for me: gone from my history, the preview
	// unchanged; the agent still has all three.
	owner.Delete(messages + "/" + ids[1]).ExpectStatus(http.StatusNoContent)
	if got := mine(); len(got) != 2 || got[0] != "three" || got[1] != "one" {
		t.Errorf("after deleting two, history = %v", got)
	}
	if preview() != "three" || theirs() != 3 {
		t.Errorf("preview = %q, agent sees %d", preview(), theirs())
	}
	// Delete the newest: the preview steps back to what I can still see.
	owner.Delete(messages + "/" + ids[2]).ExpectStatus(http.StatusNoContent)
	if preview() != "one" {
		t.Errorf("preview after deleting the newest = %q, want one", preview())
	}
	// Twice is fine.
	owner.Delete(messages + "/" + ids[2]).ExpectStatus(http.StatusNoContent)

	// Clear: nothing left to see, no preview; the agent's history untouched.
	owner.Post("/v1/client/conversations/"+f.dmID+"/clear", nil).ExpectStatus(http.StatusNoContent)
	if got := mine(); len(got) != 0 {
		t.Errorf("after clearing, history = %v", got)
	}
	if preview() != "" || theirs() != 3 {
		t.Errorf("after clearing, preview = %q, agent sees %d", preview(), theirs())
	}
	// What is said next shows as usual, and is the preview.
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{"text": "still here"}).ExpectStatus(http.StatusCreated)
	if got := mine(); len(got) != 1 || got[0] != "still here" {
		t.Errorf("after clearing, a new message shows as %v", got)
	}
	if preview() != "still here" {
		t.Errorf("preview after a new message = %q", preview())
	}

	t.Run("only your own view", func(t *testing.T) {
		other := f.srv.AsUser(t, f.other)
		other.Post("/v1/client/conversations/"+f.dmID+"/clear", nil).ExpectStatus(http.StatusForbidden)
		other.Delete(messages + "/" + ids[0]).ExpectStatus(http.StatusForbidden)
		elsewhere := domain.FormatID(domain.PrefixConv, testutil.OwnerDM(t, f.db, testutil.CreateAgent(t, f.db, f.owner)).ID)
		owner.Delete("/v1/client/conversations/"+elsewhere+"/messages/"+ids[0]).ExpectError(http.StatusNotFound, "message_not_found")
		owner.Delete(messages + "/msg_nope").ExpectStatus(http.StatusUnprocessableEntity)
	})
}

// Over HTTP: a report goes on the pile once; the agent must be one the
// person has met; the reason is one of the fixed few.
func TestReportOverHTTP(t *testing.T) {
	f := setupChat(t)
	owner := f.srv.AsUser(t, f.owner)
	other := f.srv.AsUser(t, f.other)
	agentID := domain.FormatID(domain.PrefixAgent, f.agent.ID)

	var minted struct {
		Code string `json:"code"`
	}
	owner.Post("/v1/mgmt/agents/"+agentID+"/pair-tokens", map[string]any{}).ExpectStatus(http.StatusCreated).Decode(&minted)
	var accepted struct {
		Conversation conversationJSON `json:"conversation"`
	}
	other.Post("/v1/client/pair/"+minted.Code+"/accept", nil).ExpectStatus(http.StatusCreated).Decode(&accepted)
	var said struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	other.Post("/v1/client/conversations/"+accepted.Conversation.ID+"/messages", map[string]any{"text": "hm"}).
		ExpectStatus(http.StatusCreated).Decode(&said)

	report := "/v1/client/agents/" + agentID + "/report"
	other.Post(report, map[string]any{"reason": "loud"}).ExpectError(http.StatusUnprocessableEntity, "invalid_reason")
	other.Post(report, map[string]any{"reason": "spam", "message_id": "msg_nope"}).ExpectError(http.StatusUnprocessableEntity, "invalid_id")
	other.Post(report, map[string]any{"reason": "spam", "message_id": said.Message.ID, "note": "  keeps sending offers  "}).
		ExpectStatus(http.StatusCreated)
	other.Post(report, map[string]any{"reason": "abuse"}).ExpectError(http.StatusConflict, "already_reported")

	stranger := domain.FormatID(domain.PrefixAgent, testutil.CreateAgent(t, f.db, f.owner).ID)
	other.Post("/v1/client/agents/"+stranger+"/report", map[string]any{"reason": "spam"}).ExpectError(http.StatusNotFound, "not_a_contact")

	// A removed agent can still be reported: it is one the person met.
	other.Delete("/v1/client/contacts/" + agentID).ExpectStatus(http.StatusNoContent)
	f.srv.AsUser(t, f.other).Post(report, map[string]any{"reason": "other"}).ExpectError(http.StatusConflict, "already_reported")
}
