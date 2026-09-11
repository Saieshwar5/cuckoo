package conversations

import (
	"testing"
	"time"
)

func mustZone(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestCadenceNormalized(t *testing.T) {
	good, err := Cadence{Repeat: "weekly", Time: "07:30", Days: []string{"FRI", "mon", "fri"}, Timezone: "Asia/Kolkata"}.normalized()
	if err != nil {
		t.Fatalf("weekly: %v", err)
	}
	if len(good.Days) != 2 || good.Days[0] != "mon" || good.Days[1] != "fri" {
		t.Errorf("days = %v, want [mon fri]: lower-case, once each, in week order", good.Days)
	}

	for name, c := range map[string]Cadence{
		"unknown repeat":      {Repeat: "hourly", Time: "07:00", Timezone: "Asia/Kolkata"},
		"12-hour time":        {Repeat: "daily", Time: "7:00", Timezone: "Asia/Kolkata"},
		"impossible time":     {Repeat: "daily", Time: "25:00", Timezone: "Asia/Kolkata"},
		"no zone":             {Repeat: "daily", Time: "07:00"},
		"made-up zone":        {Repeat: "daily", Time: "07:00", Timezone: "Mars/Olympus"},
		"weekly without days": {Repeat: "weekly", Time: "07:00", Timezone: "Asia/Kolkata"},
		"weekly bad day":      {Repeat: "weekly", Time: "07:00", Days: []string{"funday"}, Timezone: "Asia/Kolkata"},
		"daily with days":     {Repeat: "daily", Time: "07:00", Days: []string{"mon"}, Timezone: "Asia/Kolkata"},
		"once without date":   {Repeat: "once", Time: "07:00", Timezone: "Asia/Kolkata"},
		"daily with a date":   {Repeat: "daily", Time: "07:00", Date: "2026-09-12", Timezone: "Asia/Kolkata"},
	} {
		if _, err := c.normalized(); err == nil {
			t.Errorf("%s: accepted %+v", name, c)
		}
	}
}

func TestCadenceNextAndPrevious(t *testing.T) {
	loc := mustZone(t)
	// A Friday, 08:00 in Hyderabad.
	friday8 := time.Date(2026, 9, 11, 8, 0, 0, 0, loc)

	daily := Cadence{Repeat: "daily", Time: "07:00", Timezone: "Asia/Kolkata"}
	if next, _ := daily.Next(friday8); !next.Equal(time.Date(2026, 9, 12, 7, 0, 0, 0, loc)) {
		t.Errorf("daily next = %v, want tomorrow 07:00", next)
	}
	if prev, _ := daily.Previous(friday8); !prev.Equal(time.Date(2026, 9, 11, 7, 0, 0, 0, loc)) {
		t.Errorf("daily previous = %v, want today 07:00", prev)
	}

	weekdays := Cadence{Repeat: "weekdays", Time: "07:00", Timezone: "Asia/Kolkata"}
	if next, _ := weekdays.Next(friday8); !next.Equal(time.Date(2026, 9, 14, 7, 0, 0, 0, loc)) {
		t.Errorf("weekdays next = %v, want Monday 07:00", next)
	}

	weekly := Cadence{Repeat: "weekly", Time: "18:00", Days: []string{"wed"}, Timezone: "Asia/Kolkata"}
	if next, _ := weekly.Next(friday8); !next.Equal(time.Date(2026, 9, 16, 18, 0, 0, 0, loc)) {
		t.Errorf("weekly next = %v, want Wednesday 18:00", next)
	}
	if prev, _ := weekly.Previous(friday8); !prev.Equal(time.Date(2026, 9, 9, 18, 0, 0, 0, loc)) {
		t.Errorf("weekly previous = %v, want last Wednesday 18:00", prev)
	}

	once := Cadence{Repeat: "once", Time: "09:00", Date: "2026-09-11", Timezone: "Asia/Kolkata"}
	if next, ok := once.Next(friday8); !ok || !next.Equal(time.Date(2026, 9, 11, 9, 0, 0, 0, loc)) {
		t.Errorf("once next = %v %v, want today 09:00", next, ok)
	}
	if _, ok := once.Next(friday8.Add(2 * time.Hour)); ok {
		t.Error("a one-off that has passed has no next run")
	}
}

func TestMissedAt(t *testing.T) {
	loc := mustZone(t)
	now := time.Date(2026, 9, 11, 8, 0, 0, 0, loc)
	sch := Schedule{
		Status:    ScheduleActive,
		Cadence:   Cadence{Repeat: "daily", Time: "07:00", Timezone: "Asia/Kolkata"},
		CreatedAt: now.Add(-48 * time.Hour),
	}
	due := time.Date(2026, 9, 11, 7, 0, 0, 0, loc)

	if got := sch.MissedAt(now); got == nil || !got.Equal(due) {
		t.Errorf("nothing ran at 07:00: missed = %v, want %v", got, due)
	}
	ran := due.Add(20 * time.Second)
	sch.LastRunAt = &ran
	if got := sch.MissedAt(now); got != nil {
		t.Errorf("it ran at 07:00: missed = %v, want nil", got)
	}
	yesterday := due.Add(-24 * time.Hour)
	sch.LastRunAt = &yesterday
	if got := sch.MissedAt(due.Add(5 * time.Minute)); got != nil {
		t.Errorf("five minutes late is not yet missed: %v", got)
	}
	sch.Status = SchedulePaused
	if got := sch.MissedAt(now); got != nil || sch.NextRun(now) != nil {
		t.Errorf("a paused schedule neither misses nor runs next: %v %v", got, sch.NextRun(now))
	}
	sch.Status, sch.CreatedAt = ScheduleActive, now.Add(-30*time.Minute)
	if got := sch.MissedAt(now); got != nil {
		t.Errorf("made after seven: nothing was due yet, missed = %v", got)
	}
}
