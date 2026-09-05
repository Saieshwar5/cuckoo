package conversations

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// SendText records a text message from the caller in a conversation they are
// a member of.
//
// Recording is all it does. Who should hear about the message, and how, is
// decided by the delivery layer from the same row, so a message is never lost
// because the thing that would have delivered it was not running.
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

	row, err := s.store.CreateMessage(ctx, gen.CreateMessageParams{
		ID:             domain.NewID(),
		ConversationID: conversationID,
		SenderKind:     string(ParticipantUser),
		SenderUserID:   &callerID,
		Body:           body,
	})
	if err != nil {
		return Message{}, domain.Internal(fmt.Errorf("create message in %s: %w", conversationID, err))
	}
	return messageFromRow(row)
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
	return page, nil
}
