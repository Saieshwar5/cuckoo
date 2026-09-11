package conversations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
)

// AgentSchedule is a schedule an agent made itself, usually because the
// person asked for one in the chat and tapped to confirm.
type AgentSchedule struct {
	Title       string
	Instruction string
	Cadence     Cadence
}

// AgentScheduleChange is an agent's word on a schedule it holds. Nil leaves
// a field. Status is "active" — the usual answer to a person's request — or
// "paused".
type AgentScheduleChange struct {
	Title       *string
	Instruction *string
	Cadence     *Cadence
	Status      *string
}

// ListSchedulesForAgent returns the schedules in a conversation the agent is in.
func (s *Service) ListSchedulesForAgent(ctx context.Context, agentID, conversationID uuid.UUID) ([]Schedule, error) {
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return nil, err
	}
	all, err := s.listSchedules(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	own := all[:0]
	for _, sch := range all {
		if sch.AgentID == agentID {
			own = append(own, sch)
		}
	}
	return own, nil
}

// CreateScheduleAsAgent records a schedule the agent holds, so the person
// sees it with the ones they made themselves.
func (s *Service) CreateScheduleAsAgent(ctx context.Context, agentID, conversationID uuid.UUID, in AgentSchedule) (Schedule, error) {
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return Schedule{}, err
	}
	if err := s.ensureOpen(ctx, conversationID); err != nil {
		return Schedule{}, err
	}
	agent, err := s.store.GetAgent(ctx, agentID)
	if err != nil {
		return Schedule{}, domain.Internal(fmt.Errorf("get agent %s: %w", agentID, err))
	}
	if !agent.SupportsSchedules {
		return Schedule{}, domain.Conflict("schedules_not_supported",
			"Set supports_schedules on this agent first, so people can see the schedules it holds.")
	}
	title, err := validateTitle(in.Title)
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
	return s.createSchedule(ctx, gen.CreateScheduleParams{
		AgentID: agentID, ConversationID: conversationID, Title: title, Instruction: instruction,
		Status: ScheduleActive, CreatedBy: string(ParticipantAgent),
	}, cadence, false)
}

