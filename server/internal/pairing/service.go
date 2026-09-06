package pairing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/users"
)

// maxPayloadBytes bounds what an owner may hand to their backend per scan.
const maxPayloadBytes = 4096

// Service holds the rules for tokens and contacts.
type Service struct {
	store         *store.Store
	agents        *agents.Service
	conversations *conversations.Service
	users         *users.Service
	publicURL     string
}

// New builds the service. publicURL is the base of every link: where
// people reach this hub from outside.
func New(st *store.Store, agentService *agents.Service, conversationService *conversations.Service,
	userService *users.Service, publicURL string) *Service {
	return &Service{
		store: st, agents: agentService, conversations: conversationService, users: userService,
		publicURL: strings.TrimRight(publicURL, "/"),
	}
}

// URL is the link a token lives at: what a QR code encodes.
func (s *Service) URL(plaintext string) string {
	return s.publicURL + "/p/" + plaintext
}

// CreateToken mints a token for an agent. Owner only. The plaintext is
// returned once.
func (s *Service) CreateToken(ctx context.Context, callerID, agentID uuid.UUID, in CreateTokenInput) (Token, string, error) {
	if _, err := s.agents.GetOwned(ctx, callerID, agentID); err != nil {
		return Token{}, "", err
	}
	var payload []byte
	if len(in.Payload) > 0 && string(in.Payload) != "null" {
		if len(in.Payload) > maxPayloadBytes {
			return Token{}, "", domain.InvalidField("payload", "payload_too_large",
				fmt.Sprintf("The payload must be under %d bytes.", maxPayloadBytes))
		}
		if !json.Valid(in.Payload) {
			return Token{}, "", domain.InvalidField("payload", "invalid_payload", "The payload must be JSON.")
		}
		payload = in.Payload
	}
	var maxUses pgtype.Int4
	if in.MaxUses != nil {
		if *in.MaxUses < 1 || *in.MaxUses > 1_000_000 {
			return Token{}, "", domain.InvalidField("max_uses", "invalid_max_uses",
				"Max uses must be between 1 and 1,000,000.")
		}
		maxUses = pgtype.Int4{Int32: int32(*in.MaxUses), Valid: true} //nolint:gosec // bounded just above
	}
	var expiresAt *time.Time
	if in.ExpiresIn != nil {
		if *in.ExpiresIn <= 0 {
			return Token{}, "", domain.InvalidField("expires_in", "invalid_expires_in", "Expiry must be in the future.")
		}
		t := time.Now().Add(*in.ExpiresIn)
		expiresAt = &t
	}

	plaintext, hash := domain.NewSecret(domain.PrefixPairToken)
	row, err := s.store.CreatePairToken(ctx, gen.CreatePairTokenParams{
		ID:              domain.NewID(),
		TokenHash:       hash,
		Kind:            KindAddAgent,
		AgentID:         agentID,
		Payload:         payload,
		MaxUses:         maxUses,
		CreatedByUserID: callerID,
		ExpiresAt:       expiresAt,
	})
	if err != nil {
		return Token{}, "", domain.Internal(fmt.Errorf("create pair token for %s: %w", agentID, err))
	}
	return tokenFromRow(row), plaintext, nil
}

// ListTokens returns an agent's tokens, newest first, revoked ones included.
// Owner only.
func (s *Service) ListTokens(ctx context.Context, callerID, agentID uuid.UUID) ([]Token, error) {
	if _, err := s.agents.GetOwned(ctx, callerID, agentID); err != nil {
		return nil, err
	}
	rows, err := s.store.ListPairTokensByAgent(ctx, agentID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list pair tokens of %s: %w", agentID, err))
	}
	out := make([]Token, 0, len(rows))
	for _, r := range rows {
		out = append(out, tokenFromRow(r))
	}
	return out, nil
}

