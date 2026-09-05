package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// Membership events: the agent was added to a conversation, or a person
// closed one. Neither is about a message.
const (
	TypeConversationJoined = conversations.EventConversationJoined
	TypeConversationLeft   = conversations.EventConversationLeft
)

// ConversationJoined is the payload of TypeConversationJoined: a new chat
// exists, with these people in it. PairToken is set when someone arrived
// by scanning a code, with whatever the owner put in that code; a
// personalised code is how a backend knows who it is talking to before
// they say a word.
type ConversationJoined struct {
	Conversation Conversation  `json:"conversation"`
	Participants []Participant `json:"participants"`
	PairToken    *PairTokenRef `json:"pair_token"`
}

// PairTokenRef names the code someone scanned and carries its payload.
type PairTokenRef struct {
	ID      string          `json:"id"`
	Payload json.RawMessage `json:"payload"`
}

// ConversationLeft is the payload of TypeConversationLeft. Sends into the
// conversation are refused from now on.
type ConversationLeft struct {
	Conversation Conversation `json:"conversation"`
	Reason       string       `json:"reason"`
}

// NewConversationJoined builds the event agentID receives on joining conv.
func NewConversationJoined(eventID uuid.UUID, createdAt time.Time, agentID uuid.UUID,
	conv conversations.Conversation, p conversations.JoinedPayload) Envelope {
	data := ConversationJoined{
		Conversation: ConversationOf(conv),
		Participants: ParticipantsOf(conv, agentID),
	}
	if p.PairTokenID != nil {
		payload := p.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("null")
		}
		data.PairToken = &PairTokenRef{ID: domain.FormatID(domain.PrefixToken, *p.PairTokenID), Payload: payload}
	}
	return Envelope{
		ID:        domain.FormatID(domain.PrefixEvent, eventID),
		Type:      TypeConversationJoined,
		CreatedAt: createdAt,
		AgentID:   domain.FormatID(domain.PrefixAgent, agentID),
		Data:      data,
	}
}

// NewConversationLeft builds the event agentID receives when conv closes
// to it.
func NewConversationLeft(eventID uuid.UUID, createdAt time.Time, agentID uuid.UUID,
	conv conversations.Conversation, reason string) Envelope {
	return Envelope{
		ID:        domain.FormatID(domain.PrefixEvent, eventID),
		Type:      TypeConversationLeft,
		CreatedAt: createdAt,
		AgentID:   domain.FormatID(domain.PrefixAgent, agentID),
		Data:      ConversationLeft{Conversation: ConversationOf(conv), Reason: reason},
	}
}
