package conversations_test

import (
	"context"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/conversations"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

// A person flies from Hyderabad to London: the schedules set to India's
// clock move to London's, so seven in the morning is still their seven. One
// set on purpose to New York stays in New York, and a paused one moves but
// stays paused.
func TestSchedulesFollowThePhone(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	testutil.BindAgent(t, f.db, f.agent)
	supportSchedules(t, f)
	svc := f.svc

	make := func(title, zone string) conversations.Schedule {
		t.Helper()
		sch, err := svc.CreateScheduleAsAgent(ctx, f.agent.ID, f.dm.ID, conversations.AgentSchedule{
			Title: title, Instruction: title,
			Cadence: conversations.Cadence{Repeat: "daily", Time: "07:00", Timezone: zone},
		})
		if err != nil {
			t.Fatal(err)
		}
		return sch
	}
	morning := make("Morning weather", "Asia/Kolkata")
	market := make("Opening bell", "America/New_York")
	quiet := make("Quiet one", "Asia/Kolkata")
	paused := true
	if _, err := svc.UpdateSchedule(ctx, f.owner.ID, f.dm.ID, quiet.ID, conversations.ScheduleChange{Paused: &paused}); err != nil {
		t.Fatal(err)
	}
	before := len(scheduleEvents(t, f, conversations.EventScheduleUpdated))

	if err := svc.MoveSchedules(ctx, f.owner.ID, "Asia/Kolkata", "Europe/London"); err != nil {
		t.Fatalf("MoveSchedules: %v", err)
	}

	list, _ := svc.ListSchedules(ctx, f.owner.ID, f.dm.ID)
	got := map[string]conversations.Schedule{}
	for _, s := range list {
		got[s.Title] = s
	}
	if s := got[morning.Title]; s.Cadence.Timezone != "Europe/London" || s.Cadence.Time != "07:00" || s.Status != conversations.SchedulePending {
		t.Errorf("morning = %+v, want 07:00 London, pending until the agent confirms", s)
	}
	if s := got[market.Title]; s.Cadence.Timezone != "America/New_York" || s.Status != conversations.ScheduleActive {
		t.Errorf("market = %+v, want left in New York", s)
	}
	if s := got[quiet.Title]; s.Cadence.Timezone != "Europe/London" || s.Status != conversations.SchedulePaused {
		t.Errorf("paused = %+v, want moved and still paused", s)
	}
	if n := len(scheduleEvents(t, f, conversations.EventScheduleUpdated)) - before; n != 2 {
		t.Errorf("agents told of %d moves, want 2", n)
	}
}
