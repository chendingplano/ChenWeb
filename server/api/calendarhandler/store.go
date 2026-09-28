package calendarhandler

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrHolidayInfoMismatch signals a binding to a holiday info whose
// (country, calendar_type) differs from the calendar's.
var ErrHolidayInfoMismatch = errors.New("holiday info belongs to a different country or calendar type")

// ErrHolidayInfoInUse signals a holiday info deletion blocked by an existing
// calendar_holidays binding.
var ErrHolidayInfoInUse = errors.New("holiday info is still bound to a calendar date")

// HolidayInfo is a year-independent holiday definition (public.holiday_info),
// belonging to one (country, calendar_type).
type HolidayInfo struct {
	ID           int64     `json:"id"`
	Country      string    `json:"country"`
	CalendarType string    `json:"calendar_type"`
	Name         string    `json:"name"`
	DisplaySeqno int       `json:"display_seqno"`
	Description  string    `json:"description"`
	Note         string    `json:"note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CalendarDate is one bound date within a calendar (public.calendar_holidays),
// joined with its holiday info name for display.
type CalendarDate struct {
	HolidayDate     string `json:"holiday_date"`
	HolidayInfoID   int64  `json:"holiday_info_id"`
	HolidayInfoName string `json:"holiday_info_name"`
	DayKind         string `json:"day_kind"`
}

// Day kinds stored in calendar_holidays.day_kind.
const (
	DayKindHoliday  = "holiday"
	DayKindAdjusted = "adjusted"
)

// overlappingDate returns the first date present in both lists, or "".
func overlappingDate(holidayDates, adjustedDates []string) string {
	seen := make(map[string]struct{}, len(holidayDates))
	for _, d := range holidayDates {
		seen[d] = struct{}{}
	}
	for _, d := range adjustedDates {
		if _, ok := seen[d]; ok {
			return d
		}
	}
	return ""
}

// Calendar is the (year, country, calendar_type) container plus its bound
// dates. ID is 0 when no public.calendars row exists yet for the key.
type Calendar struct {
	ID           int64          `json:"id"`
	Year         int            `json:"year"`
	Country      string         `json:"country"`
	CalendarType string         `json:"calendar_type"`
	Dates        []CalendarDate `json:"dates"`
}

const holidayInfoColumns = `id, country, calendar_type, name, display_seqno, COALESCE(description, ''), COALESCE(note, ''), created_at, updated_at`

func scanHolidayInfo(scan func(dest ...any) error) (HolidayInfo, error) {
	var h HolidayInfo
	if err := scan(&h.ID, &h.Country, &h.CalendarType, &h.Name, &h.DisplaySeqno, &h.Description, &h.Note, &h.CreatedAt, &h.UpdatedAt); err != nil {
		return HolidayInfo{}, err
	}
	return h, nil
}

// listHolidayInfo lists holiday definitions, optionally filtered by country
// and by calendar type ("" means no filter).
func listHolidayInfo(ctx context.Context, db *sql.DB, country, calendarType string) ([]HolidayInfo, error) {
	query := `SELECT ` + holidayInfoColumns + ` FROM public.holiday_info WHERE ($1 = '' OR country = $1) AND ($2 = '' OR calendar_type = $2)`
	args := []any{country, calendarType}
	query += ` ORDER BY country, calendar_type, display_seqno, id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HolidayInfo{}
	for rows.Next() {
		h, err := scanHolidayInfo(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func createHolidayInfo(ctx context.Context, db *sql.DB, h HolidayInfo) (HolidayInfo, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return HolidayInfo{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		INSERT INTO public.holiday_info (country, calendar_type, name, display_seqno, description, note)
		VALUES ($1, $2, $3, COALESCE((SELECT MAX(display_seqno) + 1 FROM public.holiday_info WHERE country = $1 AND calendar_type = $2), 1), NULLIF($4, ''), NULLIF($5, ''))
		RETURNING `+holidayInfoColumns,
		h.Country, h.CalendarType, h.Name, h.Description, h.Note)
	created, err := scanHolidayInfo(row.Scan)
	if err != nil {
		return HolidayInfo{}, err
	}
	if err := tx.Commit(); err != nil {
		return HolidayInfo{}, err
	}
	return created, nil
}

func updateHolidayInfo(ctx context.Context, db *sql.DB, id int64, h HolidayInfo) (HolidayInfo, error) {
	if h.DisplaySeqno < 1 {
		return HolidayInfo{}, errors.New("display sequence number must be positive")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return HolidayInfo{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Display order is numbered within one (country, calendar_type) group.
	var oldCountry, oldType string
	var oldSeqno int
	if err := tx.QueryRowContext(ctx, `SELECT country, calendar_type, display_seqno FROM public.holiday_info WHERE id = $1 FOR UPDATE`, id).Scan(&oldCountry, &oldType, &oldSeqno); err != nil {
		return HolidayInfo{}, err
	}
	var targetCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.holiday_info WHERE country = $1 AND calendar_type = $2 AND id <> $3`, h.Country, h.CalendarType, id).Scan(&targetCount); err != nil {
		return HolidayInfo{}, err
	}
	maxTargetSeqno := targetCount + 1
	if h.DisplaySeqno > maxTargetSeqno {
		h.DisplaySeqno = maxTargetSeqno
	}

	if _, err := tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno + 1000000000 WHERE id = $1`, id); err != nil {
		return HolidayInfo{}, err
	}
	if oldCountry == h.Country && oldType == h.CalendarType {
		if h.DisplaySeqno < oldSeqno {
			_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno + 1 WHERE country = $1 AND calendar_type = $2 AND display_seqno >= $3 AND display_seqno < $4 AND id <> $5`, oldCountry, oldType, h.DisplaySeqno, oldSeqno, id)
		} else if h.DisplaySeqno > oldSeqno {
			_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno - 1 WHERE country = $1 AND calendar_type = $2 AND display_seqno > $3 AND display_seqno <= $4 AND id <> $5`, oldCountry, oldType, oldSeqno, h.DisplaySeqno, id)
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno - 1 WHERE country = $1 AND calendar_type = $2 AND display_seqno > $3`, oldCountry, oldType, oldSeqno)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno + 1 WHERE country = $1 AND calendar_type = $2 AND display_seqno >= $3`, h.Country, h.CalendarType, h.DisplaySeqno)
		}
	}
	if err != nil {
		return HolidayInfo{}, err
	}

	row := tx.QueryRowContext(ctx, `
		UPDATE public.holiday_info
		SET country = $2, calendar_type = $3, name = $4, display_seqno = $5, description = NULLIF($6, ''), note = NULLIF($7, ''), updated_at = NOW()
		WHERE id = $1
		RETURNING `+holidayInfoColumns,
		id, h.Country, h.CalendarType, h.Name, h.DisplaySeqno, h.Description, h.Note)
	updated, err := scanHolidayInfo(row.Scan)
	if err != nil {
		return HolidayInfo{}, err
	}
	if err := tx.Commit(); err != nil {
		return HolidayInfo{}, err
	}
	return updated, nil
}

type displaySeqRow struct {
	ID      int64
	Country string
	Seqno   int
}

func reorderDisplaySeqnos(rows []displaySeqRow, movingID int64, targetCountry string, targetSeqno int) (map[int64]int, error) {
	if targetSeqno < 1 {
		return nil, errors.New("display sequence number must be positive")
	}
	result := make(map[int64]int, len(rows))
	var oldCountry string
	var oldSeqno int
	for _, row := range rows {
		result[row.ID] = row.Seqno
		if row.ID == movingID {
			oldCountry, oldSeqno = row.Country, row.Seqno
		}
	}
	if oldCountry == "" {
		return nil, sql.ErrNoRows
	}
	if oldCountry == targetCountry {
		if targetSeqno < oldSeqno {
			for _, row := range rows {
				if row.ID != movingID && row.Country == oldCountry && row.Seqno >= targetSeqno && row.Seqno < oldSeqno {
					result[row.ID]++
				}
			}
		} else if targetSeqno > oldSeqno {
			for _, row := range rows {
				if row.ID != movingID && row.Country == oldCountry && row.Seqno > oldSeqno && row.Seqno <= targetSeqno {
					result[row.ID]--
				}
			}
		}
	} else {
		for _, row := range rows {
			if row.ID != movingID && row.Country == oldCountry && row.Seqno > oldSeqno {
				result[row.ID]--
			}
			if row.Country == targetCountry && row.Seqno >= targetSeqno {
				result[row.ID]++
			}
		}
	}
	result[movingID] = targetSeqno
	return result, nil
}

func deleteHolidayInfo(ctx context.Context, db *sql.DB, id int64) error {
	var inUse bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM public.calendar_holidays WHERE holiday_info_id = $1)`, id).Scan(&inUse); err != nil {
		return err
	}
	if inUse {
		return ErrHolidayInfoInUse
	}
	res, err := db.ExecContext(ctx, `DELETE FROM public.holiday_info WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// getCalendar returns the calendar for (year, country, calendarType) with its
// bound dates, or a zero-ID Calendar if no row exists yet.
func getCalendar(ctx context.Context, db *sql.DB, year int, country, calendarType string) (Calendar, error) {
	cal := Calendar{Year: year, Country: country, CalendarType: calendarType, Dates: []CalendarDate{}}
	err := db.QueryRowContext(ctx, `
		SELECT id FROM public.calendars WHERE year = $1 AND country = $2 AND calendar_type = $3`,
		year, country, calendarType).Scan(&cal.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return cal, nil
	}
	if err != nil {
		return Calendar{}, err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT ch.holiday_date, ch.holiday_info_id, hi.name, ch.day_kind
		FROM public.calendar_holidays ch
		JOIN public.holiday_info hi ON hi.id = ch.holiday_info_id
		WHERE ch.calendar_id = $1
		ORDER BY ch.holiday_date`, cal.ID)
	if err != nil {
		return Calendar{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var d CalendarDate
		var holidayDate time.Time
		if err := rows.Scan(&holidayDate, &d.HolidayInfoID, &d.HolidayInfoName, &d.DayKind); err != nil {
			return Calendar{}, err
		}
		d.HolidayDate = holidayDate.Format("2006-01-02")
		cal.Dates = append(cal.Dates, d)
	}
	return cal, rows.Err()
}

// createCalendar creates the calendars row for (year, country, calendarType)
// with no dates. It is idempotent: an existing row is kept as is.
func createCalendar(ctx context.Context, db *sql.DB, year int, country, calendarType string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO public.calendars (year, country, calendar_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (year, country, calendar_type) DO NOTHING`, year, country, calendarType)
	return err
}

// upsertCalendarDates creates the calendars row if missing, then upserts one
// calendar_holidays binding per date (holidayDates as day kind "holiday",
// adjustedDates as "adjusted"), replacing any existing binding for that date.
// Returns the calendar id.
func upsertCalendarDates(ctx context.Context, db *sql.DB, year int, country, calendarType string, holidayDates, adjustedDates []string, holidayInfoID int64) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var infoCountry, infoType string
	if err := tx.QueryRowContext(ctx, `SELECT country, calendar_type FROM public.holiday_info WHERE id = $1`, holidayInfoID).Scan(&infoCountry, &infoType); err != nil {
		return 0, err
	}
	if infoCountry != country || infoType != calendarType {
		return 0, ErrHolidayInfoMismatch
	}

	var calendarID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO public.calendars (year, country, calendar_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (year, country, calendar_type) DO UPDATE SET updated_at = NOW()
		RETURNING id`, year, country, calendarType).Scan(&calendarID)
	if err != nil {
		return 0, err
	}

	upsert := func(dates []string, dayKind string) error {
		for _, date := range dates {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO public.calendar_holidays (calendar_id, holiday_info_id, holiday_date, day_kind)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (calendar_id, holiday_date) DO UPDATE SET
					holiday_info_id = EXCLUDED.holiday_info_id,
					day_kind = EXCLUDED.day_kind,
					updated_at = NOW()`, calendarID, holidayInfoID, date, dayKind); err != nil {
				return err
			}
		}
		return nil
	}
	if err := upsert(holidayDates, DayKindHoliday); err != nil {
		return 0, err
	}
	if err := upsert(adjustedDates, DayKindAdjusted); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return calendarID, nil
}

func deleteCalendarDate(ctx context.Context, db *sql.DB, calendarID int64, date string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM public.calendar_holidays WHERE calendar_id = $1 AND holiday_date = $2`, calendarID, date)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func deleteCalendar(ctx context.Context, db *sql.DB, calendarID int64) error {
	res, err := db.ExecContext(ctx, `DELETE FROM public.calendars WHERE id = $1`, calendarID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// getDefaultCountry returns the configured default country, or "" if none is set.
func getDefaultCountry(ctx context.Context, db *sql.DB) (string, error) {
	var country string
	err := db.QueryRowContext(ctx, `SELECT country FROM public.calendar_default_country WHERE id = 1`).Scan(&country)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return country, err
}

// setDefaultCountry replaces the singleton default-country row.
func setDefaultCountry(ctx context.Context, db *sql.DB, country string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO public.calendar_default_country (id, country)
		VALUES (1, $1)
		ON CONFLICT (id) DO UPDATE SET country = EXCLUDED.country, updated_at = NOW()`, country)
	return err
}

// clearDefaultCountry removes the singleton default-country row, if any.
func clearDefaultCountry(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM public.calendar_default_country WHERE id = 1`)
	return err
}
