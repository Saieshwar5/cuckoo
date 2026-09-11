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
	"encoding/json"
	"fmt"
	"sort"
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

// Delivery is one event owed to one agent. MessageID is set for a message
// event; a membership event has none and carries Payload instead.
type Delivery struct {
	ID             uuid.UUID
	MessageID      *uuid.UUID
	ConversationID uuid.UUID
	AgentID        uuid.UUID
	EventType      string
	Payload        []byte
	Status         Status
	Attempts       int
	NextAttemptAt  time.Time
	LastError      string
	DeliveredAt    *time.Time
	CreatedAt      time.Time
}

func fromRow(r gen.MessageDelivery) Delivery {
	d := Delivery{
		ID:             r.ID,
		MessageID:      r.MessageID,
		ConversationID: r.ConversationID,
		AgentID:        r.AgentID,
		EventType:      r.EventType,
		Payload:        r.Payload,
		Status:         Status(r.Status),
		Attempts:       int(r.Attempts),
		NextAttemptAt:  r.NextAttemptAt,
		DeliveredAt:    r.DeliveredAt,
		CreatedAt:      r.CreatedAt,
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

// socketBatch bounds one replay pass; a full batch means more may wait and
// the socket goes straight round again.
const socketBatch = 100

// ClaimForSocket leases the agent's due deliveries to the socket calling it
// and returns them as envelopes, oldest first. More reports whether a full
// batch came back.
//
// It is the socket transport's half of the outbox: the same rows, the same
// thirty-second lease, so an event pushed and not acknowledged comes due
// again and is pushed again, and a reconnecting backend receives in order
// everything it never acknowledged.
func (s *Service) ClaimForSocket(ctx context.Context, agentID uuid.UUID) (envelopes []events.Envelope, more bool, err error) {
	var rows []gen.MessageDelivery
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		var err error
		rows, err = tx.ClaimDueSocketDeliveries(ctx, gen.ClaimDueSocketDeliveriesParams{
			AgentID: agentID, BatchSize: socketBatch,
		})
		return err
	})
	if err != nil {
		return nil, false, domain.Internal(fmt.Errorf("claim socket deliveries of %s: %w", agentID, err))
	}
	// An UPDATE's RETURNING order is not guaranteed; replay order is.
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID.String() < rows[j].ID.String() })

	ds := make([]Delivery, 0, len(rows))
	for _, r := range rows {
		ds = append(ds, fromRow(r))
	}
	envelopes, err = s.envelopes(ctx, ds)
	return envelopes, len(rows) == socketBatch, err
}

// Ack records that the agent's backend has an event. It reports false for an
// event that is not pending for this agent, which a retrying backend and a
// mistaken one both produce and neither needs told about. The sender's tick
// mark is announced on success.
func (s *Service) Ack(ctx context.Context, agentID, eventID uuid.UUID) (bool, error) {
	messageID, err := s.store.AckDelivery(ctx, gen.AckDeliveryParams{ID: eventID, AgentID: agentID})
	if err != nil {
		if store.IsNoRows(err) {
			return false, nil
		}
		return false, domain.Internal(fmt.Errorf("ack %s for %s: %w", eventID, agentID, err))
	}
	if messageID == nil {
		// A membership event: nobody's tick mark depends on it.
		return true, nil
	}
	return true, s.conversations.DeliveryChanged(ctx, *messageID)
}

// envelopes builds the wire form of a batch of deliveries, in order, loading
// the messages and conversations they refer to in two queries.
func (s *Service) envelopes(ctx context.Context, ds []Delivery) ([]events.Envelope, error) {
	out := make([]events.Envelope, 0, len(ds))
	if len(ds) == 0 {
		return out, nil
	}

	msgIDs := make([]uuid.UUID, 0, len(ds))
	convIDs := make([]uuid.UUID, 0, len(ds))
	for _, d := range ds {
		if d.MessageID != nil {
			msgIDs = append(msgIDs, *d.MessageID)
		}
		convIDs = append(convIDs, d.ConversationID)
	}
	byMessage := make(map[uuid.UUID]conversations.Message, len(msgIDs))
	if len(msgIDs) > 0 {
		msgs, err := s.conversations.GetMessages(ctx, msgIDs)
		if err != nil {
			return nil, err
		}
		for _, m := range msgs {
			byMessage[m.ID] = m
		}
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
		conv, ok := byConversation[d.ConversationID]
		if !ok {
			return nil, domain.Internal(fmt.Errorf("delivery %s refers to missing conversation %s", d.ID, d.ConversationID))
		}
		env, err := envelopeOf(d, conv, byMessage)
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, nil
}

// envelopeOf renders one delivery in the wire form of its kind.
func envelopeOf(d Delivery, conv conversations.Conversation, byMessage map[uuid.UUID]conversations.Message) (events.Envelope, error) {
	switch d.EventType {
	case conversations.EventMessageCreated:
		if d.MessageID == nil {
			return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s has no message", d.ID))
		}
		msg, ok := byMessage[*d.MessageID]
		if !ok {
			return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s refers to missing message %s", d.ID, *d.MessageID))
		}
		return events.NewMessageCreated(d.ID, d.CreatedAt, d.AgentID, msg, conv), nil
	case conversations.EventConversationJoined:
		var p conversations.JoinedPayload
		if err := json.Unmarshal(d.Payload, &p); err != nil {
			return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s payload: %w", d.ID, err))
		}
		return events.NewConversationJoined(d.ID, d.CreatedAt, d.AgentID, conv, p), nil
	case conversations.EventConversationLeft:
		var p conversations.LeftPayload
		if err := json.Unmarshal(d.Payload, &p); err != nil {
			return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s payload: %w", d.ID, err))
		}
		return events.NewConversationLeft(d.ID, d.CreatedAt, d.AgentID, conv, p.Reason), nil
	case conversations.EventStopRequested:
		var p conversations.StopPayload
		if err := json.Unmarshal(d.Payload, &p); err != nil {
			return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s payload: %w", d.ID, err))
		}
		return events.NewStopRequested(d.ID, d.CreatedAt, d.AgentID, conv, p.MessageID), nil
	case conversations.EventScheduleRequested, conversations.EventScheduleUpdated, conversations.EventScheduleDeleted:
		var sch conversations.Schedule
		if err := json.Unmarshal(d.Payload, &sch); err != nil {
			return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s payload: %w", d.ID, err))
		}
		return events.NewScheduleEvent(d.ID, d.CreatedAt, d.AgentID, d.EventType, conv, sch), nil
	default:
		return events.Envelope{}, domain.Internal(fmt.Errorf("delivery %s has unknown type %q", d.ID, d.EventType))
	}
}