// RevokeToken closes a token to new scans. People who already added the
// agent keep it. Owner only.
func (s *Service) RevokeToken(ctx context.Context, callerID, agentID, tokenID uuid.UUID) error {
	if _, err := s.agents.GetOwned(ctx, callerID, agentID); err != nil {
		return err
	}
	n, err := s.store.RevokePairToken(ctx, gen.RevokePairTokenParams{ID: tokenID, AgentID: agentID})
	if err != nil {
		return domain.Internal(fmt.Errorf("revoke pair token %s: %w", tokenID, err))
	}
	if n == 0 {
		return domain.NotFound("token_not_found", "No such live token.")
	}
	return nil
}

// Picture confirms a code belongs to one of the owner's tokens and returns
// the link it encodes, so a device that kept the code can have its QR
// drawn again. The hub keeps only the hash, so it can check but never
// reveal a code.
func (s *Service) Picture(ctx context.Context, callerID, agentID, tokenID uuid.UUID, plaintext string) (string, error) {
	if _, err := s.agents.GetOwned(ctx, callerID, agentID); err != nil {
		return "", err
	}
	row, err := s.store.GetPairToken(ctx, gen.GetPairTokenParams{ID: tokenID, AgentID: agentID})
	if err != nil {
		if store.IsNoRows(err) {
			return "", domain.NotFound("token_not_found", "No such token.")
		}
		return "", domain.Internal(fmt.Errorf("get pair token %s: %w", tokenID, err))
	}
	if !bytes.Equal(row.TokenHash, domain.HashSecret(plaintext)) {
		return "", domain.NotFound("token_not_found", "That code does not belong to this token.")
	}
	return s.URL(plaintext), nil
}

// lookup resolves the token in a link and checks it is still good for
// someone who already has the agent: it exists, its agent exists, and it
// has not been withdrawn or expired. Whether it has a use left is a
// separate question, asked only for someone new.
func (s *Service) lookup(ctx context.Context, plaintext string) (gen.GetPairTokenByHashRow, error) {
	if !strings.HasPrefix(plaintext, domain.PrefixPairToken+"_") {
		return gen.GetPairTokenByHashRow{}, errTokenInvalid()
	}
	row, err := s.store.GetPairTokenByHash(ctx, domain.HashSecret(plaintext))
	if err != nil {
		if store.IsNoRows(err) {
			return gen.GetPairTokenByHashRow{}, errTokenInvalid()
		}
		return gen.GetPairTokenByHashRow{}, domain.Internal(fmt.Errorf("look up pair token: %w", err))
	}
	switch {
	case row.AgentDeletedAt != nil:
		return gen.GetPairTokenByHashRow{}, errTokenInvalid()
	case row.RevokedAt != nil:
		return gen.GetPairTokenByHashRow{}, domain.NotFound("token_revoked", "This link has been withdrawn.")
	case row.ExpiresAt != nil && row.ExpiresAt.Before(time.Now()):
		return gen.GetPairTokenByHashRow{}, domain.NotFound("token_expired", "This link has expired.")
	}
	return row, nil
}

func spent(row gen.GetPairTokenByHashRow) bool {
	return row.MaxUses.Valid && row.UseCount >= row.MaxUses.Int32
}

func errTokenSpent() error {
	return domain.NotFound("token_spent", "This link has already been used.")
}

// Resolve shows what a link is for, without acting on it. callerID may be
// uuid.Nil for a caller without an account, as on the link page; then the
// card says nothing about whether it is already theirs.
func (s *Service) Resolve(ctx context.Context, callerID uuid.UUID, plaintext string) (Card, error) {
	tok, err := s.lookup(ctx, plaintext)
	if err != nil {
		return Card{}, err
	}
	card, err := s.card(ctx, callerID, tok.AgentID)
	if err != nil {
		return Card{}, err
	}
	if !card.AlreadyAdded && spent(tok) {
		return Card{}, errTokenSpent()
	}
	return card, nil
}

