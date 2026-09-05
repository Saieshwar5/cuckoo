package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Send limits. A person may burst a screenful, then one message every two
// seconds; a backend answering many conversations gets far more. Both are
// per sender, not per conversation, so spreading traffic across chats does
// not evade them. A stream's pieces have a bound of their own, so a long
// answer does not spend the agent's message budget word by word.
var (
	userSendPolicy   = ratelimit.Policy{Rate: 0.5, Burst: 30}
	agentSendPolicy  = ratelimit.Policy{Rate: 60, Burst: 120}
	agentDeltaPolicy = ratelimit.Policy{Rate: 200, Burst: 400}
)

// SendAsUser records a message from a person into a conversation they are a
// member of, and records that every agent in it must be told.
//
// Recording is all it does. The rows saying who must hear the message are
// written in the same transaction as the message, and the delivery layer
// works from those, so a message is never lost because the thing that would
// have delivered it was not running at the time.
func (s *Service) SendAsUser(ctx context.Context, userID, conversationID uuid.UUID, in SendInput) (SendResult, error) {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return SendResult{}, err
	}
	return s.send(ctx, Sender{Kind: ParticipantUser, ID: userID}, conversationID, in, userSendPolicy)
}

// SendAsAgent records a message from an agent's backend into a conversation
// the agent is a member of. Every other agent in it is told; the agent
// itself never hears its own message back.
func (s *Service) SendAsAgent(ctx context.Context, agentID, conversationID uuid.UUID, in SendInput) (SendResult, error) {
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return SendResult{}, err
	}
	return s.send(ctx, Sender{Kind: ParticipantAgent, ID: agentID}, conversationID, in, agentSendPolicy)
}

func (s *Service) send(ctx context.Context, sender Sender, conversationID uuid.UUID, in SendInput, policy ratelimit.Policy) (SendResult, error) {
	return s.create(ctx, sender, conversationID, in, policy, false)
}

// create records a message. A streaming one starts empty and is told to
// nobody but the person's devices; its agents hear about it when it
// finishes, as a whole.
func (s *Service) create(ctx context.Context, sender Sender, conversationID uuid.UUID, in SendInput,
	policy ratelimit.Policy, streaming bool) (SendResult, error) {

	body, tapped, err := s.composeBody(ctx, sender, conversationID, in, streaming)
	if err != nil {
		return SendResult{}, err
	}
	replyTo, err := s.validateReplyTo(ctx, conversationID, in.ReplyTo)
	if err != nil {
		return SendResult{}, err
	}
	key, err := validateIdempotencyKey(in.IdempotencyKey)
	if err != nil {
		return SendResult{}, err
	}

	// A retry of a message we already have is answered before the rate
	// limit is consulted: it costs nothing and refusing it would make the
	// retrying client believe the send failed.
	if key != nil {
		if existing, found, err := s.findByKey(ctx, sender, conversationID, *key); err != nil || found {
			return existing, err
		}
	}

	wait, err := s.limiter.Allow(ctx, "send:"+string(sender.Kind)+":"+sender.ID.String(), policy)
	if err != nil {
		return SendResult{}, domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return SendResult{}, domain.RateLimited("rate_limited",
			"You are sending messages too quickly.", wait)
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return SendResult{}, domain.Internal(fmt.Errorf("encode message body: %w", err))
	}
	params := gen.CreateMessageParams{
		ID:               domain.NewID(),
		ConversationID:   conversationID,
		SenderKind:       string(sender.Kind),
		Body:             raw,
		IdempotencyKey:   key,
		Status:           string(MessageComplete),
		ReplyToMessageID: replyTo,
	}
	if streaming {
		params.Status = string(MessageStreaming)
	}
	switch sender.Kind {
	case ParticipantUser:
		params.SenderUserID = &sender.ID
	case ParticipantAgent:
		params.SenderAgentID = &sender.ID
	}

	var (
		msg     Message
		pending []uuid.UUID
	)
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		row, err := tx.CreateMessage(ctx, params)
		if err != nil {
			return domain.Internal(fmt.Errorf("create message in %s: %w", conversationID, err))
		}
		if msg, err = messageFromRow(row); err != nil {
			return err
		}
		if tapped != nil {
			if err := recordTap(ctx, tx, tapped); err != nil {
				return err
			}
		}
		if streaming {
			return nil
		}
		pending, err = fanOut(ctx, tx, msg)
		return err
	})
	if err != nil {
		// Two retries of the same send raced; the other one won. Hand back
		// what it created.
		if key != nil && store.IsUniqueViolation(err) {
			if existing, found, err := s.findByKey(ctx, sender, conversationID, *key); err != nil || found {
				return existing, err
			}
		}
		if _, classified := domain.AsError(err); classified {
			return SendResult{}, err
		}
		return SendResult{}, domain.Internal(fmt.Errorf("send message: %w", err))
	}

	sent := []Message{msg}
	if err := s.decorate(ctx, sent, true); err != nil {
		return SendResult{}, err
	}
	if streaming {
		if err := s.openStream(ctx, sent[0]); err != nil {
			return SendResult{}, err
		}
		s.notify(ctx, EventMessageStarted, conversationID,
			MessageCreatedEvent{ConversationID: conversationID, Message: sent[0]})
		return SendResult{Message: sent[0], Created: true}, nil
	}
	s.notify(ctx, EventMessageCreated, conversationID,
		MessageCreatedEvent{ConversationID: conversationID, Message: sent[0]})
	s.nudge(ctx, pending)
	return SendResult{Message: sent[0], Created: true}, nil
}

