package peakhourshandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrPeakHoursInvalid signals a validation failure on a peak hours payload.
	ErrPeakHoursInvalid = errors.New("invalid peak hours definition")
	// ErrPeakHoursNotFound signals no peak hours record matches the given name.
	ErrPeakHoursNotFound = errors.New("peak hours not found")
)

// ApplicableDays is the discriminated applicable-days rule for a peak hours
// definition. Mode "workdays" ignores Days. Mode "weekdays" expects Days to
// hold lowercase 3-letter weekday names (mon..sun). Mode "days_of_month"
// expects Days to hold day-of-month numbers (1-31), encoded as JSON numbers.
type ApplicableDays struct {
	Mode string        `json:"mode"`
	Days []interface{} `json:"days,omitempty"`
}

// PeakHours is one named peak-hours definition (public.peak_hours).
type PeakHours struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	Hours          []string       `json:"hours"`
	Timezone       string         `json:"timezone"`
	ApplicableDays ApplicableDays `json:"applicable_days"`
	ExcludeDays    []string       `json:"exclude_days"`
	Country        string         `json:"country"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

var weekdayNames = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

// parseHourRange parses "HH:MM-HH:MM" into minutes-since-midnight bounds.
func parseHourRange(s string) (startMin, endMin int, err error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("must be HH:MM-HH:MM")
	}
	startMin, err = parseClock(parts[0])
	if err != nil {
		return 0, 0, err
	}
	endMin, err = parseClock(parts[1])
	if err != nil {
		return 0, 0, err
	}
	if startMin >= endMin {
		return 0, 0, fmt.Errorf("start must be before end")
	}
	return startMin, endMin, nil
}

func parseClock(s string) (int, error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return 0, fmt.Errorf("must be HH:MM (2-digit hour and minute)")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 23 {
		return 0, fmt.Errorf("hour must be 00-23")
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return 0, fmt.Errorf("minute must be 00-59")
	}
	return h*60 + m, nil
}

// validate checks all fields of a peak hours definition, trimming string
// fields in place. It does not touch ID/CreatedAt/UpdatedAt.
func validate(p *PeakHours) error {
	p.Name = strings.TrimSpace(p.Name)
	p.Timezone = strings.TrimSpace(p.Timezone)
	p.Country = strings.TrimSpace(p.Country)

	if p.Name == "" {
		return fmt.Errorf("%w: name is required", ErrPeakHoursInvalid)
	}
	if len(p.Hours) == 0 {
		return fmt.Errorf("%w: at least one hours range is required", ErrPeakHoursInvalid)
	}
	for _, h := range p.Hours {
		if _, _, err := parseHourRange(h); err != nil {
			return fmt.Errorf("%w: hours entry %q: %s", ErrPeakHoursInvalid, h, err)
		}
	}
	if p.Timezone == "" {
		return fmt.Errorf("%w: timezone is required", ErrPeakHoursInvalid)
	}
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return fmt.Errorf("%w: timezone %q is not a valid IANA zone", ErrPeakHoursInvalid, p.Timezone)
	}
	if err := validateApplicableDays(p.ApplicableDays); err != nil {
		return err
	}
	for _, e := range p.ExcludeDays {
		if err := validateExcludeEntry(e); err != nil {
			return err
		}
	}
	return nil
}

func validateApplicableDays(ad ApplicableDays) error {
	switch ad.Mode {
	case "workdays":
		return nil
	case "weekdays":
		if len(ad.Days) == 0 {
			return fmt.Errorf("%w: applicable_days weekdays mode requires at least one day", ErrPeakHoursInvalid)
		}
		for _, d := range ad.Days {
			name, ok := d.(string)
			if !ok {
				return fmt.Errorf("%w: applicable_days weekdays entries must be strings", ErrPeakHoursInvalid)
			}
			if _, ok := weekdayNames[strings.ToLower(name)]; !ok {
				return fmt.Errorf("%w: %q is not a valid weekday (mon..sun)", ErrPeakHoursInvalid, name)
			}
		}
		return nil
	case "days_of_month":
		if len(ad.Days) == 0 {
			return fmt.Errorf("%w: applicable_days days_of_month mode requires at least one day", ErrPeakHoursInvalid)
		}
		for _, d := range ad.Days {
			n, ok := d.(float64)
			if !ok || n != float64(int(n)) || n < 1 || n > 31 {
				return fmt.Errorf("%w: applicable_days days_of_month entries must be whole numbers 1-31", ErrPeakHoursInvalid)
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: applicable_days.mode must be workdays, weekdays, or days_of_month", ErrPeakHoursInvalid)
	}
}

func validateExcludeEntry(e string) error {
	if e == "weekends" || e == "holidays" {
		return nil
	}
	if start, end, ok := strings.Cut(e, ".."); ok {
		if _, err := time.Parse("2006-01-02", start); err != nil {
			return fmt.Errorf("%w: exclude_days entry %q has an invalid range start", ErrPeakHoursInvalid, e)
		}
		if _, err := time.Parse("2006-01-02", end); err != nil {
			return fmt.Errorf("%w: exclude_days entry %q has an invalid range end", ErrPeakHoursInvalid, e)
		}
		return nil
	}
	if _, err := time.Parse("2006-01-02", e); err != nil {
		return fmt.Errorf("%w: exclude_days entry %q must be \"weekends\", \"holidays\", an ISO date, or an ISO date range", ErrPeakHoursInvalid, e)
	}
	return nil
}

const peakHoursColumns = `id, name, hours, timezone, applicable_days, exclude_days, country, created_at, updated_at`

func scanPeakHours(scan func(dest ...any) error) (PeakHours, error) {
	var (
		p                    PeakHours
		hoursRaw, excludeRaw []byte
		applicableRaw        []byte
	)
	if err := scan(&p.ID, &p.Name, &hoursRaw, &p.Timezone, &applicableRaw, &excludeRaw, &p.Country, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return PeakHours{}, err
	}
	_ = json.Unmarshal(hoursRaw, &p.Hours)
	_ = json.Unmarshal(applicableRaw, &p.ApplicableDays)
	_ = json.Unmarshal(excludeRaw, &p.ExcludeDays)
	if p.Hours == nil {
		p.Hours = []string{}
	}
	if p.ExcludeDays == nil {
		p.ExcludeDays = []string{}
	}
	return p, nil
}

// CreatePeakHours validates and inserts a new peak hours definition.
func CreatePeakHours(ctx context.Context, db *sql.DB, p PeakHours) (PeakHours, error) {
	if err := validate(&p); err != nil {
		return PeakHours{}, err
	}
	hours, _ := json.Marshal(p.Hours)
	applicable, _ := json.Marshal(p.ApplicableDays)
	exclude, _ := json.Marshal(orEmptyStrings(p.ExcludeDays))
	row := db.QueryRowContext(ctx, `
		INSERT INTO public.peak_hours (name, hours, timezone, applicable_days, exclude_days, country)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+peakHoursColumns,
		p.Name, hours, p.Timezone, applicable, exclude, p.Country)
	return scanPeakHours(row.Scan)
}

