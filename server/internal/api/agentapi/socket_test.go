package agentapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/api/agentapi"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type envelopeJSON struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Message struct {
			ID   string `json:"id"`
			Body struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

func readEnvelope(t *testing.T, c *websocket.Conn) envelopeJSON {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var env envelopeJSON
	if err := wsjson.Read(ctx, c, &env); err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	return env
}

func ack(t *testing.T, c *websocket.Conn, eventID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, c, map[string]string{"ack": eventID}); err != nil {
		t.Fatalf("ack: %v", err)
	}
}

// waitFor polls until cond holds, for states the server reaches a moment
// after a frame is exchanged.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

type socketFixture struct {
	*chatFixture
	socketSecret string
	agentSvc     *agents.Service
}

func setupSocket(t *testing.T) *socketFixture {
	t.Helper()
	f := &socketFixture{chatFixture: setupChat(t), agentSvc: nil}
	f.agentSvc = agents.New(f.db)
	// setupChat bound the agent to a webhook; replace it with a socket
	// binding, which is what this file is about.
	_, f.socketSecret = testutil.BindAgent(t, f.db, f.agent)
	return f
}

func (f *socketFixture) bindingStatus(t *testing.T) agents.Status {
	t.Helper()
	b, err := f.agentSvc.ActiveBinding(context.Background(), f.owner.ID, f.agent.ID)
	if err != nil || b == nil {
		t.Fatalf("ActiveBinding: %v %v", b, err)
	}
	return b.Status
}

func (f *socketFixture) deliveryStatus(t *testing.T, messageID string) string {
	t.Helper()
	id, _ := domain.ParseID(domain.PrefixMessage, messageID)
	rows, err := f.db.ListDeliveriesByMessage(context.Background(), id)
	if err != nil || len(rows) != 1 {
		t.Fatalf("deliveries of %s: %v %v", messageID, rows, err)
	}
	return rows[0].Status
}

// The roadmap's own test: messages sent while the backend was away arrive in
// order on reconnect; acks turn the ticks; live messages follow.
func TestAgentSocketReplaysThenPushesLive(t *testing.T) {
	f := setupSocket(t)
	phone := f.srv.Socket(t, f.owner)
	f.userSends(t, "one")
	f.userSends(t, "two")
	readClientFrames(t, phone, 3) // ready, and the two sends

	c := f.srv.AgentSocket(t, f.socketSecret)
	first, second := readEnvelope(t, c), readEnvelope(t, c)
	if first.Data.Message.Body.Text != "one" || second.Data.Message.Body.Text != "two" {
		t.Fatalf("replay = %q, %q; want one then two", first.Data.Message.Body.Text, second.Data.Message.Body.Text)
	}
	if first.Type != "message.created" || !strings.HasPrefix(first.ID, "evt_") {
		t.Errorf("envelope = %+v", first)
	}
	waitFor(t, "binding connected", func() bool { return f.bindingStatus(t) == agents.StatusConnected })
	if f.deliveryStatus(t, first.Data.Message.ID) != "pending" {
		t.Error("a pushed but unacknowledged event counts as delivered")
	}

	ack(t, c, first.ID)
	ack(t, c, second.ID)
	waitFor(t, "acks recorded", func() bool { return f.deliveryStatus(t, second.Data.Message.ID) == "delivered" })
	if frames := readClientFrames(t, phone, 2); frames[0].Type != "delivery.updated" || frames[1].Type != "delivery.updated" {
		t.Errorf("the person's socket got %s, %s; want two tick updates", frames[0].Type, frames[1].Type)
	}

	f.userSends(t, "three")
	if live := readEnvelope(t, c); live.Data.Message.Body.Text != "three" {
		t.Errorf("live push = %q, want three", live.Data.Message.Body.Text)
	}

	_ = c.Close(websocket.StatusNormalClosure, "bye")
	waitFor(t, "binding idle after close", func() bool { return f.bindingStatus(t) == agents.StatusIdle })
}

type clientFrame struct {
	Type string `json:"type"`
}

// readClientFrames reads n frames about the conversation. Announcements
// about the agent's backend coming and going are not counted: they are the
// agent screens' business, and these tests are about the thread.
func readClientFrames(t *testing.T, c *websocket.Conn, n int) []clientFrame {
	t.Helper()
	out := make([]clientFrame, 0, n)
	for len(out) < n {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var fr clientFrame
		err := wsjson.Read(ctx, c, &fr)
		cancel()
		if err != nil {
			t.Fatalf("read client frame %d: %v", len(out)+1, err)
		}
		if fr.Type == "agent.status" {
			continue
		}
		out = append(out, fr)
	}
	return out
}

