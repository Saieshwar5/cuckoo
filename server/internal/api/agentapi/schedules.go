package agentapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

type scheduleEnvelope struct {
	Schedule events.Schedule `json:"schedule"`
}

type scheduleListEnvelope struct {
	Schedules []events.Schedule `json:"schedules"`
}

// createScheduleRequest is a schedule the agent already holds, made in the
// chat, recorded so the person sees it beside the ones they made.
type createScheduleRequest struct {
	Title       string                `json:"title"`
	Instruction string                `json:"instruction"`
	Cadence     conversations.Cadence `json:"cadence"`
}

// updateScheduleRequest is the agent's word on a schedule: usually
// {"status": "active"} in answer to a person's request.
type updateScheduleRequest struct {
	Title       *string                `json:"title"`
	Instruction *string                `json:"instruction"`
	Cadence     *conversations.Cadence `json:"cadence"`
	Status      *string                `json:"status"`
}

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	agentID, conversationID, ok := h.scheduleScope(w, r)
	if !ok {
		return
	}
	list, err := h.conversations.ListSchedulesForAgent(r.Context(), agentID, conversationID)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	now := time.Now()
	out := scheduleListEnvelope{Schedules: make([]events.Schedule, 0, len(list))}
	for _, s := range list {
		out.Schedules = append(out.Schedules, events.ScheduleOf(s, now))
	}
	httpx.JSON(w, r, http.StatusOK, out)
}

func (h *Handler) createSchedule(w http.ResponseWriter, r *http.Request) {
	agentID, conversationID, ok := h.scheduleScope(w, r)
	if !ok {
		return
	}
	var req createScheduleRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	sch, err := h.conversations.CreateScheduleAsAgent(r.Context(), agentID, conversationID, conversations.AgentSchedule{
		Title: req.Title, Instruction: req.Instruction, Cadence: req.Cadence,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, scheduleEnvelope{Schedule: events.ScheduleOf(sch, time.Now())})
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	agentID, conversationID, ok := h.scheduleScope(w, r)
	if !ok {
		return
	}
	scheduleID, err := scheduleIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req updateScheduleRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	sch, err := h.conversations.UpdateScheduleAsAgent(r.Context(), agentID, conversationID, scheduleID,
		conversations.AgentScheduleChange{
			Title: req.Title, Instruction: req.Instruction, Cadence: req.Cadence, Status: req.Status,
		})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, scheduleEnvelope{Schedule: events.ScheduleOf(sch, time.Now())})
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	agentID, conversationID, ok := h.scheduleScope(w, r)
	if !ok {
		return
	}
	scheduleID, err := scheduleIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.conversations.DeleteScheduleAsAgent(r.Context(), agentID, conversationID, scheduleID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.NoContent(w)
}

// scheduleScope reads who is asking and about which conversation.
func (h *Handler) scheduleScope(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return uuid.Nil, uuid.Nil, false
	}
	conversationID, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	return agentID, conversationID, true
}

func scheduleIDParam(r *http.Request) (uuid.UUID, error) {
	return domain.ParseID(domain.PrefixSchedule, chi.URLParam(r, "sid"))
}

// scheduleIDOf parses the schedule a message says it was sent for.
func scheduleIDOf(raw string) (*uuid.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	id, err := domain.ParseID(domain.PrefixSchedule, raw)
	if err != nil {
		return nil, domain.InvalidField("schedule_id", "invalid_id", "schedule_id must be a schedule id.")
	}
	return &id, nil
}