// GetPeakHoursByName retrieves a peak hours definition by its name.
func GetPeakHoursByName(ctx context.Context, db *sql.DB, name string) (PeakHours, error) {
	row := db.QueryRowContext(ctx, `SELECT `+peakHoursColumns+` FROM public.peak_hours WHERE name = $1`, name)
	p, err := scanPeakHours(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PeakHours{}, ErrPeakHoursNotFound
	}
	return p, err
}

// ListPeakHours lists all peak hours definitions, ordered by name.
func ListPeakHours(ctx context.Context, db *sql.DB) ([]PeakHours, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+peakHoursColumns+` FROM public.peak_hours ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PeakHours, 0)
	for rows.Next() {
		p, err := scanPeakHours(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpdatePeakHours validates and updates an existing peak hours definition by
// name. The name itself is immutable.
func UpdatePeakHours(ctx context.Context, db *sql.DB, name string, p PeakHours) (PeakHours, error) {
	p.Name = strings.TrimSpace(name)
	if err := validate(&p); err != nil {
		return PeakHours{}, err
	}
	hours, _ := json.Marshal(p.Hours)
	applicable, _ := json.Marshal(p.ApplicableDays)
	exclude, _ := json.Marshal(orEmptyStrings(p.ExcludeDays))
	row := db.QueryRowContext(ctx, `
		UPDATE public.peak_hours
		SET hours = $2, timezone = $3, applicable_days = $4, exclude_days = $5, country = $6, updated_at = NOW()
		WHERE name = $1
		RETURNING `+peakHoursColumns,
		p.Name, hours, p.Timezone, applicable, exclude, p.Country)
	updated, err := scanPeakHours(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return PeakHours{}, ErrPeakHoursNotFound
	}
	return updated, err
}

// DeletePeakHours removes a peak hours definition by name.
func DeletePeakHours(ctx context.Context, db *sql.DB, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM public.peak_hours WHERE name = $1`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrPeakHoursNotFound
	}
	return nil
}

func orEmptyStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// isHoliday reports whether date (in the record's timezone) is a bound
// holiday date for country under the default "holidays" calendar type. It
// returns false, not an error, when country is empty or no matching
// calendar row exists.
func isHoliday(ctx context.Context, db *sql.DB, country string, date time.Time) (bool, error) {
	if country == "" {
		return false, nil
	}
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM public.calendar_holidays ch
			JOIN public.calendars c ON c.id = ch.calendar_id
			WHERE c.year = $1 AND c.country = $2 AND c.calendar_type = 'holidays'
			  AND ch.holiday_date = $3
		)`, date.Year(), country, date.Format("2006-01-02")).Scan(&exists)
	return exists, err
}