// An event that is never acknowledged is pushed again once its lease
// expires, ahead of anything newer.
func TestAgentSocketRepushesUnacknowledged(t *testing.T) {
	f := setupSocket(t)
	c := f.srv.AgentSocket(t, f.socketSecret)

	f.userSends(t, "hello")
	hello := readEnvelope(t, c)
	// Pushed once and leased: not delivered, not due again yet. Reading the
	// socket with a short deadline would close it, so the state is checked
	// in the database instead.
	id, _ := domain.ParseID(domain.PrefixEvent, hello.ID)
	if d, err := f.db.GetDelivery(context.Background(), id); err != nil || d.Status != "pending" || d.Attempts != 1 {
		t.Fatalf("after one push: %+v, %v; want pending with one attempt", d, err)
	}

	if _, err := f.tx.Exec(context.Background(),
		"UPDATE message_deliveries SET next_attempt_at = now() - interval '1 second' WHERE id = $1", id); err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	f.userSends(t, "again")

	got := []string{readEnvelope(t, c).Data.Message.Body.Text, readEnvelope(t, c).Data.Message.Body.Text}
	if got[0] != "hello" || got[1] != "again" {
		t.Errorf("after the lease expired got %v, want hello re-pushed before again", got)
	}
}

func TestAgentSocketAckIsScopedToTheAgent(t *testing.T) {
	f := setupSocket(t)
	c := f.srv.AgentSocket(t, f.socketSecret)
	f.userSends(t, "hello")
	env := readEnvelope(t, c)

	other := testutil.CreateAgent(t, f.db, f.owner)
	_, otherSecret := testutil.BindAgent(t, f.db, other)
	oc := f.srv.AgentSocket(t, otherSecret)
	ack(t, oc, env.ID)
	ack(t, oc, domain.FormatID(domain.PrefixEvent, domain.NewID()))
	ack(t, oc, "garbage")
	time.Sleep(200 * time.Millisecond)
	if f.deliveryStatus(t, env.Data.Message.ID) != "pending" {
		t.Error("another agent acknowledged this agent's event")
	}

	ack(t, c, env.ID)
	waitFor(t, "own ack", func() bool { return f.deliveryStatus(t, env.Data.Message.ID) == "delivered" })
}

// One socket per binding. The newer one wins, and the older one is told
// why, so an SDK knows to stop rather than fight.
func TestSecondSocketReplacesTheFirst(t *testing.T) {
	f := setupSocket(t)
	first := f.srv.AgentSocket(t, f.socketSecret)
	second := f.srv.AgentSocket(t, f.socketSecret)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := first.Read(ctx)
	if websocket.CloseStatus(err) != agentapi.CloseReplaced {
		t.Fatalf("first socket ended with %v, want close code %d", err, agentapi.CloseReplaced)
	}

	f.userSends(t, "to the survivor")
	if env := readEnvelope(t, second); env.Data.Message.Body.Text != "to the survivor" {
		t.Errorf("second socket got %q", env.Data.Message.Body.Text)
	}
	if f.bindingStatus(t) != agents.StatusConnected {
		t.Errorf("binding status = %q after replacement, want connected", f.bindingStatus(t))
	}
}

func TestAgentSocketRefusals(t *testing.T) {
	f := setupSocket(t)
	t.Run("webhook binding", func(t *testing.T) {
		webhookAgent := testutil.CreateAgent(t, f.db, f.owner)
		_, secret := testutil.BindWebhook(t, f.db, webhookAgent, "https://example.com/cuckoo")
		_, err := f.srv.DialSocket(t, "/v1/agent/socket", http.Header{"Authorization": {"Bearer " + secret}})
		if err == nil || !strings.Contains(err.Error(), "403") {
			t.Errorf("webhook-bound backend opened a socket: %v", err)
		}
	})
	t.Run("anonymous", func(t *testing.T) {
		_, err := f.srv.DialSocket(t, "/v1/agent/socket", http.Header{})
		if err == nil || !strings.Contains(err.Error(), "401") {
			t.Errorf("anonymous: %v", err)
		}
	})
	t.Run("a person", func(t *testing.T) {
		_, err := f.srv.DialSocket(t, "/v1/agent/socket",
			http.Header{"X-Dev-User": {domain.FormatID(domain.PrefixUser, f.owner.ID)}})
		if err == nil || !strings.Contains(err.Error(), "401") {
			t.Errorf("a person opened the agent socket: %v", err)
		}
	})
}
