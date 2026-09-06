// Package pairing is how an agent reaches people who are not its owner.
//
// A pair token is what sits behind a QR code or a link. It names an agent,
// may carry a payload the owner wants handed to their backend, and may be
// limited to a number of uses or a time. Scanning it shows the agent's card;
// accepting adds the agent to the person's list, opens their chat with it,
// and tells the backend someone arrived. The token in the link is a secret:
// stored hashed, shown once.
//
// A contact is the result: this person has this agent, through this chat.
// A person may block the agent, which closes the chat in both directions
// until they add it again.
package pairing

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// KindAddAgent is the one kind of token so far: scanning adds the agent.
const KindAddAgent = "add_agent"

// Token is a pair token as its owner sees it. The plaintext is not here: it
// exists only in the response that created it.
type Token struct {
	ID        uuid.UUID
	AgentID   uuid.UUID
	Kind      string
	Payload   json.RawMessage
	MaxUses   *int
	UseCount  int
	ExpiresAt *time.Time
	CreatedAt time.Time
	RevokedAt *time.Time
}

func tokenFromRow(r gen.PairToken) Token {
	t := Token{
		ID:        r.ID,
		AgentID:   r.AgentID,
		Kind:      r.Kind,
		Payload:   r.Payload,
		UseCount:  int(r.UseCount),
		ExpiresAt: r.ExpiresAt,
		CreatedAt: r.CreatedAt,
		RevokedAt: r.RevokedAt,
	}
	if r.MaxUses.Valid {
		n := int(r.MaxUses.Int32)
		t.MaxUses = &n
	}
	return t
}

// CreateTokenInput describes a token to mint. A nil MaxUses is a poster: any
// number of people may scan it. A nil ExpiresIn never expires.
type CreateTokenInput struct {
	Payload   json.RawMessage
	MaxUses   *int
	ExpiresIn *time.Duration
}

// Card is what a person sees before deciding to add an agent: who it is,
// who owns it, and whether it is already theirs.
type Card struct {
	Agent     agents.Agent
	OwnerName string
	// Status is the binding's health, or nil with no backend connected.
	Status *agents.Status
	// AlreadyAdded, and the chat it opened, when the person has it.
	AlreadyAdded   bool
	ConversationID *uuid.UUID
	Blocked        bool
}

// Accepted is the outcome of adding an agent: the chat, and whether it is
// new to the person.
type Accepted struct {
	Conversation conversations.Conversation
	New          bool
}

// Contact is an agent in a person's list.
type Contact struct {
	AgentID        uuid.UUID
	Handle         string
	DisplayName    string
	Description    string
	HasAvatar      bool
	OwnerName      string
	Status         *agents.Status
	AddedVia       string
	Blocked        bool
	ConversationID uuid.UUID
	CreatedAt      time.Time
	AgentDeleted   bool
	// The person's own settings for this agent. MutedUntil in the future
	// means nothing about it may disturb them; far in the future means
	// always.
	MutedUntil *time.Time
	Pinned     bool
	Archived   bool
}

// pinsMax is how many chats may sit above the rest. Three is what a thumb
// reaches without scrolling; more pins and nothing is pinned.
const pinsMax = 3

func contactFromRow(r gen.ListContactsRow) Contact {
	c := Contact{
		AgentID:        r.AgentID,
		Handle:         r.Handle,
		DisplayName:    r.DisplayName,
		Description:    r.Description,
		HasAvatar:      r.HasAvatar,
		OwnerName:      r.OwnerDisplayName,
		AddedVia:       r.AddedVia,
		Blocked:        r.BlockedAt != nil,
		ConversationID: r.DmConversationID,
		CreatedAt:      r.CreatedAt,
		AgentDeleted:   r.AgentDeletedAt != nil,
		MutedUntil:     r.MutedUntil,
		Pinned:         r.PinnedAt != nil,
		Archived:       r.ArchivedAt != nil,
	}
	if r.BindingStatus != nil {
		s := agents.Status(*r.BindingStatus)
		c.Status = &s
	}
	return c
}
