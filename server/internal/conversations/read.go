package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// EventConversationRead tells a person's other devices how far they have
// read, so a badge cleared on one phone clears on the tablet too.
const EventConversationRead = "conversation.read"

// ReadEvent is the payload of EventConversationRead.
type ReadEvent struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	ReadUpTo       uuid.UUID `json:"read_up_to"`
}

// MarkRead records that a person has seen a conversation up to and
// including a message. Only ever forward: a device that was behind cannot
// unread what another has seen, and saying so again changes nothing.
func (s *Service) MarkRead(ctx context.Context, userID, conversationID, messageID uuid.UUID) error {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return err
	}
	row, err := s.store.GetMessage(ctx, messageID)
	if err != nil && !store.IsNoRows(err) {
		return domain.Internal(fmt.Errorf("get message %s: %w", messageID, err))
	}
	if err != nil || row.ConversationID != conversationID {
		return domain.NotFound("message_not_found", "That message is not in this conversation.")
	}

	moved, err := s.store.MarkRead(ctx, gen.MarkReadParams{
		ConversationID: conversationID, UserID: userID, MessageID: messageID,
	})
	if err != nil {
		return domain.Internal(fmt.Errorf("mark %s read for %s: %w", conversationID, userID, err))
	}
	if moved > 0 {
		s.notifyTo(ctx, EventConversationRead, []uuid.UUID{userID},
			ReadEvent{ConversationID: conversationID, ReadUpTo: messageID})
	}
	return nil
}

// attachUnread fills in how much of each conversation the viewer has not
// read, with one query for the whole list.
func (s *Service) attachUnread(ctx context.Context, viewer uuid.UUID, convs []Conversation) error {
	if len(convs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(convs))
	index := make(map[uuid.UUID]int, len(convs))
	for i, c := range convs {
		ids[i] = c.ID
		index[c.ID] = i
	}
	rows, err := s.store.CountUnread(ctx, gen.CountUnreadParams{UserID: viewer, ConversationIds: ids})
	if err != nil {
		return domain.Internal(fmt.Errorf("count unread for %s: %w", viewer, err))
	}
	for _, r := range rows {
		convs[index[r.ConversationID]].Unread = int(r.Unread)
	}
	return nil
}