// findByKey looks for the message a sender already created under a key. A
// key reused for a different conversation is refused: the sender's retry
// logic is confused, and silently returning a message from elsewhere would
// hide that.
func (s *Service) findByKey(ctx context.Context, sender Sender, conversationID uuid.UUID, key string) (SendResult, bool, error) {
	var (
		row gen.Message
		err error
	)
	switch sender.Kind {
	case ParticipantUser:
		row, err = s.store.GetMessageByUserKey(ctx, gen.GetMessageByUserKeyParams{
			SenderUserID: sender.ID, IdempotencyKey: key,
		})
	case ParticipantAgent:
		row, err = s.store.GetMessageByAgentKey(ctx, gen.GetMessageByAgentKeyParams{
			SenderAgentID: sender.ID, IdempotencyKey: key,
		})
	}
	if err != nil {
		if store.IsNoRows(err) {
			return SendResult{}, false, nil
		}
		return SendResult{}, false, domain.Internal(fmt.Errorf("find message by key: %w", err))
	}

	msg, err := messageFromRow(row)
	if err != nil {
		return SendResult{}, false, err
	}
	if msg.ConversationID != conversationID {
		return SendResult{}, false, domain.Conflict("idempotency_key_reused",
			"That idempotency key was already used for a message in another conversation.")
	}
	found := []Message{msg}
	if err := s.decorate(ctx, found, true); err != nil {
		return SendResult{}, false, err
	}
	return SendResult{Message: found[0], Created: false}, true, nil
}

// tap is a button press being turned into a message: the offering message,
// and the button taken.
type tap struct {
	source   Message
	buttonID string
}

