// Package apikeys holds the credential a company's own systems call the
// management API with.
//
// A person signs in once, creates a key in the app, and pastes it into
// their deployment. From then on their servers create agents, connect
// backends and mint the codes that hand agents out, with no phone in the
// loop — which is the only way a company runs fifty bots, or mints a code
// per customer from its website.
//
// A key acts as its owner and can do exactly what its owner can do in the
// management API: no more, so a leaked key is not a wider hole than a
// stolen phone; no less, so nothing forces a person back to the app. It is
// refused everywhere else — it cannot read a conversation, cannot speak
// for an agent, and cannot create another key.
package apikeys

import (
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Key is an API key as its owner sees it. The key itself is not here: it
// exists in memory only at the moment of creation.
type Key struct {
	ID         uuid.UUID
	Name       string
	LastUsedAt *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
}

func fromRow(r gen.ApiKey) Key {
	return Key{
		ID:         r.ID,
		Name:       r.Name,
		LastUsedAt: r.LastUsedAt,
		CreatedAt:  r.CreatedAt,
		RevokedAt:  r.RevokedAt,
	}
}

// maxNameLen matches the column's constraint.
const maxNameLen = 80
