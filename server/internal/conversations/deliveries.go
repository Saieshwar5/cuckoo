package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// fanOut records that every agent in the conversation, other than the
// sender, must be told about the message. It runs in the message's own
// transaction and returns the agents that now have something pending, so
// the caller can nudge their sockets once the transaction has committed.
//
// An agent with no backend connected gets a row that has already failed: the
// user sees "not delivered" now rather than a tick that never comes, and the
// event still exists for a backend that connects later and reads its history.
func fanOut(ctx context.Context, tx *store.Store, msg Message) ([]uuid.UUID, error) {
	agentIDs, err := tx.ListAgentParticipants(ctx, msg.ConversationID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list agents of %s: %w", msg.ConversationID, err))
	}

	var pending []uuid.UUID
	for _, agentID := range agentIDs {
		if msg.Sender.Kind == ParticipantAgent && msg.Sender.ID == agentID {
			continue
		}

		status := DeliveryPending
		var lastError *string
		if _, err := tx.GetActiveBinding(ctx, agentID); err != nil {
			if !store.IsNoRows(err) {
				return nil, domain.Internal(fmt.Errorf("get binding of %s: %w", agentID, err))
			}
			status = DeliveryFailed
			reason := "no_binding"
			lastError = &reason
		} else {
			pending = append(pending, agentID)
		}

		messageID := msg.ID
		_, err := tx.CreateDelivery(ctx, gen.CreateDeliveryParams{
			ID:             domain.NewID(),
			MessageID:      &messageID,
			ConversationID: msg.ConversationID,
			AgentID:        agentID,
			EventType:      EventMessageCreated,
			Status:         string(status),
			LastError:      lastError,
		})
		if err != nil {
			return nil, domain.Internal(fmt.Errorf("create delivery of %s to %s: %w", msg.ID, agentID, err))
		}
	}
	return pending, nil
}

// attachDeliveryStatus fills in DeliveryStatus for a batch of messages with
// one query.
func (s *Service) attachDeliveryStatus(ctx context.Context, msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(msgs))
	for i := range msgs {
		ids[i] = msgs[i].ID
	}

	rows, err := s.store.SummarizeDeliveries(ctx, ids)
	if err != nil {
		return domain.Internal(fmt.Errorf("summarise deliveries: %w", err))
	}
	byMessage := make(map[uuid.UUID]gen.SummarizeDeliveriesRow, len(rows))
	for _, r := range rows {
		byMessage[r.MessageID] = r
	}
	for i := range msgs {
		if r, ok := byMessage[msgs[i].ID]; ok {
			msgs[i].DeliveryStatus = summarize(r)
		}
	}
	return nil
}

// summarize turns per-backend outcomes into the one status a sender sees.
// Anyone still waiting means waiting; nobody having it means failed; at
// least one backend having it means delivered.
func summarize(r gen.SummarizeDeliveriesRow) DeliveryStatus {
	switch {
	case r.Delivered+r.Failed < r.Total:
		return DeliveryPending
	case r.Delivered == 0:
		return DeliveryFailed
	default:
		return DeliveryDelivered
	}
}
