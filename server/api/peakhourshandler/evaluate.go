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

	if !matchesApplicableDays(p.ApplicableDays, local) {
		return false, nil
	}

	excluded, err := matchesExcludeDays(ctx, db, p.ExcludeDays, p.Country, local)
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

func matchesExcludeDays(ctx context.Context, db *sql.DB, excludeDays []string, country string, local time.Time) (bool, error) {
	dateStr := local.Format("2006-01-02")
	wd := local.Weekday()
	for _, e := range excludeDays {
		switch {
		case e == "weekends":
			if wd == time.Sunday || wd == time.Saturday {
				return true, nil
			}
		case e == "holidays":
			match, err := isHoliday(ctx, db, country, local)
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
