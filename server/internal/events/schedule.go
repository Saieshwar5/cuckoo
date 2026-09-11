package events

import (
	"time"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// Schedule events: what a person did to a schedule, in the app.
const (
	// TypeScheduleRequested: the person asked for a schedule. Hold it, then
	// say so with PUT …/schedules/{id} {"status": "active"} — or decline it
	// with DELETE and a message saying why.
	TypeScheduleRequested = conversations.EventScheduleRequested
	// TypeScheduleUpdated: the person paused, resumed, or changed what or
	// when. A change of what or when is pending until confirmed again.
	TypeScheduleUpdated = conversations.EventScheduleUpdated
	// TypeScheduleDeleted: the person deleted it. Stop its timer; anything
	// sent for it from now is refused.
	TypeScheduleDeleted = conversations.EventScheduleDeleted
)

// Schedule is a schedule as a backend and a phone both see it.
//
// NextRunAt is worked out from the cadence, so no backend has to report it;
// MissedAt is set when the last time it was due passed with nothing sent for
// it. Both are the hub's reading of the clock, not a promise about the timer
// the backend keeps.
type Schedule struct {
	ID             string                `json:"id"`
	ConversationID string                `json:"conversation_id"`
	AgentID        string                `json:"agent_id"`
	Title          string                `json:"title"`
	Instruction    string                `json:"instruction"`
	Cadence        conversations.Cadence `json:"cadence"`
	Status         string                `json:"status"`
	CreatedBy      string                `json:"created_by"`
	NextRunAt      *time.Time            `json:"next_run_at"`
	LastRunAt      *time.Time            `json:"last_run_at"`
	MissedAt       *time.Time            `json:"missed_at"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

// ScheduleOf is the wire form of a schedule, read against now.
func ScheduleOf(s conversations.Schedule, now time.Time) Schedule {
	if s.Cadence.Days == nil && s.Cadence.Repeat == conversations.RepeatWeekly {
		s.Cadence.Days = []string{}
	}
	return Schedule{
		ID:             domain.FormatID(domain.PrefixSchedule, s.ID),
		ConversationID: domain.FormatID(domain.PrefixConv, s.ConversationID),
		AgentID:        domain.FormatID(domain.PrefixAgent, s.AgentID),
		Title:          s.Title,
		Instruction:    s.Instruction,
		Cadence:        s.Cadence,
		Status:         s.Status,
		CreatedBy:      string(s.CreatedBy),
		NextRunAt:      s.NextRun(now),
		LastRunAt:      s.LastRunAt,
		MissedAt:       s.MissedAt(now),
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

// ScheduleEvent is the payload of every schedule event: the schedule as it
// stood when the person acted, and where.
type ScheduleEvent struct {
	Conversation Conversation `json:"conversation"`
	Schedule     Schedule     `json:"schedule"`
}

// NewScheduleEvent builds the event agentID receives when the person acts
// on one of its schedules.
func NewScheduleEvent(eventID uuid.UUID, createdAt time.Time, agentID uuid.UUID, eventType string,
	conv conversations.Conversation, sch conversations.Schedule) Envelope {
	return Envelope{
		ID:        domain.FormatID(domain.PrefixEvent, eventID),
		Type:      eventType,
		CreatedAt: createdAt,
		AgentID:   domain.FormatID(domain.PrefixAgent, agentID),
		Data:      ScheduleEvent{Conversation: ConversationOf(conv), Schedule: ScheduleOf(sch, time.Now())},
	}
}
