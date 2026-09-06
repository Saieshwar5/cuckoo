package conversations

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// typingFor is how long a typing indicator shows without being renewed. A
// backend that streams never needs to send one: a stream is typing.
const typingFor = 10 * time.Second

// Typing states.
const (
	TypingStart = "start"
	TypingStop  = "stop"
)

// Typing tells the people in a conversation that the agent is working on a
// reply, or has stopped. Nothing is stored: the indicator expires on the
// device, and a message arriving clears it.
func (s *Service) Typing(ctx context.Context, agentID, conversationID uuid.UUID, state string) error {
	if state != TypingStart && state != TypingStop {
		return domain.InvalidField("state", "invalid_state", `State must be "start" or "stop".`)
	}
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return err
	}
	if err := s.ensureOpen(ctx, conversationID); err != nil {
		return err
	}

	wait, err := s.limiter.Allow(ctx, "typing:agent:"+agentID.String(), agentDeltaPolicy)
	if err != nil {
		return domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return domain.RateLimited("rate_limited", "You are sending typing updates too quickly.", wait)
	}

	expires := time.Now()
	if state == TypingStart {
		expires = expires.Add(typingFor)
	}
	s.notify(ctx, EventTyping, conversationID, TypingEvent{
		ConversationID: conversationID, AgentID: agentID, State: state, ExpiresAt: expires,
	})
	return nil
}
