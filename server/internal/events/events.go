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

// Message is a message as a backend sees it. Status is "streaming" while
// an agent is still writing it and "complete" once it is final; Truncated
// marks a stream the hub had to cut off.
type Message struct {
	ID        string    `json:"id"`
	Sender    Sender    `json:"sender"`
	Body      Body      `json:"body"`
	ReplyTo   *ReplyTo  `json:"reply_to"`
	Status    string    `json:"status"`
	Truncated bool      `json:"truncated"`
	CreatedAt time.Time `json:"created_at"`
}

// Sender names who wrote a message. A backend never sees more about a person
// than this.
type Sender struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// Body is the message content. Buttons and QuickReplies are what an agent
// offered; Action is what a person's tap chose; SelectedButtonID, on the
// offering message, is the button that was taken.
type Body struct {
	Text             string       `json:"text,omitempty"`
	Attachments      []Attachment `json:"attachments,omitempty"`
	Buttons          [][]Button   `json:"buttons,omitempty"`
	QuickReplies     []QuickReply `json:"quick_replies,omitempty"`
	Action           *Action      `json:"action,omitempty"`
	SelectedButtonID string       `json:"selected_button_id,omitempty"`
}

// Attachment is a file a message carries. Its bytes are fetched from the
// media endpoint by media_id; everything needed to draw it before they
// arrive is here.
type Attachment struct {
	MediaID      string `json:"media_id"`
	Kind         string `json:"kind"`
	MimeType     string `json:"mime_type"`
	ByteSize     int64  `json:"byte_size"`
	FileName     string `json:"file_name"`
	Width        int32  `json:"width,omitempty"`
	Height       int32  `json:"height,omitempty"`
	HasThumbnail bool   `json:"has_thumbnail,omitempty"`
}

// Button is a choice offered to a person.
type Button struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Style string `json:"style"`
}

// QuickReply is a suggested answer.
type QuickReply struct {
	Label string `json:"label"`
}

// Action is a tap: which button, on which message.
type Action struct {
	ButtonID        string `json:"button_id"`
	SourceMessageID string `json:"source_message_id"`
}

// ReplyTo is the message a message answers, with enough to draw the quote.
type ReplyTo struct {
	ID          string `json:"id"`
	SenderKind  string `json:"sender_kind"`
	TextPreview string `json:"text_preview"`
}

// BodyOf is the wire form of a message body.
func BodyOf(b conversations.Body) Body {
	out := Body{Text: b.Text, SelectedButtonID: b.SelectedButtonID, Attachments: AttachmentsOf(b.Attachments)}
	for _, row := range b.Buttons {
		wire := make([]Button, 0, len(row))
		for _, btn := range row {
			wire = append(wire, Button{ID: btn.ID, Label: btn.Label, Style: btn.Style})
		}
		out.Buttons = append(out.Buttons, wire)
	}
	for _, q := range b.QuickReplies {
		out.QuickReplies = append(out.QuickReplies, QuickReply{Label: q.Label})
	}
	if b.Action != nil {
		out.Action = &Action{
			ButtonID:        b.Action.ButtonID,
			SourceMessageID: domain.FormatID(domain.PrefixMessage, b.Action.SourceMessageID),
		}
	}
	return out
}

// AttachmentsOf is the wire form of the files a message carries.
func AttachmentsOf(atts []conversations.Attachment) []Attachment {
	if len(atts) == 0 {
		return nil
	}
	out := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		out = append(out, Attachment{
			MediaID:      domain.FormatID(domain.PrefixMedia, a.MediaID),
			Kind:         a.Kind,
			MimeType:     a.MimeType,
			ByteSize:     a.ByteSize,
			FileName:     a.FileName,
			Width:        a.Width,
			Height:       a.Height,
			HasThumbnail: a.HasThumbnail,
		})
	}
	return out
}

// ReplyToOf is the wire form of a quote, or nil.
func ReplyToOf(r *conversations.ReplyRef) *ReplyTo {
	if r == nil {
		return nil
	}
	return &ReplyTo{
		ID:          domain.FormatID(domain.PrefixMessage, r.ID),
		SenderKind:  string(r.SenderKind),
		TextPreview: r.TextPreview,
	}
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
	return Envelope{
		ID:        domain.FormatID(domain.PrefixEvent, eventID),
		Type:      TypeMessageCreated,
		CreatedAt: createdAt,
		AgentID:   domain.FormatID(domain.PrefixAgent, agentID),
		Data: MessageCreated{
			Conversation: ConversationOf(conv),
			Message:      MessageOf(msg, SenderName(conv, msg.Sender)),
			Participants: ParticipantsOf(conv, agentID),
		},
	}
}

// ConversationOf is the wire form of a conversation.
func ConversationOf(conv conversations.Conversation) Conversation {
	return Conversation{
		ID:   domain.FormatID(domain.PrefixConv, conv.ID),
		Kind: string(conv.Kind),
	}
}

// ParticipantsOf is the wire form of a conversation's members as seen by
// the agent me.
func ParticipantsOf(conv conversations.Conversation, me uuid.UUID) []Participant {
	out := make([]Participant, 0, len(conv.Participants))
	for _, p := range conv.Participants {
		out = append(out, Participant{
			Kind:        string(p.Kind),
			ID:          formatParticipantID(p.Kind, p.ID),
			DisplayName: p.DisplayName,
			Handle:      p.Handle,
			IsMe:        p.Kind == conversations.ParticipantAgent && p.ID == me,
		})
	}
	return out
}

// SenderName is the current display name of a message's sender, found among
// the conversation's members. Empty if the sender is no longer one.
func SenderName(conv conversations.Conversation, sender conversations.Sender) string {
	for _, p := range conv.Participants {
		if p.Kind == sender.Kind && p.ID == sender.ID {
			return p.DisplayName
		}
	}
	return ""
}

// MessageOf is the wire form of a message. The same shape whether it
// arrives in an event, a history page, or as the answer to a send, so a
// backend has one parser.
func MessageOf(msg conversations.Message, senderName string) Message {
	return Message{
		ID: domain.FormatID(domain.PrefixMessage, msg.ID),
		Sender: Sender{
			Kind:        string(msg.Sender.Kind),
			ID:          formatParticipantID(msg.Sender.Kind, msg.Sender.ID),
			DisplayName: senderName,
		},
		Body:      BodyOf(msg.Body),
		ReplyTo:   ReplyToOf(msg.ReplyTo),
		Status:    string(msg.Status),
		Truncated: msg.Truncated,
		CreatedAt: msg.CreatedAt,
	}
}

func formatParticipantID(kind conversations.ParticipantKind, id uuid.UUID) string {
	if kind == conversations.ParticipantAgent {
		return domain.FormatID(domain.PrefixAgent, id)
	}
	return domain.FormatID(domain.PrefixUser, id)
}