// composeBody turns what a sender supplied into a message body, applying
// who may send what. People send text or a tap; agents send text with,
// optionally, buttons and quick replies; a stream starts empty.
func (s *Service) composeBody(ctx context.Context, sender Sender, conversationID uuid.UUID, in SendInput, streaming bool) (Body, *tap, error) {
	if streaming {
		if strings.TrimSpace(in.Text) != "" {
			return Body{}, nil, domain.InvalidField("text", "stream_with_text",
				"A streamed message starts empty; append its text after starting it.")
		}
		if len(in.Buttons) > 0 || len(in.QuickReplies) > 0 {
			return Body{}, nil, domain.InvalidField("buttons", "stream_with_buttons",
				"Buttons and quick replies go on the finish of a stream.")
		}
		if in.Action != nil {
			return Body{}, nil, domain.InvalidField("action", "action_not_allowed", "Only people tap buttons.")
		}
		if s.streams == nil {
			return Body{}, nil, domain.Internal(errors.New("streaming is not configured"))
		}
		return Body{}, nil, nil
	}

	switch sender.Kind {
	case ParticipantUser:
		if len(in.Buttons) > 0 || len(in.QuickReplies) > 0 {
			return Body{}, nil, domain.InvalidField("buttons", "buttons_not_allowed",
				"Only agents can offer buttons or quick replies.")
		}
		if in.Action != nil {
			if strings.TrimSpace(in.Text) != "" {
				return Body{}, nil, domain.InvalidField("text", "invalid_action",
					"A tap carries no text; the button's label becomes the text.")
			}
			source, button, err := s.resolveTap(ctx, conversationID, *in.Action)
			if err != nil {
				return Body{}, nil, err
			}
			return Body{Text: button.Label, Action: &Action{ButtonID: button.ID, SourceMessageID: source.ID}},
				&tap{source: source, buttonID: button.ID}, nil
		}
		text, err := validateText(in.Text)
		if err != nil {
			return Body{}, nil, err
		}
		return Body{Text: text}, nil, nil

	default:
		if in.Action != nil {
			return Body{}, nil, domain.InvalidField("action", "action_not_allowed", "Only people tap buttons.")
		}
		text, err := validateText(in.Text)
		if err != nil {
			return Body{}, nil, err
		}
		buttons, err := validateButtons(in.Buttons)
		if err != nil {
			return Body{}, nil, err
		}
		quick, err := validateQuickReplies(in.QuickReplies)
		if err != nil {
			return Body{}, nil, err
		}
		return Body{Text: text, Buttons: buttons, QuickReplies: quick}, nil, nil
	}
}

// resolveTap finds the button a person tapped: on a message in this
// conversation, from an agent, final, and offering that id.
func (s *Service) resolveTap(ctx context.Context, conversationID uuid.UUID, a Action) (Message, Button, error) {
	invalid := func(msg string) error { return domain.InvalidField("action", "invalid_action", msg) }

	row, err := s.store.GetMessage(ctx, a.SourceMessageID)
	if err != nil {
		if store.IsNoRows(err) {
			return Message{}, Button{}, invalid("That message does not exist.")
		}
		return Message{}, Button{}, domain.Internal(fmt.Errorf("get message %s: %w", a.SourceMessageID, err))
	}
	source, err := messageFromRow(row)
	if err != nil {
		return Message{}, Button{}, err
	}
	if source.ConversationID != conversationID {
		return Message{}, Button{}, invalid("That message is not in this conversation.")
	}
	if source.Sender.Kind != ParticipantAgent || source.Status != MessageComplete {
		return Message{}, Button{}, invalid("That message has no buttons.")
	}
	for _, row := range source.Body.Buttons {
		for _, b := range row {
			if b.ID == a.ButtonID {
				return source, b, nil
			}
		}
	}
	return Message{}, Button{}, invalid("That message has no button with that id.")
}

// recordTap notes on the offering message which button was taken, so the
// history shows the choice and the app can dim the row.
func recordTap(ctx context.Context, tx *store.Store, t *tap) error {
	body := t.source.Body
	body.SelectedButtonID = t.buttonID
	raw, err := json.Marshal(body)
	if err != nil {
		return domain.Internal(fmt.Errorf("encode message body: %w", err))
	}
	if err := tx.UpdateMessageBody(ctx, gen.UpdateMessageBodyParams{ID: t.source.ID, Body: raw}); err != nil {
		return domain.Internal(fmt.Errorf("record tap on %s: %w", t.source.ID, err))
	}
	return nil
}

// validateReplyTo checks that a quoted message is in the same conversation.
func (s *Service) validateReplyTo(ctx context.Context, conversationID uuid.UUID, id *uuid.UUID) (*uuid.UUID, error) {
	if id == nil {
		return nil, nil
	}
	row, err := s.store.GetMessage(ctx, *id)
	if err != nil {
		if store.IsNoRows(err) {
			return nil, domain.InvalidField("reply_to", "invalid_reply_to", "That message does not exist.")
		}
		return nil, domain.Internal(fmt.Errorf("get message %s: %w", *id, err))
	}
	if row.ConversationID != conversationID {
		return nil, domain.InvalidField("reply_to", "invalid_reply_to", "That message is not in this conversation.")
	}
	return id, nil
}

