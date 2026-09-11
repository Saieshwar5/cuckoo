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

	// A finished message may also be worth waking a pocket for. Only a
	// finished one: a stream announces itself token by token, and a
	// notification per token is unusable.
	if eventType == EventMessageCreated || eventType == EventMessageCompleted {
		if ev, ok := payload.(MessageCreatedEvent); ok {
			s.pushed(ctx, conversationID, userIDs, ev.Message)
		}
	}
}

// Landed is a finished message and the people it reached, handed to whatever
// notifies phones. Declared here rather than in the push package so that
// nothing in the message path has to know push exists.
type Landed struct {
	ConversationID uuid.UUID
	MessageID      uuid.UUID
	// AgentID said it. The settings that decide whether to interrupt somebody
	// are per agent, and its name is the notification's title.
	AgentID uuid.UUID
	Text    string
	// Recipients are everyone in the conversation; the sender is removed by
	// the notifier rather than by every caller.
	Recipients     []uuid.UUID
	SenderID       uuid.UUID
	HasAttachments bool
}

// Pusher wakes the phones of people who are not looking. Optional: a hub with
// none simply does not notify.
type Pusher interface {
	MessageLanded(ctx context.Context, in Landed)
}

// pushed hands a finished message to whatever notifies phones, if anything
// does. Everything about whether it is welcome is decided there.
func (s *Service) pushed(ctx context.Context, conversationID uuid.UUID, userIDs []uuid.UUID, msg Message) {
	if s.push == nil || msg.Sender.Kind != ParticipantAgent || msg.Stopped {
		// Only an agent's words reach a person's lock screen. A person's own
		// message is already on their screen, and nobody else is in a DM. A
		// reply the person stopped is one they were watching, and chose to
		// end: announcing it would be the phone arguing with them.
		return
	}
	s.push.MessageLanded(ctx, Landed{
		ConversationID: conversationID,
		MessageID:      msg.ID,
		AgentID:        msg.Sender.ID,
		Text:           msg.Body.Text,
		Recipients:     userIDs,
		SenderID:       msg.Sender.ID,
		HasAttachments: len(msg.Body.Attachments) > 0,
	})
}
