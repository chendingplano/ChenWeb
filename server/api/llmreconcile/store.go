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
	CaptureSource string
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

// AlarmKindBalanceFetchFailed is persisted verbatim into alarms_errors' `kind`
// column so a fetch failure is deduplicated per account (via
// uq_alarms_errors_scope_id_kind, scope_id = account.ID) instead of writing a
// fresh row every hour the account keeps failing.
const AlarmKindBalanceFetchFailed = "deepseek_balance_fetch_failed"

// RaiseBalanceFetchAlarm records that one account's hourly balance capture
// failed, surfaced alongside every other operator alarm on
// /semos/admin/alarms. It is deliberately best-effort: the reconciliation run
// must continue to the next account regardless of whether the alarm write
// itself succeeds.
func (s *Store) RaiseBalanceFetchAlarm(ctx context.Context, accountID string, message string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO alarms_errors (severity, message, scope_id, kind) VALUES ('error',$1,$2,$3)
ON CONFLICT (scope_id, kind) WHERE scope_id IS NOT NULL AND kind IS NOT NULL DO NOTHING`, message, accountID, AlarmKindBalanceFetchFailed)
	return err
}

// ClaimHourlyBalanceCapture makes the provider balance read idempotent for an
// account/hour. Manual reconciliation therefore cannot create a burst of paid
// balance reads or misleading sub-hourly chart points.
func (s *Store) ClaimHourlyBalanceCapture(ctx context.Context, accountID string, hour time.Time) (bool, error) {
	const stmt = `INSERT INTO llm_balance_capture_slot (account_id, scheduled_hour)
VALUES ($1, $2)
ON CONFLICT (account_id, scheduled_hour) DO NOTHING`
	result, err := s.db.ExecContext(ctx, stmt, accountID, hour.UTC())
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

// InsertBalanceSnapshot records a provider-polled balance reading and derives
// total_spending in the same statement: previous total_spending for this
// account/currency, plus the drop (never negative) from the last
// provider_balance reading to this one. Deposit and set-total-spending rows
// are not provider_balance, so they are never used as the "previous" balance
// reference -- a deposit bump is not mistaken for negative spending.
func (s *Store) InsertBalanceSnapshot(ctx context.Context, snap BalanceSnapshot) error {
	const stmt = `WITH prev_balance AS (
    SELECT balance_amount
    FROM llm_balance_snapshot
    WHERE account_id = $1 AND currency_code = $5 AND entry_kind = 'provider_balance'
    ORDER BY captured_at DESC
    LIMIT 1
), prev_total AS (
    SELECT total_spending
    FROM llm_balance_snapshot
    WHERE account_id = $1 AND currency_code = $5
    ORDER BY captured_at DESC
    LIMIT 1
)
INSERT INTO llm_balance_snapshot (
    account_id, captured_at, workspace_day, balance_amount, currency_code, capture_source, raw_payload_ref, total_spending
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    COALESCE((SELECT total_spending FROM prev_total), 0)
        + GREATEST(COALESCE((SELECT balance_amount FROM prev_balance), $4) - $4, 0)
)`

	_, err := s.db.ExecContext(
		ctx,
		stmt,
		snap.AccountID,
		snap.CapturedAt,
		snap.WorkspaceDay,
		snap.BalanceAmount,
		snap.CurrencyCode,
		snap.CaptureSource,
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
