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
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
		ID:          r.ID,
		OwnerID:     r.OwnerUserID,
		Handle:      r.Handle,
		DisplayName: r.DisplayName,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
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
}

// UpdateInput is a partial update; nil leaves a field alone. The handle is
// not here: it forms the agent's stable identifier and does not change.
type UpdateInput struct {
	DisplayName *string
	Description *string
}

// SetBindingInput describes the backend that will answer for an agent.
type SetBindingInput struct {
	Mode       Mode
	WebhookURL string // required for ModeWebhook, must be empty otherwise
}
