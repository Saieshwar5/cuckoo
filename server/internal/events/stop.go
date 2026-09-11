package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// TypeStopRequested is the event for a person pressing stop.
const TypeStopRequested = conversations.EventStopRequested

// StopRequested is the payload of TypeStopRequested: the person wants the
// agent to stop what it is doing in this conversation.
//
// By the time it arrives the hub has already done its part. MessageID names
// the reply it ended, if the agent had started writing one; further appends
// to it are refused with the code "stopped". Null means the agent was still
// thinking. What is left is the work behind the words — a model call, a
// search, a booking not yet made — and cancelling that is the backend's.
type StopRequested struct {
	Conversation Conversation `json:"conversation"`
	MessageID    *string      `json:"message_id"`
}

// NewStopRequested builds the event agentID receives when a person in conv
// presses stop.
func NewStopRequested(eventID uuid.UUID, createdAt time.Time, agentID uuid.UUID,
	conv conversations.Conversation, messageID *uuid.UUID) Envelope {
	data := StopRequested{Conversation: ConversationOf(conv)}
	if messageID != nil {
		id := domain.FormatID(domain.PrefixMessage, *messageID)
		data.MessageID = &id
	}
	return Envelope{
		ID:        domain.FormatID(domain.PrefixEvent, eventID),
		Type:      TypeStopRequested,
		CreatedAt: createdAt,
		AgentID:   domain.FormatID(domain.PrefixAgent, agentID),
		Data:      data,
	}
}
