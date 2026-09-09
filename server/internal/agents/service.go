package agents

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/media"
	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Service holds the rules for agents and their bindings.
//
// It takes the concrete store rather than an interface because several of its
// operations are transactions — revoke the old binding and create the new one,
// or retire an agent and every binding with it — and a transaction needs the
// real thing. Packages without transactions may keep naming their queries in an
// interface, as users does.
type Service struct {
	store     *store.Store
	publisher realtime.Publisher
}

// Option configures a Service.
type Option func(*Service)

// WithPublisher sets where status changes are announced. Without one, they
// are recorded and nobody is told.
func WithPublisher(p realtime.Publisher) Option {
	return func(s *Service) { s.publisher = p }
}

// New builds the service.
func New(st *store.Store, opts ...Option) *Service {
	s := &Service{store: st, publisher: realtime.Discard{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Create brings an agent into existence, owned by the caller, and opens the
// DM between them: the chat the owner will find the agent in.
func (s *Service) Create(ctx context.Context, ownerID uuid.UUID, in CreateInput) (Agent, error) {
	handle, err := validateHandle(in.Handle)
	if err != nil {
		return Agent{}, err
	}
	name, err := validateDisplayName(in.DisplayName)
	if err != nil {
		return Agent{}, err
	}
	desc, err := validateDescription(in.Description)
	if err != nil {
		return Agent{}, err
	}
	starters, err := validateStarters(in.Starters)
	if err != nil {
		return Agent{}, err
	}
	if err := s.checkPicture(ctx, ownerID, in.AvatarMediaID); err != nil {
		return Agent{}, err
	}

	// One transaction for the agent and its DM, so neither exists without the
	// other. It also contains a duplicate handle: that is a constraint
	// violation, and Postgres aborts the surrounding transaction on any error,
	// so scoping the insert to a savepoint lets whatever called us — a
	// request, or a test — carry on.
	var row gen.Agent
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		var err error
		row, err = tx.CreateAgent(ctx, gen.CreateAgentParams{
			ID:            domain.NewID(),
			OwnerUserID:   ownerID,
			Handle:        handle,
			DisplayName:   name,
			Description:   desc,
			AvatarMediaID: in.AvatarMediaID,
			Starters:      startersJSON(starters),
			Private:       in.Private,
			Listed:        in.Listed,
		})
		if err != nil {
			return err
		}
		dm, err := conversations.New(tx).CreateDM(ctx, ownerID, row.ID)
		if err != nil {
			return err
		}
		_, err = tx.CreateContact(ctx, gen.CreateContactParams{
			UserID: ownerID, AgentID: row.ID, DmConversationID: dm.ID, AddedVia: "owner",
		})
		return err
	})
	if err != nil {
		if _, classified := domain.AsError(err); classified {
			return Agent{}, err
		}
		if store.IsUniqueViolation(err) {
			return Agent{}, domain.Conflict("handle_taken",
				fmt.Sprintf("The handle %q is already in use.", handle))
		}
		return Agent{}, domain.Internal(fmt.Errorf("create agent: %w", err))
	}
	return agentFromRow(row), nil
}

// Get returns an agent by identifier with no ownership check.
//
// For callers that are the agent itself, or that have some other right to see
// it. Management operations use GetOwned.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Agent, error) {
	row, err := s.store.GetAgent(ctx, id)
	if err != nil {
		if store.IsNoRows(err) {
			return Agent{}, errAgentNotFound()
		}
		return Agent{}, domain.Internal(fmt.Errorf("get agent %s: %w", id, err))
	}
	return agentFromRow(row), nil
}

