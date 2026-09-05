package conversations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// SendText records a text message from the caller in a conversation they are
// a member of, and records that every agent in it must be told.
//
// Recording is all it does. The rows saying who must hear the message are
// written in the same transaction as the message, and the delivery layer
// works from those, so a message is never lost because the thing that would
// have delivered it was not running at the time.
func (s *Service) SendText(ctx context.Context, callerID, conversationID uuid.UUID, text string) (Message, error) {
	if _, err := s.member(ctx, callerID, conversationID); err != nil {
		return Message{}, err
	}
	text, err := validateText(text)
	if err != nil {
		return Message{}, err
	}

	body, err := json.Marshal(Body{Text: text})
	if err != nil {
		return Message{}, domain.Internal(fmt.Errorf("encode message body: %w", err))
	}

	var msg Message
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		row, err := tx.CreateMessage(ctx, gen.CreateMessageParams{
			ID:             domain.NewID(),
			ConversationID: conversationID,
			SenderKind:     string(ParticipantUser),
			SenderUserID:   &callerID,
			Body:           body,
		})
		if err != nil {
			return domain.Internal(fmt.Errorf("create message in %s: %w", conversationID, err))
		}
		if msg, err = messageFromRow(row); err != nil {
			return err
		}
		return fanOut(ctx, tx, msg)
	})
	if err != nil {
		if _, classified := domain.AsError(err); classified {
			return Message{}, err
		}
		return Message{}, domain.Internal(fmt.Errorf("send message: %w", err))
	}

	sent := []Message{msg}
	if err := s.attachDeliveryStatus(ctx, sent); err != nil {
		return Message{}, err
	}
	return sent[0], nil
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
	if err := s.attachDeliveryStatus(ctx, page.Messages); err != nil {
		return Page{}, err
	}
	return page, nil
}
