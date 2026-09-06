package pairing

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// What a person decides about an agent in their list. None of it reaches
// the agent (D42).

// contact fetches the caller's row for an agent, or says it is not there.
func (s *Service) contact(ctx context.Context, callerID, agentID uuid.UUID) (gen.Contact, error) {
	c, err := s.store.GetContact(ctx, gen.GetContactParams{UserID: callerID, AgentID: agentID})
	if err != nil {
		if store.IsNoRows(err) {
			return gen.Contact{}, domain.NotFound("not_a_contact", "That agent is not in your list.")
		}
		return gen.Contact{}, domain.Internal(fmt.Errorf("get contact: %w", err))
	}
	if c.RemovedAt != nil {
		return gen.Contact{}, domain.NotFound("not_a_contact", "That agent is not in your list.")
	}
	return c, nil
}

// SetMuted silences an agent until a time, or nil to hear it again. The
// agent is not told: muting is the person's business, and messages still
// arrive to be read whenever the chat is opened.
func (s *Service) SetMuted(ctx context.Context, callerID, agentID uuid.UUID, until *time.Time) error {
	if _, err := s.contact(ctx, callerID, agentID); err != nil {
		return err
	}
	if until != nil && !until.After(time.Now()) {
		return domain.InvalidField("muted_until", "invalid_muted_until", "muted_until must be in the future, or null.")
	}
	if _, err := s.store.SetContactMuted(ctx, gen.SetContactMutedParams{UserID: callerID, AgentID: agentID, MutedUntil: until}); err != nil {
		return domain.Internal(fmt.Errorf("mute %s: %w", agentID, err))
	}
	return nil
}

// SetPinned puts a chat above the others, or lets it back down. At most
// pinsMax at a time.
func (s *Service) SetPinned(ctx context.Context, callerID, agentID uuid.UUID, pinned bool) error {
	c, err := s.contact(ctx, callerID, agentID)
	if err != nil {
		return err
	}
	if pinned && c.PinnedAt == nil {
		n, err := s.store.CountPinnedContacts(ctx, callerID)
		if err != nil {
			return domain.Internal(fmt.Errorf("count pins: %w", err))
		}
		if n >= pinsMax {
			return domain.Conflict("too_many_pins", fmt.Sprintf("You can pin up to %d chats. Unpin one first.", pinsMax))
		}
	}
	if _, err := s.store.SetContactPinned(ctx, gen.SetContactPinnedParams{UserID: callerID, AgentID: agentID, Pinned: pinned}); err != nil {
		return domain.Internal(fmt.Errorf("pin %s: %w", agentID, err))
	}
	return nil
}

// SetArchived moves a chat out of the main list, or back into it.
func (s *Service) SetArchived(ctx context.Context, callerID, agentID uuid.UUID, archived bool) error {
	if _, err := s.contact(ctx, callerID, agentID); err != nil {
		return err
	}
	if _, err := s.store.SetContactArchived(ctx, gen.SetContactArchivedParams{UserID: callerID, AgentID: agentID, Archived: archived}); err != nil {
		return domain.Internal(fmt.Errorf("archive %s: %w", agentID, err))
	}
	return nil
}

// Remove takes an added agent out of the person's list. The chat is no
// longer listed but stays theirs to open; the agent is not told, and may
// still write — the person simply will not be looking. Scanning the code
// again brings it back. An owner cannot remove their own agent: that is
// what deleting it is for.
func (s *Service) Remove(ctx context.Context, callerID, agentID uuid.UUID) error {
	c, err := s.contact(ctx, callerID, agentID)
	if err != nil {
		return err
	}
	if c.AddedVia == "owner" {
		return domain.Conflict("own_agent", "You own this agent. Delete it instead of removing it.")
	}
	if _, err := s.store.RemoveContact(ctx, gen.RemoveContactParams{UserID: callerID, AgentID: agentID}); err != nil {
		return domain.Internal(fmt.Errorf("remove %s: %w", agentID, err))
	}
	return nil
}
