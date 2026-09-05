package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Service holds the rules for conversations and their messages.
//
// It takes the concrete store because opening a conversation is a transaction:
// the conversation and its members appear together or not at all.
type Service struct {
	store *store.Store
}

// New builds the service.
func New(st *store.Store) *Service { return &Service{store: st} }

// CreateDM opens the private conversation between a person and an agent.
//
// Agent creation calls this inside its own transaction, so an agent never
// exists without the chat its owner will find it in. The caller is trusted to
// have checked that both parties exist; the foreign keys refuse anything else.
func (s *Service) CreateDM(ctx context.Context, userID, agentID uuid.UUID) (Conversation, error) {
	var row gen.Conversation
	err := s.store.WithTx(ctx, func(tx *store.Store) error {
		var err error
		row, err = tx.CreateConversation(ctx, gen.CreateConversationParams{
			ID:   domain.NewID(),
			Kind: string(KindDM),
		})
		if err != nil {
			return domain.Internal(fmt.Errorf("create conversation: %w", err))
		}
		err = tx.AddParticipant(ctx, gen.AddParticipantParams{
			ConversationID: row.ID, Kind: string(ParticipantUser), UserID: &userID,
		})
		if err != nil {
			return domain.Internal(fmt.Errorf("add user %s to %s: %w", userID, row.ID, err))
		}
		err = tx.AddParticipant(ctx, gen.AddParticipantParams{
			ConversationID: row.ID, Kind: string(ParticipantAgent), AgentID: &agentID,
		})
		if err != nil {
			return domain.Internal(fmt.Errorf("add agent %s to %s: %w", agentID, row.ID, err))
		}
		return nil
	})
	if err != nil {
		return Conversation{}, err
	}

	convs, err := s.hydrate(ctx, []gen.Conversation{row})
	if err != nil {
		return Conversation{}, err
	}
	return convs[0], nil
}

// ListMine returns every conversation the caller is in, most recently active
// first, each with its members and latest message.
func (s *Service) ListMine(ctx context.Context, callerID uuid.UUID) ([]Conversation, error) {
	rows, err := s.store.ListUserConversations(ctx, callerID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list conversations of %s: %w", callerID, err))
	}
	return s.hydrate(ctx, rows)
}

// GetMine returns one conversation the caller is a member of.
//
// A missing conversation is not-found; an existing one the caller is not in
// is forbidden. Identifiers are unguessable, so confirming that one exists
// reveals nothing, and the distinction tells a developer which mistake they
// made.
func (s *Service) GetMine(ctx context.Context, callerID, id uuid.UUID) (Conversation, error) {
	row, err := s.member(ctx, callerID, id)
	if err != nil {
		return Conversation{}, err
	}
	convs, err := s.hydrate(ctx, []gen.Conversation{row})
	if err != nil {
		return Conversation{}, err
	}
	return convs[0], nil
}

// member loads a conversation and checks the caller belongs to it. Every
// read and write of a conversation's contents goes through this.
func (s *Service) member(ctx context.Context, callerID, id uuid.UUID) (gen.Conversation, error) {
	row, err := s.store.GetConversation(ctx, id)
	if err != nil {
		if store.IsNoRows(err) {
			return gen.Conversation{}, errConversationNotFound()
		}
		return gen.Conversation{}, domain.Internal(fmt.Errorf("get conversation %s: %w", id, err))
	}

	ok, err := s.store.IsUserParticipant(ctx, gen.IsUserParticipantParams{
		ConversationID: id, UserID: callerID,
	})
	if err != nil {
		return gen.Conversation{}, domain.Internal(fmt.Errorf("check membership of %s in %s: %w", callerID, id, err))
	}
	if !ok {
		return gen.Conversation{}, errNotParticipant()
	}
	return row, nil
}

// hydrate attaches members and the latest message to conversation rows,
// keeping their order. Two queries however many conversations there are: the
// chat list must not cost a round trip per chat.
func (s *Service) hydrate(ctx context.Context, rows []gen.Conversation) ([]Conversation, error) {
	out := make([]Conversation, len(rows))
	if len(rows) == 0 {
		return out, nil
	}

	ids := make([]uuid.UUID, len(rows))
	index := make(map[uuid.UUID]int, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		index[r.ID] = i
		out[i] = Conversation{
			ID:           r.ID,
			Kind:         Kind(r.Kind),
			Participants: []Participant{},
			CreatedAt:    r.CreatedAt,
		}
	}

	members, err := s.store.ListParticipants(ctx, ids)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list participants: %w", err))
	}
	for _, m := range members {
		i := index[m.ConversationID]
		out[i].Participants = append(out[i].Participants, participantFromRow(m))
	}

	latest, err := s.store.ListLatestMessages(ctx, ids)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list latest messages: %w", err))
	}
	for _, r := range latest {
		msg, err := messageFromRow(r)
		if err != nil {
			return nil, err
		}
		out[index[r.ConversationID]].LastMessage = &msg
	}

	return out, nil
}

func errConversationNotFound() error {
	return domain.NotFound("conversation_not_found", "That conversation does not exist.")
}

func errNotParticipant() error {
	return domain.Forbidden("not_participant", "You are not in this conversation.")
}
