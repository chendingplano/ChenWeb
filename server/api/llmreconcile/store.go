package llmreconcile

import (
	"context"
	"database/sql"
	"time"
)

type Account struct {
	ID          string
	AccountName string
	Provider    string
	BaseURL     string
	APIKeyRef   string
}

type BalanceSnapshot struct {
	AccountID     string
	CapturedAt    time.Time
	WorkspaceDay  time.Time
	BalanceAmount float64
	CurrencyCode  string
	RawPayloadRef string
}

type ReconciledDailyReport struct {
	AccountID        string
	WorkspaceDay     time.Time
	TimezoneName     string
	OpeningBalance   float64
	ClosingBalance   float64
	SpendAmount      float64
	CurrencyCode     string
	SourcePayloadRef string
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ListDeepSeekReconciliationAccounts(ctx context.Context) ([]Account, error) {
	const query = `SELECT id, account_name, provider, base_url, api_key_ref
FROM llm_account
WHERE status = 'active'
  AND COALESCE(is_reconciliation_enabled, FALSE) = TRUE
  AND LOWER(provider) = 'deepseek'
  AND (
      reconciliation_kind = ''
      OR LOWER(reconciliation_kind) = 'provider_balance'
      OR LOWER(reconciliation_kind) = 'deepseek_balance'
  )
ORDER BY account_name ASC`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Account{}
	for rows.Next() {
		var row Account
		if err := rows.Scan(&row.ID, &row.AccountName, &row.Provider, &row.BaseURL, &row.APIKeyRef); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) InsertBalanceSnapshot(ctx context.Context, snap BalanceSnapshot) error {
	const stmt = `INSERT INTO llm_balance_snapshot (
    account_id, captured_at, workspace_day, balance_amount, currency_code, raw_payload_ref
) VALUES (
    $1, $2, $3, $4, $5, $6
)`

	_, err := s.db.ExecContext(
		ctx,
		stmt,
		snap.AccountID,
		snap.CapturedAt,
		snap.WorkspaceDay,
		snap.BalanceAmount,
		snap.CurrencyCode,
		snap.RawPayloadRef,
	)
	return err
}

func (s *Store) LatestBalanceSnapshotForDay(ctx context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error) {
	const query = `SELECT account_id, captured_at, workspace_day, balance_amount, currency_code, raw_payload_ref
FROM llm_balance_snapshot
WHERE account_id = $1
  AND workspace_day = $2
ORDER BY captured_at DESC
LIMIT 1`

	var row BalanceSnapshot
	err := s.db.QueryRowContext(ctx, query, accountID, workspaceDay).Scan(
		&row.AccountID,
		&row.CapturedAt,
		&row.WorkspaceDay,
		&row.BalanceAmount,
		&row.CurrencyCode,
		&row.RawPayloadRef,
	)
	if err != nil {
		return BalanceSnapshot{}, err
	}
	return row, nil
}

func (s *Store) FirstBalanceSnapshotForDay(ctx context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error) {
	const query = `SELECT account_id, captured_at, workspace_day, balance_amount, currency_code, raw_payload_ref
FROM llm_balance_snapshot
WHERE account_id = $1
  AND workspace_day = $2
ORDER BY captured_at ASC
LIMIT 1`

	var row BalanceSnapshot
	err := s.db.QueryRowContext(ctx, query, accountID, workspaceDay).Scan(
		&row.AccountID,
		&row.CapturedAt,
		&row.WorkspaceDay,
		&row.BalanceAmount,
		&row.CurrencyCode,
		&row.RawPayloadRef,
	)
	if err != nil {
		return BalanceSnapshot{}, err
	}
	return row, nil
}

func (s *Store) UpsertProviderReconciledDailyReport(ctx context.Context, report ReconciledDailyReport) error {
	const stmt = `INSERT INTO llm_daily_account_report (
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
    updated_at = NOW()`

	_, err := s.db.ExecContext(
		ctx,
		stmt,
		report.AccountID,
		report.WorkspaceDay,
		report.TimezoneName,
		report.OpeningBalance,
		report.ClosingBalance,
		report.SpendAmount,
		report.CurrencyCode,
		report.SourcePayloadRef,
	)
	return err
}
