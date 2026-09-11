package conversations_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/store/gen"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

var morning = conversations.Cadence{Repeat: "daily", Time: "07:00", Timezone: "Asia/Kolkata"}

func supportSchedules(t *testing.T, f *fixture) {
	t.Helper()
	if _, err := f.db.UpdateAgent(context.Background(), gen.UpdateAgentParams{
		ID: f.agent.ID, SupportsSchedules: pgtype.Bool{Bool: true, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
}

func scheduleEvents(t *testing.T, f *fixture, eventType string) []gen.MessageDelivery {
	t.Helper()
	rows, err := f.db.ListDeliveriesSince(context.Background(), gen.ListDeliveriesSinceParams{AgentID: f.agent.ID, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	var out []gen.MessageDelivery
	for _, r := range rows {
		if r.EventType == eventType {
			out = append(out, r)
		}
	}
	return out
}

// The whole life of a schedule a person makes: asked for, confirmed by the
// agent, run, paused, resumed, deleted — and the person's word enforced at
// every step without the hub running anything.
func TestScheduleLifecycle(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	testutil.BindAgent(t, f.db, f.agent)
	supportSchedules(t, f)
	rec := &recorder{}
	svc := conversations.New(f.db, conversations.WithPublisher(rec))

	sch, err := svc.RequestSchedule(ctx, f.owner.ID, f.dm.ID, conversations.ScheduleRequest{
		Instruction: "  Tell me the weather in Hyderabad  ", Cadence: morning,
	})
	if err != nil {
		t.Fatalf("RequestSchedule: %v", err)
	}
	if sch.Status != conversations.SchedulePending || sch.Title != "Tell me the weather in Hyderabad" || sch.CreatedBy != conversations.ParticipantUser {
		t.Errorf("requested = %+v, want pending, titled after the words", sch)
	}
	if n := len(scheduleEvents(t, f, conversations.EventScheduleRequested)); n != 1 {
		t.Errorf("schedule.requested events = %d, want 1", n)
	}
	rec.last(t, "delivery.pending") // the agent's socket was nudged
	if sch.NextRun(time.Now()) == nil {
		t.Error("a daily schedule always has a next run")
	}

	// The agent holds it and names it.
	title, active := "Morning weather", conversations.ScheduleActive
	sch, err = svc.UpdateScheduleAsAgent(ctx, f.agent.ID, f.dm.ID, sch.ID, conversations.AgentScheduleChange{Title: &title, Status: &active})
	if err != nil || sch.Status != active || sch.Title != title {
		t.Fatalf("agent confirm = %+v, %v", sch, err)
	}

	// It runs: the message says so, and the schedule knows it ran.
	sent, err := svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "31°C and clear", ScheduleID: &sch.ID})
	if err != nil {
		t.Fatalf("send for schedule: %v", err)
	}
	if sent.ScheduleTitle != "Morning weather" {
		t.Errorf("message schedule title = %q", sent.ScheduleTitle)
	}
	list, _ := svc.ListSchedules(ctx, f.owner.ID, f.dm.ID)
	if len(list) != 1 || list[0].LastRunAt == nil {
		t.Fatalf("list after a run = %+v, want last_run_at set", list)
	}

	// The person pauses it: it can no longer speak, and only they can resume it.
	paused := true
	sch, err = svc.UpdateSchedule(ctx, f.owner.ID, f.dm.ID, sch.ID, conversations.ScheduleChange{Paused: &paused})
	if err != nil || sch.Status != conversations.SchedulePaused {
		t.Fatalf("pause = %+v, %v", sch, err)
	}
	if _, err := svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "x", ScheduleID: &sch.ID}); domain.CodeOf(err) != "schedule_paused" {
		t.Errorf("send while paused got %v", err)
	}
	if _, err := svc.UpdateScheduleAsAgent(ctx, f.agent.ID, f.dm.ID, sch.ID, conversations.AgentScheduleChange{Status: &active}); domain.CodeOf(err) != "paused_by_person" {
		t.Errorf("agent resuming got %v", err)
	}
	resume := false
	if sch, err = svc.UpdateSchedule(ctx, f.owner.ID, f.dm.ID, sch.ID, conversations.ScheduleChange{Paused: &resume}); err != nil || sch.Status != active {
		t.Fatalf("resume = %+v, %v", sch, err)
	}

	// Changing when sends it back to the agent to confirm.
	later := conversations.Cadence{Repeat: "weekdays", Time: "06:30", Timezone: "Asia/Kolkata"}
	if sch, err = svc.UpdateSchedule(ctx, f.owner.ID, f.dm.ID, sch.ID, conversations.ScheduleChange{Cadence: &later}); err != nil || sch.Status != conversations.SchedulePending {
		t.Fatalf("change of time = %+v, %v; want pending again", sch, err)
	}
	if n := len(scheduleEvents(t, f, conversations.EventScheduleUpdated)); n != 3 {
		t.Errorf("schedule.updated events = %d, want 3 (pause, resume, change)", n)
	}

	// Deleted: gone from the list, and the backend's timer can no longer speak.
	if err := svc.DeleteSchedule(ctx, f.owner.ID, f.dm.ID, sch.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := svc.ListSchedules(ctx, f.owner.ID, f.dm.ID); len(list) != 0 {
		t.Errorf("list after delete = %+v", list)
	}
	if _, err := svc.SendAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.SendInput{Text: "x", ScheduleID: &sch.ID}); domain.CodeOf(err) != "schedule_deleted" {
		t.Errorf("send after delete got %v", err)
	}
	if n := len(scheduleEvents(t, f, conversations.EventScheduleDeleted)); n != 1 {
		t.Errorf("schedule.deleted events = %d, want 1", n)
	}
}

func TestScheduleRules(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := f.svc

	// An agent that does not take schedules is not offered them.
	if _, err := svc.RequestSchedule(ctx, f.owner.ID, f.dm.ID, conversations.ScheduleRequest{Instruction: "x", Cadence: morning}); domain.CodeOf(err) != "schedules_not_supported" {
		t.Errorf("unsupported agent got %v", err)
	}
	if _, err := svc.CreateScheduleAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.AgentSchedule{Title: "x", Instruction: "x", Cadence: morning}); domain.CodeOf(err) != "schedules_not_supported" {
		t.Errorf("unsupported agent creating got %v", err)
	}
	supportSchedules(t, f)

	if _, err := svc.RequestSchedule(ctx, f.other.ID, f.dm.ID, conversations.ScheduleRequest{Instruction: "x", Cadence: morning}); domain.CodeOf(err) != "not_participant" {
		t.Errorf("stranger got %v", err)
	}
	past := conversations.Cadence{Repeat: "once", Time: "07:00", Date: "2020-01-01", Timezone: "Asia/Kolkata"}
	if _, err := svc.RequestSchedule(ctx, f.owner.ID, f.dm.ID, conversations.ScheduleRequest{Instruction: "x", Cadence: past}); domain.CodeOf(err) != "invalid_cadence" {
		t.Errorf("a time already past got %v", err)
	}
	if _, err := svc.RequestSchedule(ctx, f.owner.ID, f.dm.ID, conversations.ScheduleRequest{Instruction: " ", Cadence: morning}); domain.CodeOf(err) != "invalid_instruction" {
		t.Errorf("empty instruction got %v", err)
	}

	// Ten to a chat.
	for i := 0; i < 10; i++ {
		if _, err := svc.CreateScheduleAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.AgentSchedule{Title: "Daily", Instruction: "remind me", Cadence: morning}); err != nil {
			t.Fatalf("schedule %d: %v", i, err)
		}
	}
	if _, err := svc.RequestSchedule(ctx, f.owner.ID, f.dm.ID, conversations.ScheduleRequest{Instruction: "one more", Cadence: morning}); domain.CodeOf(err) != "too_many_schedules" {
		t.Errorf("eleventh got %v", err)
	}

	// A person never sends for a schedule, and an agent never for another's.
	list, _ := svc.ListSchedules(ctx, f.owner.ID, f.dm.ID)
	if list[0].CreatedBy != conversations.ParticipantAgent || list[0].Status != conversations.ScheduleActive {
		t.Errorf("agent-made schedule = %+v, want active, by the agent", list[0])
	}
	if _, err := svc.SendAsUser(ctx, f.owner.ID, f.dm.ID, conversations.SendInput{Text: "x", ScheduleID: &list[0].ID}); domain.CodeOf(err) != "invalid_schedule_id" {
		t.Errorf("person sending for a schedule got %v", err)
	}
	stranger := testutil.CreateAgent(t, f.db, f.owner, testutil.WithHandle("stranger"))
	strangerDM := testutil.OwnerDM(t, f.db, stranger)
	if _, err := svc.SendAsAgent(ctx, stranger.ID, strangerDM.ID, conversations.SendInput{Text: "x", ScheduleID: &list[0].ID}); domain.CodeOf(err) != "schedule_not_found" {
		t.Errorf("another agent's schedule got %v", err)
	}
}
