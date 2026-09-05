package agentapi_test

import (
	"context"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type replyJSON struct {
	ReplyToCID string `json:"reply_to_cid"`
	OK         bool   `json:"ok"`
	MessageID  string `json:"message_id"`
	Message    *struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Body   struct {
			Text string `json:"text"`
		} `json:"body"`
		Truncated bool `json:"truncated"`
	} `json:"message"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func send(t *testing.T, c *websocket.Conn, frame map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, c, frame); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readReply(t *testing.T, c *websocket.Conn) replyJSON {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var r replyJSON
	if err := wsjson.Read(ctx, c, &r); err != nil {
		t.Fatalf("read reply: %v", err)
	}
	return r
}

type phoneFrame struct {
	Type string `json:"type"`
	Data struct {
		MessageID string `json:"message_id"`
		Text      string `json:"text"`
		Message   struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Body   struct {
				Text string `json:"text"`
			} `json:"body"`
		} `json:"message"`
	} `json:"data"`
}

func readPhone(t *testing.T, c *websocket.Conn) phoneFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var fr phoneFrame
	if err := wsjson.Read(ctx, c, &fr); err != nil {
		t.Fatalf("read phone frame: %v", err)
	}
	return fr
}

// The whole reason for socket operations: a stream's pieces as forty-byte
// frames, watched arriving on the person's socket one by one.
func TestStreamOverSocket(t *testing.T) {
	f := setupSocket(t)
	phone := f.srv.Socket(t, f.owner)
	readPhone(t, phone) // ready
	agent := f.srv.AgentSocket(t, f.socketSecret)

	send(t, agent, map[string]any{"op": "send", "cid": "c1", "conversation_id": f.dmID, "text": "plain"})
	reply := readReply(t, agent)
	if reply.ReplyToCID != "c1" || !reply.OK || reply.Message == nil || reply.Message.Status != "complete" {
		t.Fatalf("send reply = %+v", reply)
	}
	if fr := readPhone(t, phone); fr.Type != "message.created" || fr.Data.Message.Body.Text != "plain" {
		t.Errorf("phone got %+v after a plain send", fr)
	}

	send(t, agent, map[string]any{"op": "stream.start", "cid": "c2", "conversation_id": f.dmID})
	reply = readReply(t, agent)
	if reply.ReplyToCID != "c2" || !reply.OK || reply.Message == nil || reply.Message.Status != "streaming" {
		t.Fatalf("stream.start reply = %+v", reply)
	}
	id := reply.Message.ID
	if fr := readPhone(t, phone); fr.Type != "message.started" || fr.Data.Message.ID != id {
		t.Errorf("phone got %+v, want message.started", fr)
	}

	for _, piece := range []string{"I can ", "see ", "it."} {
		send(t, agent, map[string]any{"op": "stream.delta", "message_id": id, "text": piece})
		fr := readPhone(t, phone)
		if fr.Type != "message.delta" || fr.Data.MessageID != id || fr.Data.Text != piece {
			t.Errorf("phone got %+v, want delta %q", fr, piece)
		}
	}

	send(t, agent, map[string]any{"op": "stream.end", "cid": "c3", "message_id": id})
	reply = readReply(t, agent)
	if reply.ReplyToCID != "c3" || !reply.OK || reply.Message.Status != "complete" || reply.Message.Body.Text != "I can see it." {
		t.Fatalf("stream.end reply = %+v", reply)
	}
	if fr := readPhone(t, phone); fr.Type != "message.completed" || fr.Data.Message.Body.Text != "I can see it." {
		t.Errorf("phone got %+v, want message.completed with the whole text", fr)
	}

	t.Run("errors are answered", func(t *testing.T) {
		send(t, agent, map[string]any{"op": "stream.delta", "message_id": id, "text": "late"})
		if r := readReply(t, agent); r.OK || r.MessageID != id || r.Error == nil || r.Error.Code != "not_streaming" {
			t.Errorf("late delta reply = %+v", r)
		}
		send(t, agent, map[string]any{"op": "dance", "cid": "c4"})
		if r := readReply(t, agent); r.OK || r.ReplyToCID != "c4" || r.Error == nil || r.Error.Code != "unknown_op" {
			t.Errorf("unknown op reply = %+v", r)
		}
		send(t, agent, map[string]any{"op": "send", "cid": "c5", "conversation_id": f.dmID, "text": " "})
		if r := readReply(t, agent); r.OK || r.ReplyToCID != "c5" || r.Error == nil || r.Error.Code != "invalid_text" {
			t.Errorf("empty send reply = %+v", r)
		}
	})
}
