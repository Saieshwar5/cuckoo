// Package delivery gets events to agent backends and keeps trying until it
// has.
//
// The outbox rows written when a message is sent are the whole of its state.
// A worker leases due rows, posts them to webhook backends, and records what
// happened; a backend that was away reads the same rows back in order. There
// is no queue beside the database, so nothing can be in the queue and not in
// the database, or the reverse.
package delivery

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Status is where a delivery stands.
type Status string

const (
	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
)

// Delivery is one event owed to one agent.
type Delivery struct {
	ID            uuid.UUID
	MessageID     uuid.UUID
	AgentID       uuid.UUID
	EventType     string
	Status        Status
	Attempts      int
	NextAttemptAt time.Time
	LastError     string
	DeliveredAt   *time.Time
	CreatedAt     time.Time
}

func fromRow(r gen.MessageDelivery) Delivery {
	d := Delivery{
		ID:            r.ID,
		MessageID:     r.MessageID,
		AgentID:       r.AgentID,
		EventType:     r.EventType,
		Status:        Status(r.Status),
		Attempts:      int(r.Attempts),
		NextAttemptAt: r.NextAttemptAt,
		DeliveredAt:   r.DeliveredAt,
		CreatedAt:     r.CreatedAt,
	}
	if r.LastError != nil {
		d.LastError = *r.LastError
	}
	return d
}

// Service reads the outbox on behalf of the agent protocol.
type Service struct {
	store         *store.Store
	conversations *conversations.Service
}

// New builds the service.
func New(st *store.Store, conversationService *conversations.Service) *Service {
	return &Service{store: st, conversations: conversationService}
}

const (
	defaultPageSize int32 = 50
	maxPageSize     int32 = 100
)

// ListEventsInput selects a page of an agent's events. A nil Since starts at
// the beginning; a zero Limit means the default page size.
type ListEventsInput struct {
	Since *uuid.UUID
	Limit int
}

// ListEvents returns an agent's events oldest first, for a backend catching
// up after downtime, or one that polls instead of receiving webhooks. It
// returns every event regardless of delivery state: the caller wants what it
// missed, not what we think it has.
func (s *Service) ListEvents(ctx context.Context, agentID uuid.UUID, in ListEventsInput) ([]events.Envelope, error) {
	limit := defaultPageSize
	if in.Limit != 0 {
		if in.Limit < 1 || in.Limit > int(maxPageSize) {
			return nil, domain.InvalidField("limit", "invalid_limit",
				fmt.Sprintf("Limit must be between 1 and %d.", maxPageSize))
		}
		limit = int32(in.Limit) //nolint:gosec // bounded just above
	}

	rows, err := s.store.ListDeliveriesSince(ctx, gen.ListDeliveriesSinceParams{
		AgentID: agentID, Since: in.Since, PageSize: limit,
	})
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list events of %s: %w", agentID, err))
	}
	ds := make([]Delivery, 0, len(rows))
	for _, r := range rows {
		ds = append(ds, fromRow(r))
	}
	return s.envelopes(ctx, ds)
}

// envelopes builds the wire form of a batch of deliveries, in order, loading
// the messages and conversations they refer to in two queries.
func (s *Service) envelopes(ctx context.Context, ds []Delivery) ([]events.Envelope, error) {
	out := make([]events.Envelope, 0, len(ds))
	if len(ds) == 0 {
		return out, nil
	}

	msgIDs := make([]uuid.UUID, 0, len(ds))
	for _, d := range ds {
		msgIDs = append(msgIDs, d.MessageID)
	}
	msgs, err := s.conversations.GetMessages(ctx, msgIDs)
	if err != nil {
		return nil, err
	}
	byMessage := make(map[uuid.UUID]conversations.Message, len(msgs))
	convIDs := make([]uuid.UUID, 0, len(msgs))
	for _, m := range msgs {
		byMessage[m.ID] = m
		convIDs = append(convIDs, m.ConversationID)
	}
	convs, err := s.conversations.GetConversations(ctx, convIDs)
	if err != nil {
		return nil, err
	}
	byConversation := make(map[uuid.UUID]conversations.Conversation, len(convs))
	for _, c := range convs {
		byConversation[c.ID] = c
	}

	for _, d := range ds {
		msg, ok := byMessage[d.MessageID]
		if !ok {
			return nil, domain.Internal(fmt.Errorf("delivery %s refers to missing message %s", d.ID, d.MessageID))
		}
		conv, ok := byConversation[msg.ConversationID]
		if !ok {
			return nil, domain.Internal(fmt.Errorf("message %s refers to missing conversation %s", msg.ID, msg.ConversationID))
		}
		out = append(out, events.NewMessageCreated(d.ID, d.CreatedAt, d.AgentID, msg, conv))
	}
	return out, nil
}
