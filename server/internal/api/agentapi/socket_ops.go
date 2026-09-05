package agentapi

import (
	"context"

	"github.com/coder/websocket/wsjson"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
)

// Socket operations. They mirror the HTTP endpoints exactly and exist for
// the traffic where a request per call would hurt: a stream's pieces.
const (
	opSend        = "send"
	opStreamStart = "stream.start"
	opStreamDelta = "stream.delta"
	opStreamEnd   = "stream.end"
	opTyping      = "typing"
)

// replyFrame answers an operation. Operations that create or finish a
// message are answered by correlation id; a delta is answered only when it
// fails, by message id.
type replyFrame struct {
	ReplyToCID string          `json:"reply_to_cid,omitempty"`
	OK         bool            `json:"ok"`
	Message    *events.Message `json:"message,omitempty"`
	MessageID  string          `json:"message_id,omitempty"`
	Error      *errorDetail    `json:"error,omitempty"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func errorOf(err error) *errorDetail {
	if e, ok := domain.AsError(err); ok {
		return &errorDetail{Code: e.Code, Message: e.Message}
	}
	return &errorDetail{Code: "internal_error", Message: "Something went wrong on our side."}
}

// handleOp performs one operation and writes its reply. False means the
// connection is gone.
func (s *agentSocket) handleOp(ctx context.Context, op inboundFrame) bool {
	switch op.Op {
	case opSend, opStreamStart:
		conversationID, err := domain.ParseID(domain.PrefixConv, op.ConversationID)
		if err != nil {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, Error: errorOf(err)})
		}
		replyTo, err := replyToOf(op.ReplyTo)
		if err != nil {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, Error: errorOf(err)})
		}
		in := conversations.SendInput{
			Text: op.Text, IdempotencyKey: op.IdempotencyKey, ReplyTo: replyTo,
			Buttons: buttonsOf(op.Buttons), QuickReplies: quickRepliesOf(op.QuickReplies),
		}
		var res conversations.SendResult
		if op.Op == opSend {
			res, err = s.h.conversations.SendAsAgent(ctx, s.agentID, conversationID, in)
		} else {
			res, err = s.h.conversations.StartStream(ctx, s.agentID, conversationID, in)
		}
		if err != nil {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, Error: errorOf(err)})
		}
		msg := events.MessageOf(res.Message, s.agentName)
		return s.reply(ctx, replyFrame{ReplyToCID: op.CID, OK: true, Message: &msg})

	case opStreamDelta:
		messageID, err := domain.ParseID(domain.PrefixMessage, op.MessageID)
		if err == nil {
			err = s.h.conversations.AppendStream(ctx, s.agentID, messageID, op.Text)
		}
		if err != nil {
			return s.reply(ctx, replyFrame{MessageID: op.MessageID, Error: errorOf(err)})
		}
		return true

	case opStreamEnd:
		messageID, err := domain.ParseID(domain.PrefixMessage, op.MessageID)
		if err != nil {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, MessageID: op.MessageID, Error: errorOf(err)})
		}
		finished, err := s.h.conversations.FinishStream(ctx, s.agentID, messageID, conversations.FinishInput{
			Buttons: buttonsOf(op.Buttons), QuickReplies: quickRepliesOf(op.QuickReplies),
		})
		if err != nil {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, MessageID: op.MessageID, Error: errorOf(err)})
		}
		msg := events.MessageOf(finished, s.agentName)
		return s.reply(ctx, replyFrame{ReplyToCID: op.CID, OK: true, Message: &msg})

	case opTyping:
		conversationID, err := domain.ParseID(domain.PrefixConv, op.ConversationID)
		if err == nil {
			err = s.h.conversations.Typing(ctx, s.agentID, conversationID, op.State)
		}
		if err != nil {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, Error: errorOf(err)})
		}
		if op.CID != "" {
			return s.reply(ctx, replyFrame{ReplyToCID: op.CID, OK: true})
		}
		return true

	default:
		return s.reply(ctx, replyFrame{ReplyToCID: op.CID, Error: &errorDetail{
			Code: "unknown_op", Message: "Unknown operation " + op.Op + ".",
		}})
	}
}

func (s *agentSocket) reply(ctx context.Context, fr replyFrame) bool {
	wctx, cancel := context.WithTimeout(ctx, socketWriteTimeout)
	defer cancel()
	return wsjson.Write(wctx, s.c, fr) == nil
}
