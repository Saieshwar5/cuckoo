// Package agents owns agent identities and the bindings that connect them to
// backends.
//
// An Agent is the thing in a chat list — a name, a description, an owner. It is
// permanent, and everything that accumulates around it — its conversations —
// belongs to it. A Binding is how a backend proves it speaks for an agent. It
// is replaceable: revoking one, or swapping one for another, changes nothing
// about the agent or its history. Keeping them separate is what lets a company
// withdraw its backend while the user keeps their chat, and lets a user
// re-point their own agent at a different provider.
package agents

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Mode is how a backend receives events for an agent.
type Mode string

const (
	// ModeWebhook: the hub calls a URL the backend published.
	ModeWebhook Mode = "webhook"
	// ModeSocket: the backend connects to the hub and holds the connection.
	// Works from anywhere, including a laptop with no public address.
	ModeSocket Mode = "socket"
)

// Status is a binding's operational health. Revocation is not a status; it is
// a separate, permanent fact recorded on the binding.
type Status string

const (
	StatusIdle        Status = "idle"
	StatusConnected   Status = "connected"
	StatusUnreachable Status = "unreachable"
)

// Agent is an identity a person or a company has created.
type Agent struct {
	ID          uuid.UUID
	OwnerID     uuid.UUID
	Handle      string
	DisplayName string
	Description string
	// The picture it is published with, or nil for the initials disc.
	AvatarMediaID *uuid.UUID
	// A few things it suggests saying first, shown as chips in an empty
	// chat. Empty for most agents; a company's agent should have them.
	Starters []string
	// Private means the agent may not be handed out at all: no code can be
	// minted for it, and the app hides Share. What a person's own agent is,
	// once something is running behind it that reads their mail (D51).
	Private bool
	// Listed means it belongs in the catalogue the app shows. Separate from
	// having a code, because handing out a poster and wanting to appear in a
	// directory are different wishes.
	Listed bool
	// SupportsSchedules means its backend can hold schedules — things a
	// person asks it to do at a time — and the app offers them only then.
	SupportsSchedules bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Binding connects an agent to a backend. The secret is never part of this
// type: it exists in memory only at the moment of creation.
type Binding struct {
	ID         uuid.UUID
	AgentID    uuid.UUID
	Mode       Mode
	WebhookURL *string
	Status     Status
	LastSeenAt *time.Time
	CreatedAt  time.Time
}

func agentFromRow(r gen.Agent) Agent {
	return Agent{
		ID:                r.ID,
		OwnerID:           r.OwnerUserID,
		Handle:            r.Handle,
		DisplayName:       r.DisplayName,
		Description:       r.Description,
		AvatarMediaID:     r.AvatarMediaID,
		Starters:          startersFromRow(r.Starters),
		Private:           r.Private,
		Listed:            r.Listed,
		SupportsSchedules: r.SupportsSchedules,
		CreatedAt:         r.CreatedAt,
		UpdatedAt:         r.UpdatedAt,
	}
}

// startersFromRow decodes the stored list. A row that will not decode is
// treated as having none rather than failing every read of the agent.
func startersFromRow(raw []byte) []string {
	var out []string
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil || out == nil {
		return []string{}
	}
	return out
}

func bindingFromRow(r gen.AgentBinding) Binding {
	return Binding{
		ID:         r.ID,
		AgentID:    r.AgentID,
		Mode:       Mode(r.Mode),
		WebhookURL: r.WebhookUrl,
		Status:     Status(r.Status),
		LastSeenAt: r.LastSeenAt,
		CreatedAt:  r.CreatedAt,
	}
}

// CreateInput is what it takes to bring an agent into existence.
type CreateInput struct {
	Handle      string
	DisplayName string
	Description string
	// A picture already uploaded by the owner. A company setting its logo
	// in the same call that creates the agent.
	AvatarMediaID *uuid.UUID
	// Up to four things to suggest saying first.
	Starters []string
	// Private refuses to hand the agent out at all (D51); Listed offers it in
	// the app's catalogue. An agent cannot be both.
	Private bool
	Listed  bool
	// SupportsSchedules says its backend can hold schedules.
	SupportsSchedules bool
}

// UpdateInput is a partial update; nil leaves a field alone. The handle is
// not here: it forms the agent's stable identifier and does not change.
type UpdateInput struct {
	DisplayName   *string
	Description   *string
	AvatarMediaID *uuid.UUID
	// Starters replaces the whole list when set; nil leaves it.
	Starters          *[]string
	Private           *bool
	Listed            *bool
	SupportsSchedules *bool
}

// SetBindingInput describes the backend that will answer for an agent.
type SetBindingInput struct {
	Mode       Mode
	WebhookURL string // required for ModeWebhook, must be empty otherwise
}
