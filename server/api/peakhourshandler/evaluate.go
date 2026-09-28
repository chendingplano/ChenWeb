package peakhourshandler

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// EvaluateActive resolves whether p is active at instant at. It converts at
// into p's timezone, checks applicable_days, then exclude_days, then checks
// at's time-of-day against p.Hours. p.Timezone is assumed already validated
// (e.g. via CreatePeakHours/UpdatePeakHours).
func EvaluateActive(ctx context.Context, db *sql.DB, p PeakHours, at time.Time) (bool, error) {
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return false, err
	}
	local := at.In(loc)
	kind := &dayKindLookup{db: db, country: p.Country, date: local}

	applicable := matchesApplicableDays(p.ApplicableDays, local)
	if !applicable && p.ApplicableDays.Mode == "workdays" && isWeekend(local) {
		// A weekend bound as an adjusted working day is a workday.
		adjusted, err := kind.is(ctx, dayKindAdjusted)
		if err != nil {
			return false, err
		}
		applicable = adjusted
	}
	if !applicable {
		return false, nil
	}

	excluded, err := matchesExcludeDays(ctx, kind, p.ExcludeDays, local)
	if err != nil {
		return false, err
	}
	if excluded {
		return false, nil
	}

	minuteOfDay := local.Hour()*60 + local.Minute()
	for _, h := range p.Hours {
		startMin, endMin, err := parseHourRange(h)
		if err != nil {
			continue
		}
		if minuteOfDay >= startMin && minuteOfDay < endMin {
			return true, nil
		}
	}
	return false, nil
}

func matchesApplicableDays(ad ApplicableDays, local time.Time) bool {
	switch ad.Mode {
	case "workdays":
		wd := local.Weekday()
		return wd >= time.Monday && wd <= time.Friday
	case "weekdays":
		for _, d := range ad.Days {
			name, ok := d.(string)
			if !ok {
				continue
			}
			if wd, ok := weekdayNames[strings.ToLower(name)]; ok && wd == local.Weekday() {
				return true
			}
		}
		return false
	case "days_of_month":
		for _, d := range ad.Days {
			n, ok := d.(float64)
			if ok && int(n) == local.Day() {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func isWeekend(local time.Time) bool {
	wd := local.Weekday()
	return wd == time.Sunday || wd == time.Saturday
}

func matchesExcludeDays(ctx context.Context, kind *dayKindLookup, excludeDays []string, local time.Time) (bool, error) {
	dateStr := local.Format("2006-01-02")
	for _, e := range excludeDays {
		switch {
		case e == "weekends":
			if isWeekend(local) {
				adjusted, err := kind.is(ctx, dayKindAdjusted)
				if err != nil {
					return false, err
				}
				if !adjusted {
					return true, nil
				}
			}
		case e == "holidays":
			match, err := kind.is(ctx, dayKindHoliday)
			if err != nil {
				return false, err
			}
			if match {
				return true, nil
			}
		default:
			if start, end, ok := strings.Cut(e, ".."); ok {
				if dateStr >= start && dateStr <= end {
					return true, nil
				}
			} else if e == dateStr {
				return true, nil
			}
		}
	}
	return false, nil
}

const (
	dayKindHoliday  = "holiday"
	dayKindAdjusted = "adjusted"
)

// dayKindLookup resolves, at most once per evaluation, how date is bound in
// country's "holidays" calendar: "holiday", "adjusted" (an adjusted working
// day), or "" when unbound or country is empty.
type dayKindLookup struct {
	db      *sql.DB
	country string
	date    time.Time
	loaded  bool
	kind    string
}

func (l *dayKindLookup) is(ctx context.Context, want string) (bool, error) {
	if !l.loaded {
		kind, err := calendarDayKind(ctx, l.db, l.country, l.date)
		if err != nil {
			return false, err
		}
		l.kind, l.loaded = kind, true
	}
	return l.kind == want, nil
}
