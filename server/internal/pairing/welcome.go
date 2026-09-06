package pairing

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Welcomer returns what sign-in calls for every new account: the agent at
// handle, if there is one, is added to the person's list and told they
// joined, so the first screen has someone on it. It is an agent like any
// other — the hub's operator creates it and runs its backend — and the
// person may mute, remove or block it like any other. With no handle
// configured, or no such agent, nothing happens.
//
// The work runs inside the sign-in transaction and returns what must run
// after it commits: the nudge that wakes the agent's delivery.
func (s *Service) Welcomer(handle string) func(context.Context, *store.Store, uuid.UUID) (func(), error) {
	if handle == "" {
		return nil
	}
	return func(ctx context.Context, tx *store.Store, userID uuid.UUID) (func(), error) {
		agent, err := tx.GetAgentByHandle(ctx, handle)
		if err != nil {
			if store.IsNoRows(err) {
				return nil, nil
			}
			return nil, domain.Internal(fmt.Errorf("welcome agent @%s: %w", handle, err))
		}
		convs := conversations.New(tx)
		conv, _, err := convs.FindOrCreateDM(ctx, userID, agent.ID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.CreateContact(ctx, gen.CreateContactParams{
			UserID: userID, AgentID: agent.ID, DmConversationID: conv.ID, AddedVia: "hub",
		}); err != nil {
			return nil, domain.Internal(fmt.Errorf("welcome contact: %w", err))
		}
		pending, err := convs.Enqueue(ctx, agent.ID, conv.ID, conversations.EventConversationJoined,
			conversations.JoinedPayload{})
		if err != nil {
			return nil, err
		}
		if !pending {
			return nil, nil
		}
		return func() { s.conversations.Nudge(ctx, []uuid.UUID{agent.ID}) }, nil
	}
}
