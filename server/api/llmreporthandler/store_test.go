package llmreporthandler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreListDailyReports(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"account_id", "account_name", "workspace_day", "timezone_name", "opening_balance", "closing_balance",
		"spend_amount", "currency_code", "input_tokens", "output_tokens", "total_tokens",
		"request_count", "reconciliation_status",
	}).AddRow(
		"acct_1", "deepseek:api.deepseek.com", time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC), "America/Chicago",
		25.5, 19.25, 6.25, "USD", 1000, 250, 1250, 8, "provider_verified",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT report.account_id, acct.account_name, report.workspace_day, report.timezone_name, report.opening_balance, report.closing_balance,
spend_amount, currency_code, input_tokens, output_tokens, total_tokens, request_count, reconciliation_status
FROM llm_daily_account_report report
JOIN llm_account acct ON acct.id = report.account_id
ORDER BY report.workspace_day DESC, acct.account_name ASC
LIMIT $1`)).WithArgs(30).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListDailyReports(context.Background(), 30)
	if err != nil {
		t.Fatalf("ListDailyReports() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListDailyReports()) = %d, want 1", len(got))
	}
	if got[0].SpendAmount != 6.25 || got[0].RequestCount != 8 || got[0].AccountName != "deepseek:api.deepseek.com" {
		t.Fatalf("unexpected report = %+v", got[0])
	}
}

func TestStoreListUsageEvents(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "account_id", "account_name", "profile_id", "record_id", "provider", "model_name", "prompt_name", "call_reason", "call_loc",
		"request_started_at", "input_tokens", "output_tokens", "total_tokens", "latency_ms", "error_message",
	}).AddRow(
		"evt_1", "acct_1", "deepseek:api.deepseek.com", "prof_1", 88, "deepseek", "deepseek-v4-flash", "extract-products-v2", "extract_products", "MID-CWB-TEST",
		time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC), 100, 25, 125, 3200, "",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT evt.id, evt.account_id, acct.account_name, evt.profile_id, evt.record_id, evt.provider, evt.model_name, evt.prompt_name,
evt.call_reason, evt.call_loc, request_started_at, input_tokens, output_tokens, total_tokens, latency_ms, error_message
FROM llm_usage_event evt
JOIN llm_account acct ON acct.id = evt.account_id
ORDER BY evt.request_started_at DESC
LIMIT $1`)).WithArgs(50).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListUsageEvents(context.Background(), 50)
	if err != nil {
		t.Fatalf("ListUsageEvents() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListUsageEvents()) = %d, want 1", len(got))
	}
	if got[0].ModelName != "deepseek-v4-flash" || got[0].TotalTokens != 125 || got[0].AccountName != "deepseek:api.deepseek.com" || got[0].RecordID == nil || *got[0].RecordID != 88 || got[0].CallLoc != "MID-CWB-TEST" {
		t.Fatalf("unexpected usage event = %+v", got[0])
	}
}

func TestStoreListModelActivityReports(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"provider", "model_name", "currency_code", "workspace_day", "spend_amount", "input_tokens", "output_tokens", "total_tokens", "request_count",
	}).AddRow(
		"deepseek", "deepseek-v4-flash", "CNY", "2026-06-20", 11.88, int64(2925804), int64(3975685), int64(6901489), int64(1380),
	)

	mock.ExpectQuery(regexp.QuoteMeta(`WITH recent_days AS (
    SELECT DISTINCT workspace_day
    FROM llm_daily_account_report
    ORDER BY workspace_day DESC
    LIMIT $1
),
model_usage AS (
    SELECT
        evt.workspace_day,
        evt.account_id,
        evt.provider,
        evt.model_name,
        COALESCE(SUM(evt.input_tokens), 0) AS input_tokens,
        COALESCE(SUM(evt.output_tokens), 0) AS output_tokens,
        COALESCE(SUM(evt.total_tokens), 0) AS total_tokens,
        COUNT(*) AS request_count
    FROM llm_usage_event evt
    JOIN recent_days days ON days.workspace_day = evt.workspace_day
    GROUP BY evt.workspace_day, evt.account_id, evt.provider, evt.model_name
),
account_day_totals AS (
    SELECT account_id, workspace_day, COALESCE(SUM(total_tokens), 0) AS account_total_tokens
    FROM model_usage
    GROUP BY account_id, workspace_day
)
SELECT
    mu.provider,
    mu.model_name,
    COALESCE(MAX(NULLIF(report.currency_code, '')), 'USD') AS currency_code,
    COALESCE(TO_CHAR(mu.workspace_day, 'YYYY-MM-DD'), '') AS workspace_day,
    COALESCE(SUM(
        CASE
            WHEN adt.account_total_tokens > 0 THEN report.spend_amount * mu.total_tokens::double precision / adt.account_total_tokens::double precision
            ELSE 0
        END
    ), 0) AS spend_amount,
    COALESCE(SUM(mu.input_tokens), 0) AS input_tokens,
    COALESCE(SUM(mu.output_tokens), 0) AS output_tokens,
    COALESCE(SUM(mu.total_tokens), 0) AS total_tokens,
    COALESCE(SUM(mu.request_count), 0) AS request_count
FROM model_usage mu
JOIN account_day_totals adt
  ON adt.account_id = mu.account_id
 AND adt.workspace_day = mu.workspace_day
LEFT JOIN llm_daily_account_report report
  ON report.account_id = mu.account_id
 AND report.workspace_day = mu.workspace_day
GROUP BY mu.provider, mu.model_name, mu.workspace_day
ORDER BY mu.workspace_day DESC, mu.provider ASC, mu.model_name ASC`)).WithArgs(30).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListModelActivityReports(context.Background(), 30)
	if err != nil {
		t.Fatalf("ListModelActivityReports() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListModelActivityReports()) = %d, want 1", len(got))
	}
	if got[0].ModelName != "deepseek-v4-flash" || got[0].WorkspaceDay != "2026-06-20" || got[0].SpendAmount != 11.88 || got[0].RequestCount != 1380 {
		t.Fatalf("unexpected model report = %+v", got[0])
	}
}

