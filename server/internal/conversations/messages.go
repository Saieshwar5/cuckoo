package conversations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Send limits. A person may burst a screenful, then one message every two
// seconds; a backend answering many conversations gets far more. Both are
// per sender, not per conversation, so spreading traffic across chats does
// not evade them.
var (
	userSendPolicy  = ratelimit.Policy{Rate: 0.5, Burst: 30}
	agentSendPolicy = ratelimit.Policy{Rate: 60, Burst: 120}
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
	text, err := validateText(in.Text)
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

	body, err := json.Marshal(Body{Text: text})
	if err != nil {
		return SendResult{}, domain.Internal(fmt.Errorf("encode message body: %w", err))
	}
	params := gen.CreateMessageParams{
		ID:             domain.NewID(),
		ConversationID: conversationID,
		SenderKind:     string(sender.Kind),
		Body:           body,
		IdempotencyKey: key,
	}
	switch sender.Kind {
	case ParticipantUser:
		params.SenderUserID = &sender.ID
	case ParticipantAgent:
		params.SenderAgentID = &sender.ID
	}

	var msg Message
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		row, err := tx.CreateMessage(ctx, params)
		if err != nil {
			return domain.Internal(fmt.Errorf("create message in %s: %w", conversationID, err))
		}
		if msg, err = messageFromRow(row); err != nil {
			return err
		}
		return fanOut(ctx, tx, msg)
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
	if err := s.attachDeliveryStatus(ctx, sent); err != nil {
		return SendResult{}, err
	}
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
	if err := s.attachDeliveryStatus(ctx, found); err != nil {
		return SendResult{}, false, err
	}
	return SendResult{Message: found[0], Created: false}, true, nil
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

	// One more than asked for tells us whether an older page exists without
	// a second query or a count.
	rows, err := s.store.ListMessagesBefore(ctx, gen.ListMessagesBeforeParams{
		ConversationID: conversationID,
		Before:         in.Before,
		PageSize:       limit + 1,
	})
	if err != nil {
		return Page{}, domain.Internal(fmt.Errorf("list messages of %s: %w", conversationID, err))
	}
	page, err := pageOf(rows, limit)
	if err != nil {
		return Page{}, err
	}
	if err := s.attachDeliveryStatus(ctx, page.Messages); err != nil {
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
	return pageOf(rows, limit)
}

// pageOf turns limit+1 rows into a page and a cursor.
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
