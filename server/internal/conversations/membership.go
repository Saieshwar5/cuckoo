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

// Membership events, as the outbox carries them to a backend: the agent
// joined a conversation, or was left. Neither is about a message.
const (
	EventConversationJoined = "conversation.joined"
	EventConversationLeft   = "conversation.left"
)

// JoinedPayload is what the outbox row of a joined event holds: enough to
// tell the backend how the person arrived.
type JoinedPayload struct {
	PairTokenID *uuid.UUID      `json:"pair_token_id"`
	Payload     json.RawMessage `json:"payload"`
}

// LeftPayload is what the outbox row of a left event holds.
type LeftPayload struct {
	Reason string `json:"reason"`
}

// FindOrCreateDM returns the chat between a person and an agent, opening it
// if there is none. Created reports which.
func (s *Service) FindOrCreateDM(ctx context.Context, userID, agentID uuid.UUID) (Conversation, bool, error) {
	row, err := s.store.FindDM(ctx, gen.FindDMParams{UserID: userID, AgentID: agentID})
	if err == nil {
		convs, err := s.hydrate(ctx, []gen.Conversation{row})
		if err != nil {
			return Conversation{}, false, err
		}
		return convs[0], false, nil
	}
	if !store.IsNoRows(err) {
		return Conversation{}, false, domain.Internal(fmt.Errorf("find dm of %s and %s: %w", userID, agentID, err))
	}
	conv, err := s.CreateDM(ctx, userID, agentID)
	return conv, true, err
}

// Enqueue writes an outbox row for a membership event. Pending when the
// agent has a backend to tell, failed at once when it has none, so a
// backend that connects later still finds it in its history. It reports
// whether anything is pending, so the caller can nudge the agent's socket
// once its transaction has committed.
func (s *Service) Enqueue(ctx context.Context, agentID, conversationID uuid.UUID, eventType string, payload any) (bool, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, domain.Internal(fmt.Errorf("encode %s payload: %w", eventType, err))
	}
	status := DeliveryPending
	var lastError *string
	pending := true
	if _, err := s.store.GetActiveBinding(ctx, agentID); err != nil {
		if !store.IsNoRows(err) {
			return false, domain.Internal(fmt.Errorf("get binding of %s: %w", agentID, err))
		}
		status = DeliveryFailed
		reason := "no_binding"
		lastError = &reason
		pending = false
	}
	_, err = s.store.CreateDelivery(ctx, gen.CreateDeliveryParams{
		ID:             domain.NewID(),
		ConversationID: conversationID,
		AgentID:        agentID,
		EventType:      eventType,
		Payload:        raw,
		Status:         string(status),
		LastError:      lastError,
	})
	if err != nil {
		return false, domain.Internal(fmt.Errorf("enqueue %s for %s: %w", eventType, agentID, err))
	}
	return pending, nil
}

// Nudge tells agents' sockets that their outbox has something new.
func (s *Service) Nudge(ctx context.Context, agentIDs []uuid.UUID) {
	s.nudge(ctx, agentIDs)
}

// ensureOpen refuses to move anything in a chat where the person has
// blocked the agent. Reading history stays allowed: the block closes the
// conversation, it does not erase it.
func (s *Service) ensureOpen(ctx context.Context, conversationID uuid.UUID) error {
	blocked, err := s.store.IsConversationBlocked(ctx, conversationID)
	if err != nil {
		return domain.Internal(fmt.Errorf("check block on %s: %w", conversationID, err))
	}
	if blocked {
		return domain.Forbidden("blocked", "This chat is closed: the agent has been blocked.")
	}
	return nil
}
