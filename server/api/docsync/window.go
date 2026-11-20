package docsync

import (
	"fmt"
	"strings"
	"time"
)

type WallTime struct {
	hour, minute int
}

func ParseWallTime(value string) (WallTime, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, layout := range []string{"3PM", "3:04PM", "15:04"} {
		if t, err := time.Parse(layout, value); err == nil {
			return WallTime{t.Hour(), t.Minute()}, nil
		}
	}
	return WallTime{}, fmt.Errorf("invalid sync time %q", value)
}

func (w WallTime) on(date time.Time) time.Time {
	t := time.Date(date.Year(), date.Month(), date.Day(), w.hour, w.minute, 0, 0, date.Location())
	// In a spring-forward gap, start at the first valid time after the gap.
	for t.Day() == date.Day() && t.Hour()*60+t.Minute() < w.hour*60+w.minute {
		t = t.Add(time.Minute)
	}
	return t
}

type Window struct {
	Start WallTime
	End   WallTime
}

func (w Window) bounds(now time.Time) (start time.Time, end time.Time) {
	start = w.Start.on(now)
	if now.Before(start) {
		start = w.Start.on(now.AddDate(0, 0, -1))
	}
	end = w.End.on(start)
	if !end.After(start) {
		end = w.End.on(start.AddDate(0, 0, 1))
	}
	return start, end
}

func (w Window) Next(now time.Time) time.Time {
	next := w.Start.on(now)
	if !next.After(now) {
		next = w.Start.on(now.AddDate(0, 0, 1))
	}
	return next
}