func (s *Service) card(ctx context.Context, callerID, agentID uuid.UUID) (Card, error) {
	agent, err := s.agents.Get(ctx, agentID)
	if err != nil {
		return Card{}, err
	}
	owner, err := s.users.Get(ctx, agent.OwnerID)
	if err != nil {
		return Card{}, err
	}
	card := Card{Agent: agent, OwnerName: owner.DisplayName}
	if b, err := s.store.GetActiveBinding(ctx, agentID); err == nil {
		st := agents.Status(b.Status)
		card.Status = &st
	} else if !store.IsNoRows(err) {
		return Card{}, domain.Internal(fmt.Errorf("get binding of %s: %w", agentID, err))
	}
	if callerID == uuid.Nil {
		return card, nil
	}
	contact, err := s.store.GetContact(ctx, gen.GetContactParams{UserID: callerID, AgentID: agentID})
	// A contact the person removed reads as not added: the card offers Add
	// again, and accepting restores it.
	if err == nil && contact.RemovedAt != nil {
		return card, nil
	}
	if err == nil {
		card.AlreadyAdded = true
		id := contact.DmConversationID
		card.ConversationID = &id
		card.Blocked = contact.BlockedAt != nil
	} else if !store.IsNoRows(err) {
		return Card{}, domain.Internal(fmt.Errorf("get contact: %w", err))
	}
	return card, nil
}

// Accept adds the agent behind a link to the caller's list and opens their
// chat, telling the backend someone arrived. Accepting a link for an agent
// the person already has costs the token nothing and reopens the chat if
// they had blocked it.
func (s *Service) Accept(ctx context.Context, callerID uuid.UUID, plaintext string) (Accepted, error) {
	tok, err := s.lookup(ctx, plaintext)
	if err != nil {
		return Accepted{}, err
	}

	var (
		convID  uuid.UUID
		isNew   bool
		pending bool
	)
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		convs := conversations.New(tx)
		existing, err := tx.GetContact(ctx, gen.GetContactParams{UserID: callerID, AgentID: tok.AgentID})
		if err == nil {
			convID = existing.DmConversationID
			// Someone who removed the agent and scans it again gets it
			// back, clean. The agent was never told they left, so it is
			// not told they returned.
			if existing.RemovedAt != nil {
				if _, err := tx.RestoreContact(ctx, gen.RestoreContactParams{UserID: callerID, AgentID: tok.AgentID}); err != nil {
					return domain.Internal(fmt.Errorf("restore contact: %w", err))
				}
			}
			if existing.BlockedAt != nil {
				if _, err := tx.ClearContactBlocked(ctx, gen.ClearContactBlockedParams{UserID: callerID, AgentID: tok.AgentID}); err != nil {
					return domain.Internal(fmt.Errorf("unblock: %w", err))
				}
				pending, err = convs.Enqueue(ctx, tok.AgentID, convID, conversations.EventConversationJoined,
					conversations.JoinedPayload{})
				return err
			}
			return nil
		}
		if !store.IsNoRows(err) {
			return domain.Internal(fmt.Errorf("get contact: %w", err))
		}

		// The token gives up a use only for someone new.
		if _, err := tx.UsePairToken(ctx, tok.ID); err != nil {
			if store.IsNoRows(err) {
				return errTokenSpent()
			}
			return domain.Internal(fmt.Errorf("use pair token %s: %w", tok.ID, err))
		}
		conv, _, err := convs.FindOrCreateDM(ctx, callerID, tok.AgentID)
		if err != nil {
			return err
		}
		convID = conv.ID
		tokenID := tok.ID
		if _, err := tx.CreateContact(ctx, gen.CreateContactParams{
			UserID: callerID, AgentID: tok.AgentID, DmConversationID: conv.ID,
			AddedVia: "pair_token", PairTokenID: &tokenID,
		}); err != nil {
			return domain.Internal(fmt.Errorf("create contact: %w", err))
		}
		isNew = true
		pending, err = convs.Enqueue(ctx, tok.AgentID, conv.ID, conversations.EventConversationJoined,
			conversations.JoinedPayload{PairTokenID: &tokenID, Payload: tok.Payload})
		return err
	})
	if err != nil {
		return Accepted{}, err
	}
	if pending {
		s.conversations.Nudge(ctx, []uuid.UUID{tok.AgentID})
	}

	convs, err := s.conversations.GetConversations(ctx, []uuid.UUID{convID})
	if err != nil {
		return Accepted{}, err
	}
	if len(convs) == 0 {
		return Accepted{}, domain.Internal(fmt.Errorf("conversation %s vanished", convID))
	}
	return Accepted{Conversation: convs[0], New: isNew}, nil
}

