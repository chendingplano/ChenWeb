package calendarhandler

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrHolidayInfoInUse signals a holiday info deletion blocked by an existing
// calendar_holidays binding.
var ErrHolidayInfoInUse = errors.New("holiday info is still bound to a calendar date")

// HolidayInfo is a year-independent holiday definition (public.holiday_info).
type HolidayInfo struct {
	ID           int64     `json:"id"`
	Country      string    `json:"country"`
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

const holidayInfoColumns = `id, country, name, display_seqno, COALESCE(description, ''), COALESCE(note, ''), created_at, updated_at`

func scanHolidayInfo(scan func(dest ...any) error) (HolidayInfo, error) {
	var h HolidayInfo
	if err := scan(&h.ID, &h.Country, &h.Name, &h.DisplaySeqno, &h.Description, &h.Note, &h.CreatedAt, &h.UpdatedAt); err != nil {
		return HolidayInfo{}, err
	}
	return h, nil
}

func listHolidayInfo(ctx context.Context, db *sql.DB, country string) ([]HolidayInfo, error) {
	query := `SELECT ` + holidayInfoColumns + ` FROM public.holiday_info`
	args := []any{}
	if country != "" {
		query += ` WHERE country = $1`
		args = append(args, country)
	}
	query += ` ORDER BY country, display_seqno, id`
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
		INSERT INTO public.holiday_info (country, name, display_seqno, description, note)
		VALUES ($1, $2, COALESCE((SELECT MAX(display_seqno) + 1 FROM public.holiday_info WHERE country = $1), 1), NULLIF($3, ''), NULLIF($4, ''))
		RETURNING `+holidayInfoColumns,
		h.Country, h.Name, h.Description, h.Note)
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

	var oldCountry string
	var oldSeqno int
	if err := tx.QueryRowContext(ctx, `SELECT country, display_seqno FROM public.holiday_info WHERE id = $1 FOR UPDATE`, id).Scan(&oldCountry, &oldSeqno); err != nil {
		return HolidayInfo{}, err
	}
	var targetCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM public.holiday_info WHERE country = $1 AND id <> $2`, h.Country, id).Scan(&targetCount); err != nil {
		return HolidayInfo{}, err
	}
	maxTargetSeqno := targetCount + 1
	if h.DisplaySeqno > maxTargetSeqno {
		h.DisplaySeqno = maxTargetSeqno
	}

	if _, err := tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno + 1000000000 WHERE id = $1`, id); err != nil {
		return HolidayInfo{}, err
	}
	if oldCountry == h.Country {
		if h.DisplaySeqno < oldSeqno {
			_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno + 1 WHERE country = $1 AND display_seqno >= $2 AND display_seqno < $3 AND id <> $4`, oldCountry, h.DisplaySeqno, oldSeqno, id)
		} else if h.DisplaySeqno > oldSeqno {
			_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno - 1 WHERE country = $1 AND display_seqno > $2 AND display_seqno <= $3 AND id <> $4`, oldCountry, oldSeqno, h.DisplaySeqno, id)
		}
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno - 1 WHERE country = $1 AND display_seqno > $2`, oldCountry, oldSeqno)
		if err == nil {
			_, err = tx.ExecContext(ctx, `UPDATE public.holiday_info SET display_seqno = display_seqno + 1 WHERE country = $1 AND display_seqno >= $2`, h.Country, h.DisplaySeqno)
		}
	}
	if err != nil {
		return HolidayInfo{}, err
	}

	row := tx.QueryRowContext(ctx, `
		UPDATE public.holiday_info
		SET country = $2, name = $3, display_seqno = $4, description = NULLIF($5, ''), note = NULLIF($6, ''), updated_at = NOW()
		WHERE id = $1
		RETURNING `+holidayInfoColumns,
		id, h.Country, h.Name, h.DisplaySeqno, h.Description, h.Note)
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
		SELECT ch.holiday_date, ch.holiday_info_id, hi.name
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
		if err := rows.Scan(&holidayDate, &d.HolidayInfoID, &d.HolidayInfoName); err != nil {
			return Calendar{}, err
		}
		d.HolidayDate = holidayDate.Format("2006-01-02")
		cal.Dates = append(cal.Dates, d)
	}
	return cal, rows.Err()
}

// upsertCalendarDates creates the calendars row if missing, then upserts one
// calendar_holidays binding per date, replacing any existing binding for that
// date. Returns the calendar id.
func upsertCalendarDates(ctx context.Context, db *sql.DB, year int, country, calendarType string, dates []string, holidayInfoID int64) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var calendarID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO public.calendars (year, country, calendar_type)
		VALUES ($1, $2, $3)
		ON CONFLICT (year, country, calendar_type) DO UPDATE SET updated_at = NOW()
		RETURNING id`, year, country, calendarType).Scan(&calendarID)
	if err != nil {
		return 0, err
	}

	for _, date := range dates {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO public.calendar_holidays (calendar_id, holiday_info_id, holiday_date)
			VALUES ($1, $2, $3)
			ON CONFLICT (calendar_id, holiday_date) DO UPDATE SET
				holiday_info_id = EXCLUDED.holiday_info_id,
				updated_at = NOW()`, calendarID, holidayInfoID, date); err != nil {
			return 0, err
		}
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
