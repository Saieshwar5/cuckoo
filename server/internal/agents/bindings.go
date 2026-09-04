package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// ActiveBinding returns the agent's live binding, or nil if it has none.
// Owner only.
func (s *Service) ActiveBinding(ctx context.Context, callerID, agentID uuid.UUID) (*Binding, error) {
	if _, err := s.GetOwned(ctx, callerID, agentID); err != nil {
		return nil, err
	}
	return s.activeBinding(ctx, s.store, agentID)
}

func (s *Service) activeBinding(ctx context.Context, st *store.Store, agentID uuid.UUID) (*Binding, error) {
	row, err := st.GetActiveBinding(ctx, agentID)
	if err != nil {
		if store.IsNoRows(err) {
			return nil, nil
		}
		return nil, domain.Internal(fmt.Errorf("get binding for %s: %w", agentID, err))
	}
	b := bindingFromRow(row)
	return &b, nil
}

// SetBinding connects an agent to a backend, replacing any existing binding.
//
// The plaintext secret is returned exactly once. It is not stored, so it cannot
// be shown again; a lost secret means setting a new binding. Revoking the old
// binding and creating the new one happen in one transaction, and the partial
// unique index on the table makes any other sequence impossible.
func (s *Service) SetBinding(ctx context.Context, callerID, agentID uuid.UUID, in SetBindingInput) (Binding, string, error) {
	if _, err := s.GetOwned(ctx, callerID, agentID); err != nil {
		return Binding{}, "", err
	}

	var webhookURL *string
	switch in.Mode {
	case ModeSocket:
		if strings.TrimSpace(in.WebhookURL) != "" {
			return Binding{}, "", domain.InvalidField("webhook_url", "invalid_webhook_url",
				"A socket binding does not take a URL.")
		}
	case ModeWebhook:
		u, err := validateWebhookURL(in.WebhookURL)
		if err != nil {
			return Binding{}, "", err
		}
		webhookURL = &u
	default:
		return Binding{}, "", domain.InvalidField("mode", "invalid_mode",
			`Mode must be "webhook" or "socket".`)
	}

	secret, hash := domain.NewSecret(domain.PrefixBindingSecret)

	var created gen.AgentBinding
	err := s.store.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.RevokeActiveBinding(ctx, agentID); err != nil {
			return domain.Internal(fmt.Errorf("revoke previous binding of %s: %w", agentID, err))
		}
		row, err := tx.CreateBinding(ctx, gen.CreateBindingParams{
			ID:         domain.NewID(),
			AgentID:    agentID,
			Mode:       string(in.Mode),
			WebhookUrl: webhookURL,
			SecretHash: hash,
		})
		if err != nil {
			return domain.Internal(fmt.Errorf("create binding for %s: %w", agentID, err))
		}
		created = row
		return nil
	})
	if err != nil {
		return Binding{}, "", err
	}

	return bindingFromRow(created), secret, nil
}

// RevokeBinding ends the agent's current binding. Owner only. The agent
// becomes idle; nothing else about it changes.
func (s *Service) RevokeBinding(ctx context.Context, callerID, agentID uuid.UUID) error {
	if _, err := s.GetOwned(ctx, callerID, agentID); err != nil {
		return err
	}

	n, err := s.store.RevokeActiveBinding(ctx, agentID)
	if err != nil {
		return domain.Internal(fmt.Errorf("revoke binding of %s: %w", agentID, err))
	}
	if n == 0 {
		return domain.NotFound("no_binding", "This agent has no backend connected.")
	}
	return nil
}

// ResolveSecret identifies the agent a bearer secret speaks for.
//
// This is the authentication path for the agent protocol, so it does the
// minimum: hash, one indexed lookup, done. The query already excludes revoked
// bindings and deleted agents.
func (s *Service) ResolveSecret(ctx context.Context, secret string) (uuid.UUID, error) {
	if !strings.HasPrefix(secret, domain.PrefixBindingSecret+"_") {
		return uuid.Nil, errBadSecret()
	}

	row, err := s.store.AuthenticateBinding(ctx, domain.HashSecret(secret))
	if err != nil {
		if store.IsNoRows(err) {
			return uuid.Nil, errBadSecret()
		}
		return uuid.Nil, domain.Internal(fmt.Errorf("authenticate binding: %w", err))
	}
	return row.AgentID, nil
}

func errBadSecret() error {
	return domain.Unauthorized("invalid_credentials",
		"That binding secret is not valid. It may have been revoked, or the agent removed.")
}
