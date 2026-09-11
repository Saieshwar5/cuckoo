package conversations

import (
	"fmt"
	"slices"
	"strings"
	"time"
	// The hub's image carries no zone files of its own; the list is small
	// enough to take along, and "Asia/Kolkata" must mean the same on every
	// machine.
	_ "time/tzdata"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
)

// Cadence is when a schedule runs, as structure rather than a sentence.
//
// A person picks a time and a repeat on their phone; nothing reads "7" and
// decides it meant the evening. The agent interprets what to do, never when.
type Cadence struct {
	// Repeat is once, daily, weekdays or weekly.
	Repeat string `json:"repeat"`
	// Time is the hour and minute, "07:00", on a 24-hour clock, in Timezone.
	Time string `json:"time"`
	// Days, for weekly only: "mon" … "sun".
	Days []string `json:"days,omitempty"`
	// Date, for once only: "2026-09-12".
	Date string `json:"date,omitempty"`
	// Timezone is where the clock is: "Asia/Kolkata".
	Timezone string `json:"timezone"`
}

// How often a schedule repeats. Nothing more often than daily on purpose:
// hourly is where a person's own schedule turns into the agent's spam.
const (
	RepeatOnce     = "once"
	RepeatDaily    = "daily"
	RepeatWeekdays = "weekdays"
	RepeatWeekly   = "weekly"
)

var weekdays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

// missGrace is how late a run may be before the app says it did not happen.
const missGrace = 10 * time.Minute

func invalidCadence(msg string) error {
	return domain.InvalidField("cadence", "invalid_cadence", msg)
}

// normalized checks a cadence and returns it in its one written form: days
// lower-case, deduplicated and in week order.
func (c Cadence) normalized() (Cadence, error) {
	switch c.Repeat {
	case RepeatOnce, RepeatDaily, RepeatWeekdays, RepeatWeekly:
	default:
		return Cadence{}, invalidCadence(`repeat must be "once", "daily", "weekdays" or "weekly".`)
	}
	if _, err := time.Parse("15:04", c.Time); err != nil || len(c.Time) != 5 {
		return Cadence{}, invalidCadence(`time must be an hour and minute on a 24-hour clock, like "07:00".`)
	}
	if c.Timezone == "" {
		return Cadence{}, invalidCadence(`timezone is needed, like "Asia/Kolkata".`)
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return Cadence{}, invalidCadence(fmt.Sprintf("%q is not a time zone.", c.Timezone))
	}

	out := Cadence{Repeat: c.Repeat, Time: c.Time, Timezone: c.Timezone}
	if c.Repeat == RepeatWeekly {
		for _, d := range weekdays {
			for _, given := range c.Days {
				if strings.EqualFold(strings.TrimSpace(given), d) && !slices.Contains(out.Days, d) {
					out.Days = append(out.Days, d)
				}
			}
		}
		if len(out.Days) == 0 || len(out.Days) != len(uniqueLower(c.Days)) {
			return Cadence{}, invalidCadence(`a weekly schedule names its days, from "mon" to "sun".`)
		}
	} else if len(c.Days) > 0 {
		return Cadence{}, invalidCadence("only a weekly schedule names days.")
	}
	if c.Repeat == RepeatOnce {
		if _, err := time.Parse("2006-01-02", c.Date); err != nil {
			return Cadence{}, invalidCadence(`a one-off schedule has a date, like "2026-09-12".`)
		}
		out.Date = c.Date
	} else if c.Date != "" {
		return Cadence{}, invalidCadence("only a one-off schedule has a date.")
	}
	return out, nil
}

func uniqueLower(days []string) []string {
	var out []string
	for _, d := range days {
		d = strings.ToLower(strings.TrimSpace(d))
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// allows says whether the cadence runs on a day of the week.
func (c Cadence) allows(day time.Weekday) bool {
	switch c.Repeat {
	case RepeatWeekdays:
		return day >= time.Monday && day <= time.Friday
	case RepeatWeekly:
		return slices.Contains(c.Days, weekdays[day])
	default:
		return true
	}
}

// at is the cadence's clock time on the local day `days` from t's.
func (c Cadence) at(t time.Time, days int, loc *time.Location) time.Time {
	clock, _ := time.Parse("15:04", c.Time)
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day()+days, clock.Hour(), clock.Minute(), 0, 0, loc)
}

// Next is the first time the cadence runs after t, if it ever will.
func (c Cadence) Next(t time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Time{}, false
	}
	if c.Repeat == RepeatOnce {
		once, ok := c.once(loc)
		return once, ok && once.After(t)
	}
	for days := 0; days <= 8; days++ {
		if run := c.at(t, days, loc); run.After(t) && c.allows(run.Weekday()) {
			return run, true
		}
	}
	return time.Time{}, false
}

// Previous is the last time the cadence was due at or before t, if ever.
func (c Cadence) Previous(t time.Time) (time.Time, bool) {
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return time.Time{}, false
	}
	if c.Repeat == RepeatOnce {
		once, ok := c.once(loc)
		return once, ok && !once.After(t)
	}
	for days := 0; days >= -8; days-- {
		if run := c.at(t, days, loc); !run.After(t) && c.allows(run.Weekday()) {
			return run, true
		}
	}
	return time.Time{}, false
}

func (c Cadence) once(loc *time.Location) (time.Time, bool) {
	day, err := time.Parse("2006-01-02", c.Date)
	if err != nil {
		return time.Time{}, false
	}
	clock, _ := time.Parse("15:04", c.Time)
	return time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, loc), true
}