// GetOwned returns an agent only if the caller owns it.
//
// A missing agent is not-found; an existing agent owned by someone else is
// forbidden. The two are distinguished on purpose: identifiers are random and
// unguessable, so revealing that one exists costs nothing, and a developer
// who has pasted an id from the wrong account deserves to be told so.
func (s *Service) GetOwned(ctx context.Context, callerID, id uuid.UUID) (Agent, error) {
	agent, err := s.Get(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	if agent.OwnerID != callerID {
		return Agent{}, errNotOwner()
	}
	return agent, nil
}

// ListMine returns every live agent the caller owns, oldest first.
func (s *Service) ListMine(ctx context.Context, callerID uuid.UUID) ([]Agent, error) {
	rows, err := s.store.ListAgentsByOwner(ctx, callerID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list agents: %w", err))
	}
	out := make([]Agent, 0, len(rows))
	for _, r := range rows {
		out = append(out, agentFromRow(r))
	}
	return out, nil
}

// Update changes the fields supplied and leaves the rest alone. Owner only.
func (s *Service) Update(ctx context.Context, callerID, id uuid.UUID, in UpdateInput) (Agent, error) {
	if _, err := s.GetOwned(ctx, callerID, id); err != nil {
		return Agent{}, err
	}

	if err := s.checkPicture(ctx, callerID, in.AvatarMediaID); err != nil {
		return Agent{}, err
	}

	var starters []byte
	if in.Starters != nil {
		list, err := validateStarters(*in.Starters)
		if err != nil {
			return Agent{}, err
		}
		starters = startersJSON(list)
	}
	params := gen.UpdateAgentParams{
		Starters: starters, ID: id, AvatarMediaID: in.AvatarMediaID,
		Private: optionalBool(in.Private), Listed: optionalBool(in.Listed)}
	if in.DisplayName != nil {
		name, err := validateDisplayName(*in.DisplayName)
		if err != nil {
			return Agent{}, err
		}
		params.DisplayName = &name
	}
	if in.Description != nil {
		desc, err := validateDescription(*in.Description)
		if err != nil {
			return Agent{}, err
		}
		params.Description = &desc
	}
	if params.DisplayName == nil && params.Description == nil && params.AvatarMediaID == nil && params.Starters == nil {
		return Agent{}, domain.Invalid("no_changes", "Provide at least one field to update.")
	}

	row, err := s.store.UpdateAgent(ctx, params)
	if err != nil {
		if store.IsNoRows(err) {
			return Agent{}, errAgentNotFound()
		}
		return Agent{}, domain.Internal(fmt.Errorf("update agent %s: %w", id, err))
	}
	return agentFromRow(row), nil
}

// Delete retires an agent and revokes its binding, atomically. Owner only.
//
// The row is kept: conversations will refer to it, and the people who talked to
// it must still be able to read what was said.
func (s *Service) Delete(ctx context.Context, callerID, id uuid.UUID) error {
	if _, err := s.GetOwned(ctx, callerID, id); err != nil {
		return err
	}

	err := s.store.WithTx(ctx, func(tx *store.Store) error {
		if _, err := tx.RevokeActiveBinding(ctx, id); err != nil {
			return domain.Internal(fmt.Errorf("revoke bindings of %s: %w", id, err))
		}
		n, err := tx.SoftDeleteAgent(ctx, id)
		if err != nil {
			return domain.Internal(fmt.Errorf("delete agent %s: %w", id, err))
		}
		if n == 0 {
			return errAgentNotFound()
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.announce(ctx, id, StatusNone)
	return nil
}

func errAgentNotFound() error {
	return domain.NotFound("agent_not_found", "That agent does not exist.")
}

func errNotOwner() error {
	return domain.Forbidden("not_owner", "Only the agent's owner can do that.")
}

// checkPicture refuses a face that is not the owner's own upload. The rule
// itself is the media package's, so a person's photo and an agent's logo
// are held to one standard.
func (s *Service) checkPicture(ctx context.Context, ownerID uuid.UUID, mediaID *uuid.UUID) error {
	if mediaID == nil {
		return nil
	}
	row, err := s.store.GetMedia(ctx, *mediaID)
	if err != nil && !store.IsNoRows(err) {
		return domain.Internal(fmt.Errorf("get picture %s: %w", *mediaID, err))
	}
	return media.Picture(row, err == nil, media.Owner{Kind: media.OwnerUser, ID: ownerID})
}

// GetByHandle finds a live agent by its address on this hub.
func (s *Service) GetByHandle(ctx context.Context, handle string) (Agent, error) {
	row, err := s.store.GetAgentByHandle(ctx, handle)
	if err != nil {
		if store.IsNoRows(err) {
			return Agent{}, domain.NotFound("agent_not_found", "That agent does not exist.")
		}
		return Agent{}, domain.Internal(fmt.Errorf("get agent @%s: %w", handle, err))
	}
	return agentFromRow(row), nil
}

// DeleteAllOwned retires every agent an owner has, as deleting each would:
// bindings revoked, handles kept off the market, history readable.
func (s *Service) DeleteAllOwned(ctx context.Context, ownerID uuid.UUID) error {
	rows, err := s.store.ListAgentsByOwner(ctx, ownerID)
	if err != nil {
		return domain.Internal(fmt.Errorf("list agents of %s: %w", ownerID, err))
	}
	for _, r := range rows {
		if err := s.Delete(ctx, ownerID, r.ID); err != nil {
			return err
		}
	}
	return nil
}

// optionalBool carries "leave it alone" for a column that cannot itself be
// null. sqlc maps a narg over a NOT NULL boolean to pgtype.Bool, where Valid
// false means the COALESCE keeps what is already there.
func optionalBool(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}
