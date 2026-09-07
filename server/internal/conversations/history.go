package conversations

// Reading history: a page of a conversation as a person or an agent sees
// it, and messages by id for the callers that already know which they want.

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

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
	conv, err := s.member(ctx, callerID, conversationID)
	if err != nil {
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
			UserID:         callerID,
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
		page.ConversationStarted = conv.CreatedAt
	} else {
		// One more than asked for tells us whether another page exists
		// without a second query or a count.
		rows, err := s.store.ListMessagesBefore(ctx, gen.ListMessagesBeforeParams{
			UserID:         callerID,
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
		page.ConversationStarted = conv.CreatedAt
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