// GetMessages returns the given messages in no particular order, with no
// membership check. See GetConversations for who may call this.
func (s *Service) GetMessages(ctx context.Context, ids []uuid.UUID) ([]Message, error) {
	rows, err := s.store.ListMessagesByIDs(ctx, ids)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list messages by id: %w", err))
	}
	out := make([]Message, 0, len(rows))
	for _, r := range rows {
		msg, err := messageFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := s.decorate(ctx, out, false); err != nil {
		return nil, err
	}
	return out, nil
}

// ListMessages returns one page of a conversation's history, newest first, to
// a caller who is a member of it.
func (s *Service) ListMessages(ctx context.Context, callerID, conversationID uuid.UUID, in ListMessagesInput) (Page, error) {
	if _, err := s.member(ctx, callerID, conversationID); err != nil {
		return Page{}, err
	}
	limit, err := pageSize(in.Limit)
	if err != nil {
		return Page{}, err
	}
	if in.Before != nil && in.After != nil {
		return Page{}, domain.InvalidField("after", "invalid_cursor",
			"Use before or after, not both.")
	}

	var page Page
	if in.After != nil {
		rows, err := s.store.ListMessagesAfter(ctx, gen.ListMessagesAfterParams{
			ConversationID: conversationID,
			After:          *in.After,
			PageSize:       limit + 1,
		})
		if err != nil {
			return Page{}, domain.Internal(fmt.Errorf("list messages of %s after: %w", conversationID, err))
		}
		if page, err = pageOf(rows, limit); err != nil {
			return Page{}, err
		}
		page.NextAfter, page.NextBefore = page.NextBefore, nil
	} else {
		// One more than asked for tells us whether another page exists
		// without a second query or a count.
		rows, err := s.store.ListMessagesBefore(ctx, gen.ListMessagesBeforeParams{
			ConversationID: conversationID,
			Before:         in.Before,
			PageSize:       limit + 1,
		})
		if err != nil {
			return Page{}, domain.Internal(fmt.Errorf("list messages of %s: %w", conversationID, err))
		}
		if page, err = pageOf(rows, limit); err != nil {
			return Page{}, err
		}
	}
	if err := s.decorate(ctx, page.Messages, true); err != nil {
		return Page{}, err
	}
	return page, nil
}

// ListMessagesForAgent is ListMessages for an agent caller: only what was
// said since it joined. Delivery status is the sender's concern and is not
// filled in.
func (s *Service) ListMessagesForAgent(ctx context.Context, agentID, conversationID uuid.UUID, in ListMessagesInput) (Page, error) {
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return Page{}, err
	}
	limit, err := pageSize(in.Limit)
	if err != nil {
		return Page{}, err
	}

	rows, err := s.store.ListMessagesBeforeForAgent(ctx, gen.ListMessagesBeforeForAgentParams{
		AgentID:        agentID,
		ConversationID: conversationID,
		Before:         in.Before,
		PageSize:       limit + 1,
	})
	if err != nil {
		return Page{}, domain.Internal(fmt.Errorf("list messages of %s for agent: %w", conversationID, err))
	}
	page, err := pageOf(rows, limit)
	if err != nil {
		return Page{}, err
	}
	if err := s.decorate(ctx, page.Messages, false); err != nil {
		return Page{}, err
	}
	return page, nil
}

// pageOf turns limit+1 rows into a page and a cursor on its last message,
// whichever direction the rows came in. The caller names the cursor.
func pageOf(rows []gen.Message, limit int32) (Page, error) {
	page := Page{Messages: make([]Message, 0, len(rows))}
	for _, r := range rows[:min(len(rows), int(limit))] {
		msg, err := messageFromRow(r)
		if err != nil {
			return Page{}, err
		}
		page.Messages = append(page.Messages, msg)
	}
	if len(rows) > int(limit) {
		oldest := page.Messages[len(page.Messages)-1].ID
		page.NextBefore = &oldest
	}
	return page, nil
}
