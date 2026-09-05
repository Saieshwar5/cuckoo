// Package events defines what an agent's backend receives: the envelope every
// event arrives in, and the payload of each event type.
//
// These are wire shapes and part of the public protocol. A backend written
// against them today must keep working, so fields are added and never
// renamed or removed.
package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// TypeMessageCreated is the event for a message posted in a conversation
// the agent belongs to. It is the one that matters.
const TypeMessageCreated = conversations.EventMessageCreated

// Envelope wraps every event. ID is what a backend acknowledges and
// de-duplicates on; retries carry the same one.
type Envelope struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	AgentID   string    `json:"agent_id"`
	Data      any       `json:"data"`
}

// MessageCreated is the payload of TypeMessageCreated.
type MessageCreated struct {
	Conversation Conversation  `json:"conversation"`
	Message      Message       `json:"message"`
	Participants []Participant `json:"participants"`
}

// Conversation is the little a backend needs to reply: where, and what kind.
type Conversation struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// Message is a message as a backend sees it.
type Message struct {
	ID        string    `json:"id"`
	Sender    Sender    `json:"sender"`
	Body      Body      `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Sender names who wrote a message. A backend never sees more about a person
// than this.
type Sender struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Body is the message content.
type Body struct {
	Text string `json:"text,omitempty"`
}

// Participant is a current member of the conversation. IsMe marks the
// receiving agent itself, so a backend serving several agents need not
// compare identifiers.
type Participant struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Handle      string `json:"handle,omitempty"`
	IsMe        bool   `json:"is_me"`
}

// NewMessageCreated builds the event agentID receives for msg in conv.
func NewMessageCreated(eventID uuid.UUID, createdAt time.Time, agentID uuid.UUID,
	msg conversations.Message, conv conversations.Conversation) Envelope {

	participants := make([]Participant, 0, len(conv.Participants))
	senderName := ""
	for _, p := range conv.Participants {
		if p.Kind == msg.Sender.Kind && p.ID == msg.Sender.ID {
			senderName = p.DisplayName
		}
		participants = append(participants, Participant{
			Kind:        string(p.Kind),
			ID:          formatParticipantID(p.Kind, p.ID),
			DisplayName: p.DisplayName,
			Handle:      p.Handle,
			IsMe:        p.Kind == conversations.ParticipantAgent && p.ID == agentID,
		})
	}

	return Envelope{
		ID:        domain.FormatID(domain.PrefixEvent, eventID),
		Type:      TypeMessageCreated,
		CreatedAt: createdAt,
		AgentID:   domain.FormatID(domain.PrefixAgent, agentID),
		Data: MessageCreated{
			Conversation: Conversation{
				ID:   domain.FormatID(domain.PrefixConv, conv.ID),
				Kind: string(conv.Kind),
			},
			Message: Message{
				ID: domain.FormatID(domain.PrefixMessage, msg.ID),
				Sender: Sender{
					Kind:        string(msg.Sender.Kind),
					ID:          formatParticipantID(msg.Sender.Kind, msg.Sender.ID),
					DisplayName: senderName,
				},
				Body:      Body{Text: msg.Body.Text},
				CreatedAt: msg.CreatedAt,
			},
			Participants: participants,
		},
	}
}

func formatParticipantID(kind conversations.ParticipantKind, id uuid.UUID) string {
	if kind == conversations.ParticipantAgent {
		return domain.FormatID(domain.PrefixAgent, id)
	}
	return domain.FormatID(domain.PrefixUser, id)
}
