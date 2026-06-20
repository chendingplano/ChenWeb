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
