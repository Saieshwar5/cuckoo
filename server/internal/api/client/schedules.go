package client

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

// scheduleRequest is a person asking for a schedule: what, in their words,
// and when, as a time and a repeat picked on their phone.
type scheduleRequest struct {
	Instruction string                `json:"instruction"`
	Cadence     conversations.Cadence `json:"cadence"`
}

// scheduleChangeRequest pauses, resumes or changes one.
type scheduleChangeRequest struct {
	Paused      *bool                  `json:"paused"`
	Instruction *string                `json:"instruction"`
	Cadence     *conversations.Cadence `json:"cadence"`
}

// scheduleChangedFrame tells a person's devices a schedule is new,
// different, or gone.
type scheduleChangedFrame struct {
	ConversationID string          `json:"conversation_id"`
	Schedule       events.Schedule `json:"schedule"`
	Deleted        bool            `json:"deleted"`
}

func (h *Handler) listSchedules(w http.ResponseWriter, r *http.Request) {
	userID, conversationID, ok := scheduleScope(w, r)
	if !ok {
		return
	}
	list, err := h.conversations.ListSchedules(r.Context(), userID, conversationID)
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

func (h *Handler) requestSchedule(w http.ResponseWriter, r *http.Request) {
	userID, conversationID, ok := scheduleScope(w, r)
	if !ok {
		return
	}
	var req scheduleRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	sch, err := h.conversations.RequestSchedule(r.Context(), userID, conversationID, conversations.ScheduleRequest{
		Instruction: req.Instruction, Cadence: req.Cadence,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusCreated, scheduleEnvelope{Schedule: events.ScheduleOf(sch, time.Now())})
}

func (h *Handler) updateSchedule(w http.ResponseWriter, r *http.Request) {
	userID, conversationID, ok := scheduleScope(w, r)
	if !ok {
		return
	}
	scheduleID, err := domain.ParseID(domain.PrefixSchedule, chi.URLParam(r, "sid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	var req scheduleChangeRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, err)
		return
	}
	sch, err := h.conversations.UpdateSchedule(r.Context(), userID, conversationID, scheduleID, conversations.ScheduleChange{
		Paused: req.Paused, Instruction: req.Instruction, Cadence: req.Cadence,
	})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, scheduleEnvelope{Schedule: events.ScheduleOf(sch, time.Now())})
}

func (h *Handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	userID, conversationID, ok := scheduleScope(w, r)
	if !ok {
		return
	}
	scheduleID, err := domain.ParseID(domain.PrefixSchedule, chi.URLParam(r, "sid"))
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	if err := h.conversations.DeleteSchedule(r.Context(), userID, conversationID, scheduleID); err != nil {
		httpx.Error(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func scheduleScope(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := principal.UserID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Sign in to continue."))
		return uuid.Nil, uuid.Nil, false
	}
	conversationID, err := conversationIDParam(r)
	if err != nil {
		httpx.Error(w, r, err)
		return uuid.Nil, uuid.Nil, false
	}
	return userID, conversationID, true
}
