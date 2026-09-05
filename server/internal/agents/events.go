package agents

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/realtime"
)

// EventAgentStatus tells a device that an agent's backend came, went, or
// stopped answering. It goes to everyone who has a conversation with the
// agent, so the dot on its avatar and the line under its name are live.
const EventAgentStatus = "agent.status"

// StatusNone is what an agent with no binding at all reports: the binding
// was revoked, or the agent was deleted. It is not a binding status, which
// is why it is not a Status.
const StatusNone = "none"

// AgentStatusEvent is the internal form; the client socket renders the
// wire shape.
type AgentStatusEvent struct {
	AgentID uuid.UUID `json:"agent_id"`
	Status  string    `json:"status"`
}

// announce publishes a status change to everyone who shares a conversation
// with the agent. It never fails the operation that caused it: the record
// is written, and a device that missed the announcement reads the status
// on its next request.
func (s *Service) announce(ctx context.Context, agentID uuid.UUID, status string) {
	userIDs, err := s.store.ListUsersSharingAgent(ctx, agentID)
	if err != nil {
		slog.WarnContext(ctx, "realtime: could not list who shares agent", "agent", agentID, "error", err)
		return
	}
	if len(userIDs) == 0 {
		return
	}
	ev, err := realtime.NewEvent(EventAgentStatus, userIDs, AgentStatusEvent{AgentID: agentID, Status: status})
	if err != nil {
		slog.WarnContext(ctx, "realtime: could not encode agent status", "agent", agentID, "error", err)
		return
	}
	if err := s.publisher.Publish(ctx, ev); err != nil {
		slog.WarnContext(ctx, "realtime: could not publish agent status", "agent", agentID, "error", err)
	}
}