// UpdateScheduleAsAgent is the agent confirming, renaming or pausing a
// schedule it holds. It cannot resume one the person paused: that is theirs
// to undo.
func (s *Service) UpdateScheduleAsAgent(ctx context.Context, agentID, conversationID, scheduleID uuid.UUID, in AgentScheduleChange) (Schedule, error) {
	current, err := s.agentSchedule(ctx, agentID, conversationID, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	params := gen.UpdateScheduleParams{ID: scheduleID}
	if in.Title != nil {
		title, err := validateTitle(*in.Title)
		if err != nil {
			return Schedule{}, err
		}
		params.Title = &title
	}
	if in.Instruction != nil {
		instruction, err := validateInstruction(*in.Instruction)
		if err != nil {
			return Schedule{}, err
		}
		params.Instruction = &instruction
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
	if in.Status != nil {
		switch *in.Status {
		case ScheduleActive:
			if current.Status == SchedulePaused {
				return Schedule{}, domain.Forbidden("paused_by_person",
					"The person paused this schedule. Only they can resume it.")
			}
		case SchedulePaused:
		default:
			return Schedule{}, domain.InvalidField("status", "invalid_status", `Status must be "active" or "paused".`)
		}
		params.Status = in.Status
	}
	if params.Title == nil && params.Instruction == nil && params.Cadence == nil && params.Status == nil {
		return Schedule{}, domain.Invalid("no_changes", "Provide at least one field to update.")
	}

	row, err := s.store.UpdateSchedule(ctx, params)
	if err != nil {
		if store.IsNoRows(err) {
			return Schedule{}, errScheduleNotFound()
		}
		return Schedule{}, domain.Internal(fmt.Errorf("update schedule %s: %w", scheduleID, err))
	}
	sch, err := scheduleFromRow(row)
	if err != nil {
		return Schedule{}, err
	}
	s.scheduleChanged(ctx, sch, false)
	return sch, nil
}

// DeleteScheduleAsAgent is the agent letting go of a schedule: declining a
// request it cannot do, or ending a one-off that has run.
func (s *Service) DeleteScheduleAsAgent(ctx context.Context, agentID, conversationID, scheduleID uuid.UUID) error {
	if _, err := s.agentSchedule(ctx, agentID, conversationID, scheduleID); err != nil {
		return err
	}
	row, err := s.store.DeleteSchedule(ctx, scheduleID)
	if err != nil {
		if store.IsNoRows(err) {
			return errScheduleNotFound()
		}
		return domain.Internal(fmt.Errorf("delete schedule %s: %w", scheduleID, err))
	}
	sch, err := scheduleFromRow(row)
	if err != nil {
		return err
	}
	s.scheduleChanged(ctx, sch, false)
	return nil
}

func (s *Service) agentSchedule(ctx context.Context, agentID, conversationID, scheduleID uuid.UUID) (Schedule, error) {
	if _, err := s.agentMember(ctx, agentID, conversationID); err != nil {
		return Schedule{}, err
	}
	sch, err := s.scheduleIn(ctx, conversationID, scheduleID)
	if err != nil {
		return Schedule{}, err
	}
	if sch.AgentID != agentID {
		return Schedule{}, errScheduleNotFound()
	}
	return sch, nil
}

// checkScheduleTag decides whether a message may say a schedule sent it.
//
// This is where a person's word on a schedule is enforced without the hub
// running anything: the backend keeps its own timer, but a message for a
// schedule the person deleted or paused is refused, whatever that timer says.
func (s *Service) checkScheduleTag(ctx context.Context, sender Sender, conversationID uuid.UUID, scheduleID *uuid.UUID) error {
	if scheduleID == nil {
		return nil
	}
	if sender.Kind != ParticipantAgent {
		return domain.InvalidField("schedule_id", "invalid_schedule_id", "Only an agent sends for a schedule.")
	}
	row, err := s.store.GetSchedule(ctx, *scheduleID)
	if err != nil {
		if store.IsNoRows(err) {
			return errScheduleNotFound()
		}
		return domain.Internal(fmt.Errorf("get schedule %s: %w", *scheduleID, err))
	}
	switch {
	case row.AgentID != sender.ID || row.ConversationID != conversationID:
		return errScheduleNotFound()
	case row.DeletedAt != nil:
		return domain.Conflict("schedule_deleted", "The person deleted this schedule. Stop running it.")
	case row.Status == SchedulePaused:
		return domain.Conflict("schedule_paused", "The person paused this schedule. Skip this run.")
	}
	return nil
}

// ranSchedule records that a schedule just spoke, and shows the person.
func (s *Service) ranSchedule(ctx context.Context, scheduleID uuid.UUID) {
	if err := s.store.MarkScheduleRan(ctx, scheduleID); err != nil {
		return
	}
	row, err := s.store.GetSchedule(ctx, scheduleID)
	if err != nil {
		return
	}
	if sch, err := scheduleFromRow(row); err == nil {
		s.scheduleChanged(ctx, sch, false)
	}
}

// attachScheduleTitles names the schedule each message was sent for, with
// one query, for the tag on the bubble.
func (s *Service) attachScheduleTitles(ctx context.Context, msgs []Message) error {
	var ids []uuid.UUID
	for _, m := range msgs {
		if m.ScheduleID != nil {
			ids = append(ids, *m.ScheduleID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.store.ListScheduleTitles(ctx, ids)
	if err != nil {
		return domain.Internal(fmt.Errorf("list schedule titles: %w", err))
	}
	titles := make(map[uuid.UUID]string, len(rows))
	for _, r := range rows {
		titles[r.ID] = r.Title
	}
	for i := range msgs {
		if msgs[i].ScheduleID != nil {
			msgs[i].ScheduleTitle = titles[*msgs[i].ScheduleID]
		}
	}
	return nil
}

func validateTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || strings.ContainsAny(title, "\n\r") || utf8.RuneCountInString(title) > titleMax {
		return "", domain.InvalidField("title", "invalid_title",
			fmt.Sprintf("A title is one line of up to %d characters.", titleMax))
	}
	return title, nil
}