func TestStoreListCurrentBalances(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"account_id", "account_name", "provider", "workspace_day", "captured_at", "balance_amount", "currency_code",
	}).AddRow(
		"acct_1", "deepseek:api.deepseek.com", "deepseek",
		time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 20, 15, 42, 19, 0, time.UTC),
		475.59, "CNY",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT snap.account_id, acct.account_name, acct.provider, snap.workspace_day,
snap.captured_at, snap.balance_amount, snap.currency_code
FROM llm_balance_snapshot snap
JOIN llm_account acct ON acct.id = snap.account_id
JOIN (
    SELECT account_id, MAX(captured_at) AS max_captured_at
    FROM llm_balance_snapshot
    GROUP BY account_id
) latest ON latest.account_id = snap.account_id AND latest.max_captured_at = snap.captured_at
ORDER BY snap.captured_at DESC, acct.account_name ASC
LIMIT $1`)).WithArgs(20).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListCurrentBalances(context.Background(), 20)
	if err != nil {
		t.Fatalf("ListCurrentBalances() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListCurrentBalances()) = %d, want 1", len(got))
	}
	if got[0].BalanceAmount != 475.59 || got[0].CurrencyCode != "CNY" {
		t.Fatalf("unexpected balance row = %+v", got[0])
	}
}

func TestStoreGetTodaySummary(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	workspaceDay := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
COALESCE(SUM(spend_amount), 0),
COALESCE(MAX(NULLIF(currency_code, '')), 'USD')
FROM llm_daily_account_report
WHERE workspace_day = $1`)).
		WithArgs(workspaceDay).
		WillReturnRows(sqlmock.NewRows([]string{"spend_amount", "currency_code"}).AddRow(12.34, "CNY"))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT
COALESCE(COUNT(*), 0),
COALESCE(SUM(total_tokens), 0),
COALESCE(SUM(CASE WHEN NULLIF(error_message, '') IS NOT NULL THEN 1 ELSE 0 END), 0)
FROM llm_usage_event
WHERE workspace_day = $1`)).
		WithArgs(workspaceDay).
		WillReturnRows(sqlmock.NewRows([]string{"request_count", "total_tokens", "error_count"}).AddRow(7, 4567, 2))

	store := NewStore(db)
	got, err := store.GetTodaySummary(context.Background(), workspaceDay, "America/Chicago")
	if err != nil {
		t.Fatalf("GetTodaySummary() error = %v", err)
	}
	if got.SpendAmount != 12.34 || got.CurrencyCode != "CNY" || got.RequestCount != 7 || got.TotalTokens != 4567 || got.ErrorCount != 2 {
		t.Fatalf("unexpected summary = %+v", got)
	}
	if got.WorkspaceDay != "2026-06-20" || got.TimezoneName != "America/Chicago" {
		t.Fatalf("unexpected summary metadata = %+v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestStoreGenerateDailyUsageReport(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	workspaceDay := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_daily_account_report (
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
GROUP BY account_id
ON CONFLICT (account_id, workspace_day) DO UPDATE SET
    timezone_name = EXCLUDED.timezone_name,
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    total_tokens = EXCLUDED.total_tokens,
    request_count = EXCLUDED.request_count,
    reconciliation_status = CASE
        WHEN llm_daily_account_report.reconciliation_status = 'provider_verified'
            THEN llm_daily_account_report.reconciliation_status
        ELSE EXCLUDED.reconciliation_status
    END,
    source_kind = CASE
        WHEN llm_daily_account_report.reconciliation_status = 'provider_verified'
            THEN llm_daily_account_report.source_kind
        ELSE EXCLUDED.source_kind
    END,
    updated_at = NOW()`)).
		WithArgs(workspaceDay, "America/Chicago").
		WillReturnResult(sqlmock.NewResult(0, 2))

	store := NewStore(db)
	got, err := store.GenerateDailyUsageReport(context.Background(), workspaceDay, "America/Chicago")
	if err != nil {
		t.Fatalf("GenerateDailyUsageReport() error = %v", err)
	}
	if got != 2 {
		t.Fatalf("GenerateDailyUsageReport() = %d, want 2", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
