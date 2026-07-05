package llmreporthandler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDailyUsageReportRunnerRunAggregatesYesterdayAndToday(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	loc := time.UTC
	now := time.Date(2026, 6, 20, 10, 0, 0, 0, loc)
	stmt := regexp.QuoteMeta(`INSERT INTO llm_daily_account_report (
    account_id, workspace_day, timezone_name, opening_balance, closing_balance, spend_amount, currency_code,
    input_tokens, output_tokens, total_tokens, request_count, reconciliation_status, source_kind
)
SELECT
    account_id,
    $1,
    $2,
    0,
    0,
    0,
    'USD',
    COALESCE(SUM(input_tokens), 0),
    COALESCE(SUM(output_tokens), 0),
    COALESCE(SUM(total_tokens), 0),
    COUNT(*),
    'usage_aggregated',
    'usage_events'
FROM llm_usage_event
WHERE workspace_day = $1
  AND account_id IS NOT NULL
GROUP BY account_id
ON CONFLICT (account_id, workspace_day) DO UPDATE SET
    timezone_name = EXCLUDED.timezone_name,
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    total_tokens = EXCLUDED.total_tokens,
    request_count = EXCLUDED.request_count,
    reconciliation_status = CASE WHEN llm_daily_account_report.reconciliation_status = 'provider_verified' THEN llm_daily_account_report.reconciliation_status ELSE EXCLUDED.reconciliation_status END,
    source_kind = CASE WHEN llm_daily_account_report.reconciliation_status = 'provider_verified' THEN llm_daily_account_report.source_kind ELSE EXCLUDED.source_kind END,
    updated_at = NOW()`)
	mock.ExpectExec(stmt).
		WithArgs(time.Date(2026, 6, 19, 0, 0, 0, 0, loc), "UTC").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(stmt).
		WithArgs(time.Date(2026, 6, 20, 0, 0, 0, 0, loc), "UTC").
		WillReturnResult(sqlmock.NewResult(0, 1))

	runner := &DailyUsageReportRunner{
		DB:           db,
		WorkspaceTZ:  loc,
		TimezoneName: "UTC",
		Now:          func() time.Time { return now },
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestNextDailyReportRunAtUsesWorkspaceTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	now := time.Date(2026, 6, 20, 8, 15, 0, 0, time.UTC)

	got := nextDailyReportRunAt(now, loc, 2)
	want := time.Date(2026, 6, 20, 2, 0, 0, 0, loc).Add(24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("nextDailyReportRunAt() = %v, want %v", got, want)
	}
}
