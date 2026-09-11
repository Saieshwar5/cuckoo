package agentapi_test

import (
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

type scheduleJSON struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	NextRunAt *string `json:"next_run_at"`
	LastRunAt *string `json:"last_run_at"`
	Cadence   struct {
		Repeat string `json:"repeat"`
		Time   string `json:"time"`
	} `json:"cadence"`
}

// A person makes a schedule in the app; the agent hears of it, holds it, and
// the message it sends later says which schedule sent it.
func TestSchedulesAcrossTheAPIs(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)
	person := f.srv.AsUser(t, f.owner)
	agentID := domain.FormatID(domain.PrefixAgent, f.agent.ID)
	base := "/v1/client/conversations/" + f.dmID + "/schedules"
	ask := map[string]any{
		"instruction": "Weather in Hyderabad",
		"cadence":     map[string]any{"repeat": "daily", "time": "07:00", "timezone": "Asia/Kolkata"},
	}

	person.Post(base, ask).ExpectError(http.StatusConflict, "schedules_not_supported")
	person.Patch("/v1/mgmt/agents/"+agentID, map[string]any{"supports_schedules": true}).ExpectStatus(http.StatusOK)

	var made struct {
		Schedule scheduleJSON `json:"schedule"`
	}
	person.Post(base, ask).ExpectStatus(http.StatusCreated).Decode(&made)
	if made.Schedule.Status != "pending" || made.Schedule.NextRunAt == nil || made.Schedule.Cadence.Time != "07:00" {
		t.Fatalf("made = %+v", made.Schedule)
	}

	var evs struct {
		Events []struct {
			Type string `json:"type"`
			Data struct {
				Schedule scheduleJSON `json:"schedule"`
			} `json:"data"`
		} `json:"events"`
	}
	agent.Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&evs)
	var heard bool
	for _, ev := range evs.Events {
		if ev.Type == "schedule.requested" && ev.Data.Schedule.ID == made.Schedule.ID {
			heard = true
		}
	}
	if !heard {
		t.Fatalf("events = %+v, want a schedule.requested for %s", evs.Events, made.Schedule.ID)
	}

	agentBase := "/v1/agent/conversations/" + f.dmID + "/schedules/" + made.Schedule.ID
	agent.Patch(agentBase, map[string]any{"status": "active", "title": "Morning weather"}).ExpectStatus(http.StatusOK)

	var sent struct {
		Message struct {
			ScheduleID string `json:"schedule_id"`
		} `json:"message"`
	}
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"text": "31°C and clear", "schedule_id": made.Schedule.ID,
	}).ExpectStatus(http.StatusCreated).Decode(&sent)
	if sent.Message.ScheduleID != made.Schedule.ID {
		t.Errorf("sent schedule_id = %q", sent.Message.ScheduleID)
	}

	var page struct {
		Messages []struct {
			Schedule *struct {
				ID    string `json:"id"`
				Title string `json:"title"`
			} `json:"schedule"`
		} `json:"messages"`
	}
	person.Get("/v1/client/conversations/" + f.dmID + "/messages").ExpectStatus(http.StatusOK).Decode(&page)
	if len(page.Messages) == 0 || page.Messages[0].Schedule == nil || page.Messages[0].Schedule.Title != "Morning weather" {
		t.Errorf("newest message = %+v, want tagged Morning weather", page.Messages)
	}

	var list struct {
		Schedules []scheduleJSON `json:"schedules"`
	}
	person.Get(base).ExpectStatus(http.StatusOK).Decode(&list)
	if len(list.Schedules) != 1 || list.Schedules[0].Status != "active" || list.Schedules[0].LastRunAt == nil {
		t.Errorf("list = %+v, want one active that has run", list.Schedules)
	}

	person.Patch(base+"/"+made.Schedule.ID, map[string]any{"paused": true}).ExpectStatus(http.StatusOK)
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"text": "again", "schedule_id": made.Schedule.ID,
	}).ExpectError(http.StatusConflict, "schedule_paused")
	person.Delete(base + "/" + made.Schedule.ID).ExpectStatus(http.StatusNoContent)
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"text": "again", "schedule_id": made.Schedule.ID,
	}).ExpectError(http.StatusConflict, "schedule_deleted")
}
