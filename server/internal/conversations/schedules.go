package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// Schedule statuses.
const (
	// SchedulePending: the person asked, and the agent has not yet said it
	// holds the schedule. The app says "waiting for …".
	SchedulePending = "pending"
	ScheduleActive  = "active"
	// SchedulePaused: the person paused it. Only the person resumes it.
	SchedulePaused = "paused"
)

// Outbox events: what the person did to a schedule, told to the agent.
const (
	EventScheduleRequested = "schedule.requested"
	EventScheduleUpdated   = "schedule.updated"
	EventScheduleDeleted   = "schedule.deleted"
	// EventScheduleChanged is for the person's devices: a schedule in one of
	// their chats is new, different, or gone.
	EventScheduleChanged = "schedule.changed"
)

const (
	maxSchedules   = 10
	titleMax       = 60
	instructionMax = 500
)

// Schedule is something a person asked an agent to do at a time.
//
// The agent's backend holds it and runs it; this is the copy the person's
// phone shows. The json tags are the outbox's own snapshot format.
type Schedule struct {
	ID             uuid.UUID       `json:"id"`
	AgentID        uuid.UUID       `json:"agent_id"`
	ConversationID uuid.UUID       `json:"conversation_id"`
	Title          string          `json:"title"`
	Instruction    string          `json:"instruction"`
	Cadence        Cadence         `json:"cadence"`
	Status         string          `json:"status"`
	CreatedBy      ParticipantKind `json:"created_by"`
	LastRunAt      *time.Time      `json:"last_run_at"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Deleted        bool            `json:"deleted"`
}

// NextRun is when it will next run, as far as its cadence says. Nothing for
// a paused one, or a one-off already past.
func (s Schedule) NextRun(now time.Time) *time.Time {
	if s.Status == SchedulePaused || s.Deleted {
		return nil
	}
	if next, ok := s.Cadence.Next(now); ok {
		return &next
	}
	return nil
}

// MissedAt is the last time it was due and nothing came from it, when that
// is worth telling the person: it is active, the time is well past, and no
// message tagged with it arrived around then. The hub runs nothing, so a
// backend that was down at seven is exactly what this is for.
func (s Schedule) MissedAt(now time.Time) *time.Time {
	if s.Status != ScheduleActive || s.Deleted {
		return nil
	}
	due, ok := s.Cadence.Previous(now.Add(-missGrace))
	if !ok || due.Before(s.CreatedAt) {
		return nil
	}
	if s.LastRunAt != nil && !s.LastRunAt.Before(due.Add(-time.Minute)) {
		return nil
	}
	return &due
}

func scheduleFromRow(r gen.Schedule) (Schedule, error) {
	var c Cadence
	if err := json.Unmarshal(r.Cadence, &c); err != nil {
		return Schedule{}, domain.Internal(fmt.Errorf("decode cadence of %s: %w", r.ID, err))
	}
	return Schedule{
		ID: r.ID, AgentID: r.AgentID, ConversationID: r.ConversationID,
		Title: r.Title, Instruction: r.Instruction, Cadence: c, Status: r.Status,
		CreatedBy: ParticipantKind(r.CreatedBy), LastRunAt: r.LastRunAt,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Deleted: r.DeletedAt != nil,
	}, nil
}

// ScheduleRequest is a person asking for a schedule: what, and when.
type ScheduleRequest struct {
	Instruction string
	Cadence     Cadence
}

// ScheduleChange is a person's change to a schedule. Nil leaves a field.
type ScheduleChange struct {
	Paused      *bool
	Instruction *string
	Cadence     *Cadence
}

// ListSchedules returns the schedules in a conversation the caller is in.
func (s *Service) ListSchedules(ctx context.Context, userID, conversationID uuid.UUID) ([]Schedule, error) {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return nil, err
	}
	return s.listSchedules(ctx, conversationID)
}

func (s *Service) listSchedules(ctx context.Context, conversationID uuid.UUID) ([]Schedule, error) {
	rows, err := s.store.ListSchedules(ctx, conversationID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list schedules of %s: %w", conversationID, err))
	}
	out := make([]Schedule, 0, len(rows))
	for _, r := range rows {
		sch, err := scheduleFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, sch)
	}
	return out, nil
}

// RequestSchedule is a person asking the agent in a conversation to do
// something at a time. It is pending until the agent says it holds it.
func (s *Service) RequestSchedule(ctx context.Context, userID, conversationID uuid.UUID, in ScheduleRequest) (Schedule, error) {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return Schedule{}, err
	}
	if err := s.ensureOpen(ctx, conversationID); err != nil {
		return Schedule{}, err
	}
	agentID, err := s.schedulingAgent(ctx, conversationID)
	if err != nil {
		return Schedule{}, err
	}
	instruction, err := validateInstruction(in.Instruction)
	if err != nil {
		return Schedule{}, err
	}
	cadence, err := futureCadence(in.Cadence)
	if err != nil {
		return Schedule{}, err
	}
	wait, err := s.limiter.Allow(ctx, "schedule:user:"+userID.String(), userSendPolicy)
	if err != nil {
		return Schedule{}, domain.Internal(fmt.Errorf("rate limit: %w", err))
	}
	if wait > 0 {
		return Schedule{}, domain.RateLimited("rate_limited", "You are making schedules too quickly.", wait)
	}

	return s.createSchedule(ctx, gen.CreateScheduleParams{
		AgentID: agentID, ConversationID: conversationID,
		Title: titleFrom(instruction), Instruction: instruction,
		Status: SchedulePending, CreatedBy: string(ParticipantUser),
	}, cadence, true)
}

// createSchedule writes a schedule, and — when the person made it — the
// event that asks the agent to hold it, in the same transaction.
func (s *Service) createSchedule(ctx context.Context, params gen.CreateScheduleParams, cadence Cadence, tellAgent bool) (Schedule, error) {
	raw, err := json.Marshal(cadence)
	if err != nil {
		return Schedule{}, domain.Internal(fmt.Errorf("encode cadence: %w", err))
	}
	params.ID = domain.NewID()
	params.Cadence = raw

	var (
		sch     Schedule
		pending bool
	)
	err = s.store.WithTx(ctx, func(tx *store.Store) error {
		count, err := tx.CountSchedules(ctx, params.ConversationID)
		if err != nil {
			return domain.Internal(fmt.Errorf("count schedules: %w", err))
		}
		if count >= maxSchedules {
			return domain.Conflict("too_many_schedules",
				fmt.Sprintf("A chat can hold %d schedules. Delete one to make another.", maxSchedules))
		}
		row, err := tx.CreateSchedule(ctx, params)
		if err != nil {
			return domain.Internal(fmt.Errorf("create schedule: %w", err))
		}
		if sch, err = scheduleFromRow(row); err != nil {
			return err
		}
		if tellAgent {
			pending, err = New(tx).Enqueue(ctx, sch.AgentID, sch.ConversationID, EventScheduleRequested, sch)
		}
		return err
	})
	if err != nil {
		return Schedule{}, err
	}
	s.scheduleChanged(ctx, sch, pending)
	return sch, nil
}

// UpdateSchedule is a person pausing, resuming or changing a schedule.
// Pausing takes effect here at once — a paused schedule can no longer speak
// — and a change of what or when waits for the agent again.
func (s *Service) UpdateSchedule(ctx context.Context, userID, conversationID, scheduleID uuid.UUID, in ScheduleChange) (Schedule, error) {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return Schedule{}, err
	}
	current, err := s.scheduleIn(ctx, conversationID, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	params := gen.UpdateScheduleParams{ID: scheduleID}
	if in.Instruction != nil {
		instruction, err := validateInstruction(*in.Instruction)
		if err != nil {
			return Schedule{}, err
		}
		params.Instruction = &instruction
		title := titleFrom(instruction)
		params.Title = &title
	}
	if in.Cadence != nil {
		cadence, err := futureCadence(*in.Cadence)
		if err != nil {
			return Schedule{}, err
		}
		if params.Cadence, err = json.Marshal(cadence); err != nil {
			return Schedule{}, domain.Internal(fmt.Errorf("encode cadence: %w", err))
		}
	}
	status := current.Status
	if params.Instruction != nil || params.Cadence != nil {
		status = SchedulePending
	}
	if in.Paused != nil {
		switch {
		case *in.Paused:
			status = SchedulePaused
		case status == SchedulePaused:
			status = ScheduleActive
		}
	}
	if params.Instruction == nil && params.Cadence == nil && status == current.Status {
		return current, nil
	}
	params.Status = &status

	return s.writeSchedule(ctx, func(tx *store.Store) (gen.Schedule, error) {
		return tx.UpdateSchedule(ctx, params)
	}, EventScheduleUpdated)
}

// DeleteSchedule is a person ending a schedule. It is gone from their list
// at once, and anything the agent sends for it afterwards is refused.
func (s *Service) DeleteSchedule(ctx context.Context, userID, conversationID, scheduleID uuid.UUID) error {
	if _, err := s.member(ctx, userID, conversationID); err != nil {
		return err
	}
	if _, err := s.scheduleIn(ctx, conversationID, scheduleID); err != nil {
		return err
	}
	_, err := s.writeSchedule(ctx, func(tx *store.Store) (gen.Schedule, error) {
		return tx.DeleteSchedule(ctx, scheduleID)
	}, EventScheduleDeleted)
	return err
}

// writeSchedule applies a person's change and tells the agent, together.
func (s *Service) writeSchedule(ctx context.Context, write func(*store.Store) (gen.Schedule, error), event string) (Schedule, error) {
	var (
		sch     Schedule
		pending bool
	)
	err := s.store.WithTx(ctx, func(tx *store.Store) error {
		row, err := write(tx)
		if err != nil {
			if store.IsNoRows(err) {
				return errScheduleNotFound()
			}
			return domain.Internal(fmt.Errorf("write schedule: %w", err))
		}
		if sch, err = scheduleFromRow(row); err != nil {
			return err
		}
		pending, err = New(tx).Enqueue(ctx, sch.AgentID, sch.ConversationID, event, sch)
		return err
	})
	if err != nil {
		return Schedule{}, err
	}
	s.scheduleChanged(ctx, sch, pending)
	return sch, nil
}

// ScheduleChangedEvent tells a person's devices a schedule is new,
// different, or — with Deleted set — gone.
type ScheduleChangedEvent struct {
	ConversationID uuid.UUID `json:"conversation_id"`
	Schedule       Schedule  `json:"schedule"`
}

// scheduleChanged announces a schedule to the conversation's people, and
// nudges the agent's socket when an event is waiting for it.
func (s *Service) scheduleChanged(ctx context.Context, sch Schedule, nudgeAgent bool) {
	s.notify(ctx, EventScheduleChanged, sch.ConversationID,
		ScheduleChangedEvent{ConversationID: sch.ConversationID, Schedule: sch})
	if nudgeAgent {
		s.nudge(ctx, []uuid.UUID{sch.AgentID})
	}
}

// scheduleIn loads a live schedule and checks it belongs to the conversation.
func (s *Service) scheduleIn(ctx context.Context, conversationID, scheduleID uuid.UUID) (Schedule, error) {
	row, err := s.store.GetSchedule(ctx, scheduleID)
	if err != nil {
		if store.IsNoRows(err) {
			return Schedule{}, errScheduleNotFound()
		}
		return Schedule{}, domain.Internal(fmt.Errorf("get schedule %s: %w", scheduleID, err))
	}
	if row.ConversationID != conversationID || row.DeletedAt != nil {
		return Schedule{}, errScheduleNotFound()
	}
	return scheduleFromRow(row)
}

// schedulingAgent is the agent in a conversation that would hold a schedule,
// and whether it can.
func (s *Service) schedulingAgent(ctx context.Context, conversationID uuid.UUID) (uuid.UUID, error) {
	agentIDs, err := s.store.ListAgentParticipants(ctx, conversationID)
	if err != nil {
		return uuid.Nil, domain.Internal(fmt.Errorf("list agents of %s: %w", conversationID, err))
	}
	if len(agentIDs) != 1 {
		return uuid.Nil, errSchedulesNotSupported()
	}
	agent, err := s.store.GetAgent(ctx, agentIDs[0])
	if err != nil || !agent.SupportsSchedules {
		return uuid.Nil, errSchedulesNotSupported()
	}
	return agent.ID, nil
}

// futureCadence is a cadence that will run at least once more.
func futureCadence(c Cadence) (Cadence, error) {
	c, err := c.normalized()
	if err != nil {
		return Cadence{}, err
	}
	if _, ok := c.Next(time.Now()); !ok {
		return Cadence{}, invalidCadence("That time has already passed.")
	}
	return c, nil
}

func validateInstruction(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > instructionMax {
		return "", domain.InvalidField("instruction", "invalid_instruction",
			fmt.Sprintf("Say what it should do, in up to %d characters.", instructionMax))
	}
	return text, nil
}

// titleFrom names a schedule after the person's own words until the agent
// names it better.
func titleFrom(instruction string) string {
	line, _, _ := strings.Cut(instruction, "\n")
	if utf8.RuneCountInString(line) <= titleMax {
		return line
	}
	return string([]rune(line)[:titleMax-1]) + "…"
}

func errScheduleNotFound() error {
	return domain.NotFound("schedule_not_found", "No such schedule in this chat.")
}

func errSchedulesNotSupported() error {
	return domain.Conflict("schedules_not_supported", "This agent does not take schedules.")
}
