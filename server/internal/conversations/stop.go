package conversations

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/ratelimit"
)

// EventStopRequested is what an agent's backend receives when a person
// presses stop. It travels through the outbox like every other event, so a
// backend that was away still hears it — although by then there is rarely
// anything left to stop.
const EventStopRequested = "stop.requested"

// StopPayload is what the outbox row of a stop holds: the reply the hub
// ended, when the agent had started writing one.
type StopPayload struct {
	MessageID *uuid.UUID `json:"message_id"`
}

// stopPolicy bounds how often one person can press stop. Generous, because
// a thumb on a button is not an attack; there only to keep a stuck client
// from writing an outbox row a second to every agent it knows.
var stopPolicy = ratelimit.Policy{Rate: 1, Burst: 20}

// Stop is a person asking the agents in a conversation to stop what they
// are doing.
//
// The hub does everything it can do by itself, at once, without waiting for
// any backend: every reply being written is finished where it stands and
// marked stopped, so no further word appears in it whatever the agent does
// next; the indicator is cleared on every device; and each agent is told,
// so it can cancel the work behind the words. What the hub cannot do is
// take back something already done — an email already sent stays sent, and
// saying so is the agent's job.
//
// It returns the replies it ended.
func (s *Service) Stop(ctx context.Context, userID, conversationID uuid.UUID) ([]Message, error) {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return nil, err
	}
	if err := s.ensureOpen(ctx, conversationID); err != nil {
		return nil, err
	}
	wait, err := s.limiter.Allow(ctx, "stop:user:"+userID.String(), stopPolicy)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return nil, domain.RateLimited("rate_limited", "You are pressing stop too quickly.", wait)
	}

	open, err := s.store.ListStreamingInConversation(ctx, conversationID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list streams in %s: %w", conversationID, err))
	}
	ended := make([]Message, 0, len(open))
	byAgent := make(map[uuid.UUID]uuid.UUID, len(open))
	for _, row := range open {
		msg, err := s.finishStream(ctx, row.SenderAgentID, row.ID, FinishInput{}, endedByPerson)
		if err != nil {
			// The agent finished it itself between the listing and now:
			// nothing left to stop, which is the outcome asked for.
			if domain.CodeOf(err) == "stream_not_found" {
				continue
			}
			return nil, err
		}
		if msg.Stopped {
			ended = append(ended, msg)
			byAgent[row.SenderAgentID] = row.ID
		}
	}

	agentIDs, err := s.store.ListAgentParticipants(ctx, conversationID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list agents of %s: %w", conversationID, err))
	}
	var pending []uuid.UUID
	for _, agentID := range agentIDs {
		s.announceActivity(ctx, agentID, conversationID, ActivityIdle, "")
		payload := StopPayload{}
		if id, ok := byAgent[agentID]; ok {
			payload.MessageID = &id
		}
		queued, err := s.Enqueue(ctx, agentID, conversationID, EventStopRequested, payload)
		if err != nil {
			return nil, err
		}
		if queued {
			pending = append(pending, agentID)
		}
	}
	s.nudge(ctx, pending)
	return ended, nil
}

// whyNotStreaming explains a refused append. Most often the stream finished
// or never was; when the person stopped it, the agent is told so in a code
// of its own, because that is the one case where the right response is to
// stop working rather than to report a bug.
func (s *Service) whyNotStreaming(ctx context.Context, agentID, messageID uuid.UUID) error {
	row, err := s.store.GetMessage(ctx, messageID)
	if err == nil && row.Stopped && row.SenderAgentID != nil && *row.SenderAgentID == agentID {
		return domain.Conflict("stopped", "The person stopped this reply. Stop the work behind it.")
	}
	return domain.Conflict("not_streaming",
		"That message is not streaming. It may have finished, been cut off, or belong to another agent.")
}
