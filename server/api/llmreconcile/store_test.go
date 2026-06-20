package llmreconcile

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreListDeepSeekReconciliationAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"id", "account_name", "provider", "base_url", "api_key_ref",
	}).AddRow(
		"acct_1", "DeepSeek Primary", "deepseek", "https://api.deepseek.com", "sk-live-1",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, account_name, provider, base_url, api_key_ref
FROM llm_account
WHERE status = 'active'
  AND COALESCE(is_reconciliation_enabled, FALSE) = TRUE
  AND LOWER(provider) = 'deepseek'
  AND (
      reconciliation_kind = ''
      OR LOWER(reconciliation_kind) = 'provider_balance'
      OR LOWER(reconciliation_kind) = 'deepseek_balance'
  )
ORDER BY account_name ASC`)).WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.ListDeepSeekReconciliationAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListDeepSeekReconciliationAccounts() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(ListDeepSeekReconciliationAccounts()) = %d, want 1", len(got))
	}
	if got[0].AccountName != "DeepSeek Primary" || got[0].APIKeyRef != "sk-live-1" {
		t.Fatalf("unexpected account = %+v", got[0])
	}
}

func TestStoreInsertBalanceSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	capturedAt := time.Date(2026, 6, 20, 7, 0, 0, 0, time.UTC)
	workspaceDay := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_balance_snapshot (
    account_id, captured_at, workspace_day, balance_amount, currency_code, raw_payload_ref
) VALUES (
    $1, $2, $3, $4, $5, $6
)`)).
		WithArgs("acct_1", capturedAt, workspaceDay, 19.25, "USD", "2026/2026-06/2026-06-20/reconciliation/deepseek-account-acct_1-balance.json").
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewStore(db)
	err = store.InsertBalanceSnapshot(context.Background(), BalanceSnapshot{
		AccountID:     "acct_1",
		CapturedAt:    capturedAt,
		WorkspaceDay:  workspaceDay,
		BalanceAmount: 19.25,
		CurrencyCode:  "USD",
		RawPayloadRef: "2026/2026-06/2026-06-20/reconciliation/deepseek-account-acct_1-balance.json",
	})
	if err != nil {
		t.Fatalf("InsertBalanceSnapshot() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestStoreLatestBalanceSnapshotForDay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	workspaceDay := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	capturedAt := time.Date(2026, 6, 19, 7, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"account_id", "captured_at", "workspace_day", "balance_amount", "currency_code", "raw_payload_ref",
	}).AddRow(
		"acct_1", capturedAt, workspaceDay, 25.50, "USD", "2026/2026-06/2026-06-19/reconciliation/deepseek-account-acct_1-balance.json",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT account_id, captured_at, workspace_day, balance_amount, currency_code, raw_payload_ref
FROM llm_balance_snapshot
WHERE account_id = $1
  AND workspace_day = $2
ORDER BY captured_at DESC
LIMIT 1`)).
		WithArgs("acct_1", workspaceDay).
		WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.LatestBalanceSnapshotForDay(context.Background(), "acct_1", workspaceDay)
	if err != nil {
		t.Fatalf("LatestBalanceSnapshotForDay() error = %v", err)
	}
	if got.BalanceAmount != 25.50 || got.CurrencyCode != "USD" {
		t.Fatalf("unexpected snapshot = %+v", got)
	}
}

func TestStoreFirstBalanceSnapshotForDay(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	workspaceDay := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	capturedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"account_id", "captured_at", "workspace_day", "balance_amount", "currency_code", "raw_payload_ref",
	}).AddRow(
		"acct_1", capturedAt, workspaceDay, 30.25, "CNY", "2026/2026-06/2026-06-20/reconciliation/deepseek-account-acct_1-balance.json",
	)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT account_id, captured_at, workspace_day, balance_amount, currency_code, raw_payload_ref
FROM llm_balance_snapshot
WHERE account_id = $1
  AND workspace_day = $2
ORDER BY captured_at ASC
LIMIT 1`)).
		WithArgs("acct_1", workspaceDay).
		WillReturnRows(rows)

	store := NewStore(db)
	got, err := store.FirstBalanceSnapshotForDay(context.Background(), "acct_1", workspaceDay)
	if err != nil {
		t.Fatalf("FirstBalanceSnapshotForDay() error = %v", err)
	}
	if got.BalanceAmount != 30.25 || got.CurrencyCode != "CNY" {
		t.Fatalf("unexpected snapshot = %+v", got)
	}
}

