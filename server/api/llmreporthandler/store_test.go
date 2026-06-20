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
		"account_id", "workspace_day", "timezone_name", "opening_balance", "closing_balance",
		"spend_amount", "currency_code", "input_tokens", "output_tokens", "total_tokens",
		"request_count", "reconciliation_status",
	}).AddRow(
		"acct_1", time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC), "America/Chicago",
		25.5, 19.25, 6.25, "USD", 1000, 250, 1250, 8, "provider_verified",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT account_id, workspace_day, timezone_name, opening_balance, closing_balance,
spend_amount, currency_code, input_tokens, output_tokens, total_tokens, request_count, reconciliation_status
FROM llm_daily_account_report
ORDER BY workspace_day DESC, account_id ASC
LIMIT $1`)).WithArgs(30).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListDailyReports(context.Background(), 30)
	if err != nil {
		t.Fatalf("ListDailyReports() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListDailyReports()) = %d, want 1", len(got))
	}
	if got[0].SpendAmount != 6.25 || got[0].RequestCount != 8 {
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
		"id", "account_id", "profile_id", "provider", "model_name", "prompt_name",
		"request_started_at", "input_tokens", "output_tokens", "total_tokens", "latency_ms", "error_message",
	}).AddRow(
		"evt_1", "acct_1", "prof_1", "deepseek", "deepseek-v4-flash", "extract-products-v2",
		time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC), 100, 25, 125, 3200, "",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, account_id, profile_id, provider, model_name, prompt_name,
request_started_at, input_tokens, output_tokens, total_tokens, latency_ms, error_message
FROM llm_usage_event
ORDER BY request_started_at DESC
LIMIT $1`)).WithArgs(50).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListUsageEvents(context.Background(), 50)
	if err != nil {
		t.Fatalf("ListUsageEvents() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListUsageEvents()) = %d, want 1", len(got))
	}
	if got[0].ModelName != "deepseek-v4-flash" || got[0].TotalTokens != 125 {
		t.Fatalf("unexpected usage event = %+v", got[0])
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
    reconciliation_status = EXCLUDED.reconciliation_status,
    source_kind = EXCLUDED.source_kind,
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
