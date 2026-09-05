package client_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type frameJSON struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func readFrame(t *testing.T, c *websocket.Conn) frameJSON {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var fr frameJSON
	if err := wsjson.Read(ctx, c, &fr); err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return fr
}

func expectSilence(t *testing.T, c *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var fr frameJSON
	if err := wsjson.Read(ctx, c, &fr); err == nil {
		t.Errorf("expected nothing, got %s: %s", fr.Type, fr.Data)
	}
}

// The socket says hello, then relays what happens in the person's chats and
// nothing else.
func TestSocketRelaysMessagesToParticipants(t *testing.T) {
	f := setupChat(t)
	owner := f.srv.Socket(t, f.owner)
	other := f.srv.Socket(t, f.other)

	ready := readFrame(t, owner)
	var hello struct {
		UserID string `json:"user_id"`
	}
	_ = json.Unmarshal(ready.Data, &hello)
	if ready.Type != "ready" || hello.UserID != domain.FormatID(domain.PrefixUser, f.owner.ID) {
		t.Fatalf("first frame = %s %s, want ready for the owner", ready.Type, ready.Data)
	}
	readFrame(t, other)

	sent := send(t, f.srv.AsUser(t, f.owner), f.dmID, "hello")

	fr := readFrame(t, owner)
	var created struct {
		ConversationID string      `json:"conversation_id"`
		Message        messageJSON `json:"message"`
	}
	if err := json.Unmarshal(fr.Data, &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fr.Type != "message.created" || created.ConversationID != f.dmID || created.Message.ID != sent.ID ||
		created.Message.Body.Text != "hello" || created.Message.DeliveryStatus == nil {
		t.Errorf("frame = %s %s, want message.created for the sent message", fr.Type, fr.Data)
	}
	expectSilence(t, other)
}

// The whole loop, live: the worker delivers, the tick moves on the socket;
// the agent replies, the reply appears on the socket.
func TestSocketRelaysDeliveryAndReplies(t *testing.T) {
	f := setupChat(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer backend.Close()
	_, secret := testutil.BindWebhook(t, f.db, f.agent, backend.URL)
	worker := delivery.NewWorker(f.db, delivery.New(f.db, f.srv.Conversations), agents.New(f.db), delivery.Options{
		AllowLoopback: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	sock := f.srv.Socket(t, f.owner)
	readFrame(t, sock) // ready

	sent := send(t, f.srv.AsUser(t, f.owner), f.dmID, "are you there?")
	if fr := readFrame(t, sock); fr.Type != "message.created" {
		t.Fatalf("first frame = %s", fr.Type)
	}

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	fr := readFrame(t, sock)
	var tick struct {
		MessageID      string `json:"message_id"`
		DeliveryStatus string `json:"delivery_status"`
	}
	_ = json.Unmarshal(fr.Data, &tick)
	if fr.Type != "delivery.updated" || tick.MessageID != sent.ID || tick.DeliveryStatus != "delivered" {
		t.Errorf("frame = %s %s, want delivery.updated delivered for the sent message", fr.Type, fr.Data)
	}

	var env struct {
		Message struct {
			ID string `json:"id"`
		} `json:"message"`
	}
	f.srv.AsAgent(t, secret).Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{"text": "yes"}).
		ExpectStatus(http.StatusCreated).Decode(&env)
	fr = readFrame(t, sock)
	var reply struct {
		Message messageJSON `json:"message"`
	}
	_ = json.Unmarshal(fr.Data, &reply)
	if fr.Type != "message.created" || reply.Message.ID != env.Message.ID || reply.Message.Sender.Kind != "agent" {
		t.Errorf("frame = %s %s, want the agent's reply", fr.Type, fr.Data)
	}
}

func TestSocketRequiresAUser(t *testing.T) {
	f := setupChat(t)
	if _, err := f.srv.DialSocket(t, http.Header{}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("anonymous socket: %v, want a 401 handshake failure", err)
	}
}

// After a disconnect the app fills its gap from the record.
func TestCatchUpAfterCursor(t *testing.T) {
	f := setupChat(t)
	c := f.srv.AsUser(t, f.owner)
	last := send(t, c, f.dmID, "seen")
	for _, text := range []string{"missed one", "missed two"} {
		send(t, c, f.dmID, text)
	}

	var page messagePageJSON
	c.Get("/v1/client/conversations/" + f.dmID + "/messages?after=" + last.ID).ExpectStatus(http.StatusOK).Decode(&page)
	if len(page.Messages) != 2 || page.Messages[0].Body.Text != "missed one" || page.Messages[1].Body.Text != "missed two" {
		t.Errorf("after = %+v, want the two missed messages oldest first", page.Messages)
	}
	if page.NextBefore != nil || page.NextAfter != nil {
		t.Errorf("cursors after a complete catch-up: before %v after %v", page.NextBefore, page.NextAfter)
	}
	c.Get("/v1/client/conversations/"+f.dmID+"/messages?after="+last.ID+"&before="+last.ID).
		ExpectError(http.StatusUnprocessableEntity, "invalid_cursor")
}