func TestStoreUpsertProviderReconciledDailyReport(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	workspaceDay := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO llm_daily_account_report (
    account_id, workspace_day, timezone_name, opening_balance, closing_balance, spend_amount, currency_code,
    input_tokens, output_tokens, total_tokens, request_count, reconciliation_status, source_kind, source_payload_ref
) VALUES (
    $1::text, $2::date, $3::text, $4::double precision, $5::double precision, $6::double precision, $7::text,
    COALESCE((SELECT input_tokens FROM llm_daily_account_report WHERE account_id = $1::text AND workspace_day = $2::date), 0),
    COALESCE((SELECT output_tokens FROM llm_daily_account_report WHERE account_id = $1::text AND workspace_day = $2::date), 0),
    COALESCE((SELECT total_tokens FROM llm_daily_account_report WHERE account_id = $1::text AND workspace_day = $2::date), 0),
    COALESCE((SELECT request_count FROM llm_daily_account_report WHERE account_id = $1::text AND workspace_day = $2::date), 0),
    'provider_verified',
    'provider_balance',
    $8::text
)
ON CONFLICT (account_id, workspace_day) DO UPDATE SET
    timezone_name = EXCLUDED.timezone_name,
    opening_balance = EXCLUDED.opening_balance,
    closing_balance = EXCLUDED.closing_balance,
    spend_amount = EXCLUDED.spend_amount,
    currency_code = EXCLUDED.currency_code,
    reconciliation_status = EXCLUDED.reconciliation_status,
    source_kind = EXCLUDED.source_kind,
    source_payload_ref = EXCLUDED.source_payload_ref,
    updated_at = NOW()`)).
		WithArgs(
			"acct_1",
			workspaceDay,
			"America/Chicago",
			25.50,
			19.25,
			6.25,
			"USD",
			"2026/2026-06/2026-06-20/reconciliation/deepseek-account-acct_1-balance.json",
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := NewStore(db)
	err = store.UpsertProviderReconciledDailyReport(context.Background(), ReconciledDailyReport{
		AccountID:        "acct_1",
		WorkspaceDay:     workspaceDay,
		TimezoneName:     "America/Chicago",
		OpeningBalance:   25.50,
		ClosingBalance:   19.25,
		SpendAmount:      6.25,
		CurrencyCode:     "USD",
		SourcePayloadRef: "2026/2026-06/2026-06-20/reconciliation/deepseek-account-acct_1-balance.json",
	})
	if err != nil {
		t.Fatalf("UpsertProviderReconciledDailyReport() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

type sqlNoRowsStore struct{}

func (sqlNoRowsStore) ListDeepSeekReconciliationAccounts(context.Context) ([]Account, error) {
	return nil, nil
}

func (sqlNoRowsStore) InsertBalanceSnapshot(context.Context, BalanceSnapshot) error {
	return nil
}

func (sqlNoRowsStore) LatestBalanceSnapshotForDay(context.Context, string, time.Time) (BalanceSnapshot, error) {
	return BalanceSnapshot{}, sql.ErrNoRows
}

func (sqlNoRowsStore) FirstBalanceSnapshotForDay(context.Context, string, time.Time) (BalanceSnapshot, error) {
	return BalanceSnapshot{}, sql.ErrNoRows
}

func (sqlNoRowsStore) UpsertProviderReconciledDailyReport(context.Context, ReconciledDailyReport) error {
	return nil
}
