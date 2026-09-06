package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// What a person may put out of their own sight. Nothing here touches what
// anyone else sees: the agent has the message already, and a chat cleared
// on one side is a chat with an honest history on the other. The words in
// the app say "on this device" for exactly that reason, even though the
// hub is what remembers the clearing.

// Clear hides everything said so far in a conversation from the caller.
// Whatever is said next shows as usual.
func (s *Service) Clear(ctx context.Context, callerID, conversationID uuid.UUID) error {
	if _, err := s.member(ctx, callerID, conversationID); err != nil {
		return err
	}
	// participants.user_id is nullable, since a row may be an agent's.
	if _, err := s.store.ClearConversation(ctx, gen.ClearConversationParams{
		ConversationID: conversationID, UserID: &callerID,
	}); err != nil {
		return domain.Internal(fmt.Errorf("clear %s for %s: %w", conversationID, callerID, err))
	}
	return nil
}

// Hide takes one message out of the caller's view. Hiding it twice, or
// hiding one already cleared, changes nothing.
func (s *Service) Hide(ctx context.Context, callerID, conversationID, messageID uuid.UUID) error {
	if _, err := s.member(ctx, callerID, conversationID); err != nil {
		return err
	}
	row, err := s.store.GetMessage(ctx, messageID)
	if err != nil {
		if store.IsNoRows(err) {
			return errMessageNotFound()
		}
		return domain.Internal(fmt.Errorf("get message %s: %w", messageID, err))
	}
	if row.ConversationID != conversationID {
		return errMessageNotFound()
	}
	if err := s.store.HideMessage(ctx, gen.HideMessageParams{UserID: callerID, MessageID: messageID}); err != nil {
		return domain.Internal(fmt.Errorf("hide %s for %s: %w", messageID, callerID, err))
	}
	return nil
}

func errMessageNotFound() error {
	return domain.NotFound("message_not_found", "That message does not exist.")
}
