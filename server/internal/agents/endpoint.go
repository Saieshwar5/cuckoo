package agents

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
)

// Endpoint is how to reach an agent's backend right now.
//
// SigningKey is what signs the requests the hub makes to a webhook backend.
// It is the SHA-256 of the binding secret: the very bytes stored to
// authenticate the backend, so the secret's plaintext is never at rest, and a
// backend derives the same key with one hash of the secret it was given. A
// 32-byte key gives HMAC-SHA256 its full strength.
type Endpoint struct {
	BindingID  uuid.UUID
	Mode       Mode
	WebhookURL string // set for ModeWebhook only
	SigningKey []byte
}

// ActiveEndpoint returns where the agent's events should go, or nil when no
// backend is connected. It has no caller check: it exists for the delivery
// worker, which acts for the hub, not for a request.
func (s *Service) ActiveEndpoint(ctx context.Context, agentID uuid.UUID) (*Endpoint, error) {
	row, err := s.store.GetActiveBinding(ctx, agentID)
	if err != nil {
		if store.IsNoRows(err) {
			return nil, nil
		}
		return nil, domain.Internal(fmt.Errorf("get binding of %s: %w", agentID, err))
	}
	ep := &Endpoint{BindingID: row.ID, Mode: Mode(row.Mode), SigningKey: row.SecretHash}
	if row.WebhookUrl != nil {
		ep.WebhookURL = *row.WebhookUrl
	}
	return ep, nil
}

// RecordDeliverySuccess notes that the backend behind a binding answered: it
// is connected, and any failure streak is over.
func (s *Service) RecordDeliverySuccess(ctx context.Context, bindingID uuid.UUID) error {
	if err := s.store.RecordBindingSuccess(ctx, bindingID); err != nil {
		return domain.Internal(fmt.Errorf("record success of binding %s: %w", bindingID, err))
	}
	return nil
}

// RecordDeliveryFailure notes that the backend behind a binding did not
// answer. Five minutes of unbroken failures makes the binding unreachable,
// which is what the app shows as a grey dot.
func (s *Service) RecordDeliveryFailure(ctx context.Context, bindingID uuid.UUID) error {
	if err := s.store.RecordBindingFailure(ctx, bindingID); err != nil {
		return domain.Internal(fmt.Errorf("record failure of binding %s: %w", bindingID, err))
	}
	return nil
}