// Contacts lists the agents in a person's list, newest first.
func (s *Service) Contacts(ctx context.Context, callerID uuid.UUID) ([]Contact, error) {
	rows, err := s.store.ListContacts(ctx, callerID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list contacts of %s: %w", callerID, err))
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		out = append(out, contactFromRow(r))
	}
	return out, nil
}

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
	if c.AddedVia != "pair_token" {
		return domain.Conflict("own_agent", "You own this agent. Delete it instead of removing it.")
	}
	if _, err := s.store.RemoveContact(ctx, gen.RemoveContactParams{UserID: callerID, AgentID: agentID}); err != nil {
		return domain.Internal(fmt.Errorf("remove %s: %w", agentID, err))
	}
	return nil
}

// Block closes the person's chat with an agent in both directions and tells
// the backend it has been left. Blocking again changes nothing.
func (s *Service) Block(ctx context.Context, callerID, agentID uuid.UUID) error {
	contact, err := s.store.GetContact(ctx, gen.GetContactParams{UserID: callerID, AgentID: agentID})
	if err != nil {
		if store.IsNoRows(err) {
			return domain.NotFound("not_a_contact", "That agent is not in your list.")
		}
		return domain.Internal(fmt.Errorf("get contact: %w", err))
	}
	if contact.BlockedAt != nil {
		return nil
	}
	var pending bool
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.SetContactBlocked(ctx, gen.SetContactBlockedParams{UserID: callerID, AgentID: agentID}); err != nil {
			return domain.Internal(fmt.Errorf("block %s: %w", agentID, err))
		}
		var err error
		pending, err = conversations.New(tx).Enqueue(ctx, agentID, contact.DmConversationID,
			conversations.EventConversationLeft, conversations.LeftPayload{Reason: "user_blocked"})
		return err
	})
	if err != nil {
		return err
	}
	if pending {
		s.conversations.Nudge(ctx, []uuid.UUID{agentID})
	}
	return nil
}

// Unblock reopens the chat and tells the backend the person is back.
func (s *Service) Unblock(ctx context.Context, callerID, agentID uuid.UUID) error {
	contact, err := s.store.GetContact(ctx, gen.GetContactParams{UserID: callerID, AgentID: agentID})
	if err != nil {
		if store.IsNoRows(err) {
			return domain.NotFound("not_a_contact", "That agent is not in your list.")
		}
		return domain.Internal(fmt.Errorf("get contact: %w", err))
	}
	if contact.BlockedAt == nil {
		return nil
	}
	var pending bool
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.ClearContactBlocked(ctx, gen.ClearContactBlockedParams{UserID: callerID, AgentID: agentID}); err != nil {
			return domain.Internal(fmt.Errorf("unblock %s: %w", agentID, err))
		}
		var err error
		pending, err = conversations.New(tx).Enqueue(ctx, agentID, contact.DmConversationID,
			conversations.EventConversationJoined, conversations.JoinedPayload{})
		return err
	})
	if err != nil {
		return err
	}
	if pending {
		s.conversations.Nudge(ctx, []uuid.UUID{agentID})
	}
	return nil
}

func errTokenInvalid() error {
	return domain.NotFound("invalid_token", "This link is not valid.")
}
