// Package conversations owns conversations, who is in them, and the messages
// inside them.
//
// It is the participant-based core of the chat. A conversation is a set of
// members, each a person or an agent; a message is something one member said
// in it. Nothing here knows how a message reaches an agent's backend or how a
// phone learns that one arrived: delivery and live updates are built on top of
// this package, not into it, so the record of what was said never depends on
// whether anyone was listening.
package conversations

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Kind is the shape of a conversation.
type Kind string

// KindDM is a private conversation between one person and one agent.
const KindDM Kind = "dm"

// ParticipantKind says whether a member, or a sender, is a person or an agent.
type ParticipantKind string

const (
	ParticipantUser  ParticipantKind = "user"
	ParticipantAgent ParticipantKind = "agent"
)

// Participant is a member of a conversation as it should be shown: with the
// name it goes by now. Handle is set only for agents.
type Participant struct {
	Kind        ParticipantKind
	ID          uuid.UUID
	DisplayName string
	Handle      string
	JoinedAt    time.Time
}

// Conversation is what the chat list shows: who is in it and what was said
// last. LastMessage is nil until someone speaks.
type Conversation struct {
	ID           uuid.UUID
	Kind         Kind
	Participants []Participant
	LastMessage  *Message
	CreatedAt    time.Time
}

// Event types. EventMessageCreated is both what an agent's backend receives
// through the outbox and what a person's device hears live; it is the same
// fact. EventDeliveryUpdated is for devices only: a tick mark changed.
const (
	EventMessageCreated  = "message.created"
	EventDeliveryUpdated = "delivery.updated"
)

// MessageCreatedEvent is what a device hears when a message lands in one of
// its person's conversations. Internal form; the socket renders the wire
// shape.
type MessageCreatedEvent struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Message        Message   `json:"message"`
}

// DeliveryUpdatedEvent is what a device hears when a message's delivery
// status changes: the tick mark.
type DeliveryUpdatedEvent struct {
	ConversationID uuid.UUID      `json:"conversation_id"`
	MessageID      uuid.UUID      `json:"message_id"`
	DeliveryStatus DeliveryStatus `json:"delivery_status"`
}

// DeliveryStatus summarises whether the backends a message was for have it.
// Empty when the message had nobody to be delivered to.
type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

// Sender identifies who wrote a message.
type Sender struct {
	Kind ParticipantKind
	ID   uuid.UUID
}

// Body is the content of a message, stored as one JSON object.
//
// It is text alone today. Attachments, buttons and quick replies are further
// fields of this same object, which is why it is a document and not a column.
type Body struct {
	Text string `json:"text,omitempty"`
}

// Message is one thing said in a conversation.
type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Sender         Sender
	Body           Body
	DeliveryStatus DeliveryStatus
	CreatedAt      time.Time
}

// SendInput is what a sender supplies. IdempotencyKey is optional: a sender
// that retries after a lost response sends the same key and gets the same
// message back instead of a duplicate.
type SendInput struct {
	Text           string
	IdempotencyKey string
}

// SendResult is the message a send produced. Created is false when the
// idempotency key matched a message that already existed.
type SendResult struct {
	Message
	Created bool
}

// Page is one slice of a conversation's history. Paging backwards, messages
// are newest first and NextBefore is the cursor for older ones; paging
// forwards from After, they are oldest first and NextAfter continues. A nil
// cursor means the history ends there.
type Page struct {
	Messages   []Message
	NextBefore *uuid.UUID
	NextAfter  *uuid.UUID
}

// ListMessagesInput selects a page of history. Neither cursor means the
// newest page; Before pages back through older messages; After pages forward
// from a message the caller already has, which is how a device that was
// away fills its gap. A zero Limit means the default page size.
type ListMessagesInput struct {
	Before *uuid.UUID
	After  *uuid.UUID
	Limit  int
}

func participantFromRow(r gen.ListParticipantsRow) Participant {
	p := Participant{Kind: ParticipantKind(r.Kind), JoinedAt: r.JoinedAt}
	switch p.Kind {
	case ParticipantUser:
		p.ID = derefID(r.UserID)
		p.DisplayName = derefString(r.UserDisplayName)
	case ParticipantAgent:
		p.ID = derefID(r.AgentID)
		p.DisplayName = derefString(r.AgentDisplayName)
		p.Handle = derefString(r.AgentHandle)
	}
	return p
}

// messageFromRow decodes a stored message. A body that does not parse is a
// corrupt row, which is our fault and reported as such rather than hidden.
func messageFromRow(r gen.Message) (Message, error) {
	var body Body
	if err := json.Unmarshal(r.Body, &body); err != nil {
		return Message{}, domain.Internal(fmt.Errorf("decode body of message %s: %w", r.ID, err))
	}

	sender := Sender{Kind: ParticipantKind(r.SenderKind)}
	switch sender.Kind {
	case ParticipantUser:
		sender.ID = derefID(r.SenderUserID)
	case ParticipantAgent:
		sender.ID = derefID(r.SenderAgentID)
	}

	return Message{
		ID:             r.ID,
		ConversationID: r.ConversationID,
		Sender:         sender,
		Body:           body,
		CreatedAt:      r.CreatedAt,
	}, nil
}

// The database guarantees these pointers are set for the kind that needs them;
// dereferencing through a helper keeps a broken invariant from becoming a
// panic in a request.
func derefID(p *uuid.UUID) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return *p
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
