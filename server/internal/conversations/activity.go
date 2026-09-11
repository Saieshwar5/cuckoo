package conversations

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// activityFor is how long an activity shows without being renewed. A backend
// that crashes mid-thought leaves "thinking…" on nobody's screen for longer
// than this; one that streams never needs to send one, because the bubble
// growing is itself the sign.
const activityFor = 10 * time.Second

// What an agent can say it is doing.
const (
	// ActivityThinking: working on a reply, with nothing more to say.
	ActivityThinking = "thinking"
	// ActivityWorking: doing something it names in a short label —
	// "checking the weather", "searching flights".
	ActivityWorking = "working"
	// ActivityIdle: done, or given up. Clears whatever was showing.
	ActivityIdle = "idle"
)

// activityLabelMax is the longest label, in characters. It is a line under
// a name in the chat list, not a message.
const activityLabelMax = 40

// The older way to say the same thing: typing start is thinking, stop is
// idle. Kept, because a backend written against it must keep working.
const (
	TypingStart = "start"
	TypingStop  = "stop"
)

// ActivityEvent tells a device what an agent is doing. It expires on its
// own: the device clears it at ExpiresAt unless told again.
type ActivityEvent struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	AgentID        uuid.UUID `json:"agent_id"`
	State          string    `json:"state"`
	Label          string    `json:"label,omitempty"`
	ExpiresAt      time.Time `json:"expires_at"`
}

// Typing is Activity in the words of the first version of the protocol.
func (s *Service) Typing(ctx context.Context, agentID, conversationID uuid.UUID, state string) error {
	switch state {
	case TypingStart:
		return s.Activity(ctx, agentID, conversationID, ActivityThinking, "")
	case TypingStop:
		return s.Activity(ctx, agentID, conversationID, ActivityIdle, "")
	default:
		return domain.InvalidField("state", "invalid_state", `State must be "start" or "stop".`)
	}
}

// Activity tells the people in a conversation what the agent is doing.
// Nothing is stored: the indicator expires on the device, and a message
// arriving clears it.
func (s *Service) Activity(ctx context.Context, agentID, conversationID uuid.UUID, state, label string) error {
	label, err := validateActivity(state, label)
	if err != nil {
		return err
	}
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return err
	}
	if err := s.ensureOpen(ctx, conversationID); err != nil {
		return err
	}

	wait, err := s.limiter.Allow(ctx, "activity:agent:"+agentID.String(), agentDeltaPolicy)
	if err != nil {
		return domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return domain.RateLimited("rate_limited", "You are sending activity updates too quickly.", wait)
	}

	s.announceActivity(ctx, agentID, conversationID, state, label)
	return nil
}

// announceActivity publishes an activity with no questions asked, for
// callers that have already decided it is right: the agent's own request,
// or the hub clearing the indicator when a person presses stop.
func (s *Service) announceActivity(ctx context.Context, agentID, conversationID uuid.UUID, state, label string) {
	expires := time.Now()
	if state != ActivityIdle {
		expires = expires.Add(activityFor)
	}
	s.notify(ctx, EventActivity, conversationID, ActivityEvent{
		ConversationID: conversationID, AgentID: agentID, State: state, Label: label, ExpiresAt: expires,
	})
}

// validateActivity checks a state and its label, returning the label as it
// will be shown.
//
// The label is text an agent controls, drawn in the chat list under its
// name, so it is held to what a status line is for: short, one line, and
// never a link. Otherwise it is an advertising slot nobody asked for.
func validateActivity(state, label string) (string, error) {
	switch state {
	case ActivityThinking, ActivityIdle:
		if label != "" {
			return "", domain.InvalidField("label", "invalid_label",
				`Only "working" takes a label; "thinking" and "idle" say enough.`)
		}
		return "", nil
	case ActivityWorking:
	default:
		return "", domain.InvalidField("state", "invalid_state",
			`State must be "thinking", "working" or "idle".`)
	}

	label = strings.TrimSpace(label)
	if label == "" {
		return "", nil
	}
	if !utf8.ValidString(label) || utf8.RuneCountInString(label) > activityLabelMax {
		return "", domain.InvalidField("label", "invalid_label",
			fmt.Sprintf("A label is at most %d characters.", activityLabelMax))
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return "", domain.InvalidField("label", "invalid_label", "A label is one line of plain text.")
		}
	}
	lower := strings.ToLower(label)
	if strings.Contains(lower, "://") || strings.Contains(lower, "www.") {
		return "", domain.InvalidField("label", "invalid_label", "A label cannot contain a link.")
	}
	return label, nil
}
