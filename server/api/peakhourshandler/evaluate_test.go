package peakhourshandler

import (
	"context"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func rx(s string) string { return regexp.QuoteMeta(s) }

// installMockDB swaps ApiTypes.ProjectDBHandle for a sqlmock-backed *sql.DB
// for the duration of the test, restoring the original on cleanup.
func installMockDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = old; _ = db.Close() })
	return mock
}

// deepSeekExample is the worked example from the proposal: 9:00am-12:00pm and
// 2:00pm-6:00pm, Asia/Shanghai, workdays, excluding holidays and weekends.
func deepSeekExample() PeakHours {
	return PeakHours{
		Name:           "deepseek",
		Hours:          []string{"09:00-12:00", "14:00-18:00"},
		Timezone:       "Asia/Shanghai",
		ApplicableDays: ApplicableDays{Mode: "workdays"},
		ExcludeDays:    []string{"holidays", "weekends"},
		Country:        "CN",
	}
}

func mustParse(t *testing.T, rfc3339 string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("parse %q: %v", rfc3339, err)
	}
	return tm
}

func TestEvaluateActive_DeepSeekExample_ActiveWithinMorningWindow(t *testing.T) {
	mock := installMockDB(t)
	p := deepSeekExample()
	// 2026-09-21 is a Monday.
	at := mustParse(t, "2026-09-21T10:00:00+08:00")

	mock.ExpectQuery(rx("FROM public.calendar_holidays ch")).
		WithArgs(2026, "CN", "2026-09-21").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if !active {
		t.Fatalf("active = false, want true (10:00 Monday, within 09:00-12:00)")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB calls: %v", err)
	}
}

func TestEvaluateActive_DeepSeekExample_InactiveBetweenWindows(t *testing.T) {
	mock := installMockDB(t)
	p := deepSeekExample()
	// 2026-09-21 is a Monday; 13:00 is between the morning and afternoon windows.
	at := mustParse(t, "2026-09-21T13:00:00+08:00")

	mock.ExpectQuery(rx("FROM public.calendar_holidays ch")).
		WithArgs(2026, "CN", "2026-09-21").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if active {
		t.Fatalf("active = true, want false (13:00 Monday, between windows)")
	}
}

func TestEvaluateActive_DeepSeekExample_InactiveOnWeekend(t *testing.T) {
	installMockDB(t)
	p := deepSeekExample()
	// 2026-09-20 is a Sunday.
	at := mustParse(t, "2026-09-20T10:00:00+08:00")

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if active {
		t.Fatalf("active = true, want false (Sunday is excluded as a weekend)")
	}
}

func TestEvaluateActive_DeepSeekExample_InactiveOnHoliday(t *testing.T) {
	mock := installMockDB(t)
	p := deepSeekExample()
	// 2026-10-01 is a Thursday (a workday) but a configured CN holiday.
	at := mustParse(t, "2026-10-01T10:00:00+08:00")

	mock.ExpectQuery(rx("FROM public.calendar_holidays ch")).
		WithArgs(2026, "CN", "2026-10-01").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if active {
		t.Fatalf("active = true, want false (2026-10-01 is a configured holiday)")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB calls: %v", err)
	}
}

func TestEvaluateActive_HolidaysExclusionWithNoCountry_DoesNotExcludeAnything(t *testing.T) {
	installMockDB(t)
	p := deepSeekExample()
	p.Country = ""
	// 2026-09-21 is a Monday within the morning window; no country means
	// "holidays" cannot match, so this stays active.
	at := mustParse(t, "2026-09-21T10:00:00+08:00")

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if !active {
		t.Fatalf("active = false, want true (empty country should not error or wrongly exclude)")
	}
}

func TestEvaluateActive_InactiveOnNonApplicableDay(t *testing.T) {
	installMockDB(t)
	p := PeakHours{
		Name:           "weekday-window",
		Hours:          []string{"09:00-17:00"},
		Timezone:       "UTC",
		ApplicableDays: ApplicableDays{Mode: "weekdays", Days: []interface{}{"mon", "wed", "fri"}},
	}
	// 2026-09-22 is a Tuesday, not in the applicable-days list.
	at := mustParse(t, "2026-09-22T10:00:00Z")

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if active {
		t.Fatalf("active = true, want false (Tuesday not in weekdays list)")
	}
}

func TestEvaluateActive_DaysOfMonth(t *testing.T) {
	installMockDB(t)
	p := PeakHours{
		Name:           "month-end",
		Hours:          []string{"09:00-17:00"},
		Timezone:       "UTC",
		ApplicableDays: ApplicableDays{Mode: "days_of_month", Days: []interface{}{float64(1), float64(15)}},
	}
	active15, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, mustParse(t, "2026-09-15T10:00:00Z"))
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if !active15 {
		t.Fatalf("active = false, want true (day 15 is in days_of_month)")
	}
	active16, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, mustParse(t, "2026-09-16T10:00:00Z"))
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if active16 {
		t.Fatalf("active = true, want false (day 16 is not in days_of_month)")
	}
}

func TestEvaluateActive_ExcludeSpecificDateRange(t *testing.T) {
	installMockDB(t)
	p := deepSeekExample()
	p.ExcludeDays = []string{"2026-12-24..2026-12-31"}
	// 2026-12-28 is a Monday within the excluded range.
	at := mustParse(t, "2026-12-28T10:00:00+08:00")

	active, err := EvaluateActive(context.Background(), ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		t.Fatalf("EvaluateActive: %v", err)
	}
	if active {
		t.Fatalf("active = true, want false (2026-12-28 is within the excluded range)")
	}
}
