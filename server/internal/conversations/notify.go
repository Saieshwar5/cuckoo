package conversations

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
)

// DeliveryChanged announces that a message's tick mark changed. The delivery
// worker calls it after recording an outcome.
func (s *Service) DeliveryChanged(ctx context.Context, messageID uuid.UUID) error {
	msgs, err := s.GetMessages(ctx, []uuid.UUID{messageID})
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	if err := s.attachDeliveryStatus(ctx, msgs); err != nil {
		return err
	}
	msg := msgs[0]
	s.notify(ctx, EventDeliveryUpdated, msg.ConversationID, DeliveryUpdatedEvent{
		ConversationID: msg.ConversationID,
		MessageID:      msg.ID,
		DeliveryStatus: msg.DeliveryStatus,
	})
	return nil
}

// nudge tells the sockets of agents that their outbox has something new.
// Agents on webhooks have no socket and hear nothing, which is fine: the
// worker polls for them.
func (s *Service) nudge(ctx context.Context, agentIDs []uuid.UUID) {
	if len(agentIDs) == 0 {
		return
	}
	ev, err := realtime.NewEvent(EventDeliveryPending, agentIDs, struct{}{})
	if err != nil {
		slog.WarnContext(ctx, "realtime: could not encode nudge", "error", err)
		return
	}
	if err := s.publisher.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "realtime: could not publish nudge", "error", err)
	}
}

// notify publishes an event to the devices of everyone in a conversation.
//
// It never fails the operation that triggered it: the record is already
// written, and a device that missed the announcement catches up from that
// record on its next connection. A failure to announce is logged and no more.
func (s *Service) notify(ctx context.Context, eventType string, conversationID uuid.UUID, payload any) {
	userIDs, err := s.store.ListUserParticipants(ctx, conversationID)
	if err != nil {
		slog.WarnContext(ctx, "realtime: could not list recipients",
			"event", eventType, "conversation", conversationID, "error", err)
		return
	}
	if len(userIDs) == 0 {
		return
	}
	ev, err := realtime.NewEvent(eventType, userIDs, payload)
	if err != nil {
		slog.WarnContext(ctx, "realtime: could not encode event",
			"event", eventType, "conversation", conversationID, "error", err)
		return
	}
	if err := s.publisher.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "realtime: could not publish",
			"event", eventType, "conversation", conversationID, "error", err)
	}
}
