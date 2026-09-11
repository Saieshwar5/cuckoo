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
// name it goes by now. Handle and Status are set only for agents; Status is
// the live binding's health, or empty when no backend is connected.
type Participant struct {
	Kind        ParticipantKind
	ID          uuid.UUID
	DisplayName string
	Handle      string
	Status      string
	// HasAvatar says an agent has a published picture to fetch.
	HasAvatar bool
	// Starters are what an agent suggests saying first.
	Starters []string
	// SupportsSchedules says an agent's backend holds schedules.
	SupportsSchedules bool
	JoinedAt          time.Time
}

// Conversation is what the chat list shows: who is in it and what was said
// last. LastMessage is nil until someone speaks.
type Conversation struct {
	ID           uuid.UUID
	Kind         Kind
	Participants []Participant
	LastMessage  *Message
	// Unread is how many messages others sent after the viewer last read,
	// counted to 100. Zero for a reader with no view of their own.
	Unread    int
	CreatedAt time.Time
}

// Event types. EventMessageCreated is both what an agent's backend receives
// through the outbox and what a person's device hears live; it is the same
// fact. EventDeliveryUpdated is for devices only: a tick mark changed.
const (
	EventMessageCreated  = "message.created"
	EventDeliveryUpdated = "delivery.updated"
	// EventDeliveryPending is a nudge to an agent's socket: something is
	// waiting in the outbox. It carries nothing; the socket reads the rows.
	EventDeliveryPending = "delivery.pending"
	// The life of a streamed message, as a device sees it: an empty bubble
	// appears, text arrives in pieces, the bubble is final.
	EventMessageStarted   = "message.started"
	EventMessageDelta     = "message.delta"
	EventMessageCompleted = "message.completed"
	// EventActivity: what an agent is doing in a conversation — thinking,
	// working at something it names, or nothing.
	EventActivity = "activity"
)

// MessageDeltaEvent is one piece of a streaming message's text.
type MessageDeltaEvent struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	MessageID      uuid.UUID `json:"message_id"`
	Text           string    `json:"text"`
}

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
// Attachments are further fields of this same object, which is why it is a
// document and not a column. Buttons and quick replies are what an agent
// offers; Action is what a person's tap produced; SelectedButtonID records
// on the offering message which button was taken.
type Body struct {
	Text             string       `json:"text,omitempty"`
	Attachments      []Attachment `json:"attachments,omitempty"`
	Buttons          [][]Button   `json:"buttons,omitempty"`
	QuickReplies     []QuickReply `json:"quick_replies,omitempty"`
	Action           *Action      `json:"action,omitempty"`
	SelectedButtonID string       `json:"selected_button_id,omitempty"`
}

// Button is something a person can tap. A button has an id or a url, never
// both, and the two do different things.
//
// An id button is a choice: the backend picks the id and gets it back
// exactly, which is the whole point — something it can match rather than
// text it has to interpret.
//
// A url button leaves the app and tells the backend nothing. It is how an
// agent hands someone to a page it already has: a tracking link, a payment,
// a phone number, or its own site to sign in to when a poster's code could
// not say who scanned it.
type Button struct {
	ID    string `json:"id,omitempty"`
	URL   string `json:"url,omitempty"`
	Label string `json:"label"`
	Style string `json:"style,omitempty"`
}

// Button styles.
const (
	ButtonDefault = "default"
	ButtonPrimary = "primary"
	ButtonDanger  = "danger"
)

// QuickReply is a suggested answer, sent as plain text when tapped.
type QuickReply struct {
	Label string `json:"label"`
}

// Action is what a tap produced: which button, on which message.
type Action struct {
	ButtonID        string    `json:"button_id"`
	SourceMessageID uuid.UUID `json:"source_message_id"`
}

// ReplyRef names the message a message answers, with enough of it to draw
// the quote. The preview is read from the original at read time, never
// copied, so it is always the original as it stands.
type ReplyRef struct {
	ID          uuid.UUID
	SenderKind  ParticipantKind
	TextPreview string
}

// MessageStatus says whether a message is still being written.
type MessageStatus string

const (
	// MessageStreaming: an agent has started the message and is appending to
	// it. Its text so far lives in the stream buffer, not the row.
	MessageStreaming MessageStatus = "streaming"
	MessageComplete  MessageStatus = "complete"
)

// Message is one thing said in a conversation. Truncated marks a stream the
// hub finished rather than the agent; Stopped says the person asked it to.
type Message struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Sender         Sender
	Body           Body
	ReplyTo        *ReplyRef
	Status         MessageStatus
	Truncated      bool
	Stopped        bool
	// ScheduleID is the schedule the agent sent it for, and ScheduleTitle
	// its name, read when the message is.
	ScheduleID     *uuid.UUID
	ScheduleTitle  string
	DeliveryStatus DeliveryStatus
	CreatedAt      time.Time
	// Signature is the hub's mark over the content (see signature.go). Empty
	// while a stream is still being written, and on messages that predate
	// signing.
	Signature []byte
}

// SendInput is what a sender supplies. IdempotencyKey is optional: a sender
// that retries after a lost response sends the same key and gets the same
// message back instead of a duplicate. Buttons and QuickReplies are for
// agents; Action is for a person tapping a button, instead of text.
type SendInput struct {
	Text string
	// Attachments are files already uploaded by this sender, in the order
	// they should appear.
	Attachments    []uuid.UUID
	IdempotencyKey string
	ReplyTo        *uuid.UUID
	Buttons        [][]Button
	QuickReplies   []QuickReply
	Action         *Action
	// ScheduleID says a schedule of the agent's sent this. Agents only.
	ScheduleID *uuid.UUID
}

// FinishInput is what an agent may add when it finishes a stream. Buttons
// only make sense on a message that is final.
type FinishInput struct {
	Buttons      [][]Button
	QuickReplies []QuickReply
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
	// ConversationStarted is when the conversation was created. With
	// NextBefore empty it tells a caller whether history ended because the
	// conversation began there or because older messages have expired.
	ConversationStarted time.Time
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
		p.Status = derefString(r.AgentStatus)
		p.HasAvatar = r.AgentHasAvatar
		p.SupportsSchedules = r.AgentSupportsSchedules
		if len(r.AgentStarters) > 0 {
			_ = json.Unmarshal(r.AgentStarters, &p.Starters)
		}
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

	msg := Message{
		ID:             r.ID,
		ConversationID: r.ConversationID,
		Sender:         sender,
		Body:           body,
		Status:         MessageStatus(r.Status),
		Truncated:      r.Truncated,
		Stopped:        r.Stopped,
		ScheduleID:     r.ScheduleID,
		CreatedAt:      r.CreatedAt,
		Signature:      r.Signature,
	}
	if r.ReplyToMessageID != nil {
		msg.ReplyTo = &ReplyRef{ID: *r.ReplyToMessageID}
	}
	return msg, nil
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
