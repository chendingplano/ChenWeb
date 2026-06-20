package llmreporthandler

import (
	"context"
	"database/sql"
	"time"
)

type DailyReport struct {
	AccountID            string    `json:"account_id"`
	AccountName          string    `json:"account_name"`
	WorkspaceDay         time.Time `json:"workspace_day"`
	TimezoneName         string    `json:"timezone_name"`
	OpeningBalance       float64   `json:"opening_balance"`
	ClosingBalance       float64   `json:"closing_balance"`
	SpendAmount          float64   `json:"spend_amount"`
	CurrencyCode         string    `json:"currency_code"`
	InputTokens          int64     `json:"input_tokens"`
	OutputTokens         int64     `json:"output_tokens"`
	TotalTokens          int64     `json:"total_tokens"`
	RequestCount         int64     `json:"request_count"`
	ReconciliationStatus string    `json:"reconciliation_status"`
}

type UsageEvent struct {
	ID               string    `json:"id"`
	AccountID        string    `json:"account_id"`
	AccountName      string    `json:"account_name"`
	ProfileID        string    `json:"profile_id"`
	RecordID         *int64    `json:"record_id"`
	Provider         string    `json:"provider"`
	ModelName        string    `json:"model_name"`
	PromptName       string    `json:"prompt_name"`
	CallReason       string    `json:"call_reason"`
	CallLoc          string    `json:"call_loc"`
	RequestStartedAt time.Time `json:"request_started_at"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	LatencyMS        int64     `json:"latency_ms"`
	ErrorMessage     string    `json:"error_message"`
}

type CurrentBalance struct {
	AccountID     string    `json:"account_id"`
	AccountName   string    `json:"account_name"`
	Provider      string    `json:"provider"`
	WorkspaceDay  time.Time `json:"workspace_day"`
	CapturedAt    time.Time `json:"captured_at"`
	BalanceAmount float64   `json:"balance_amount"`
	CurrencyCode  string    `json:"currency_code"`
}

type TodaySummary struct {
	WorkspaceDay string  `json:"workspace_day"`
	TimezoneName string  `json:"timezone_name"`
	SpendAmount  float64 `json:"spend_amount"`
	CurrencyCode string  `json:"currency_code"`
	RequestCount int64   `json:"request_count"`
	TotalTokens  int64   `json:"total_tokens"`
	ErrorCount   int64   `json:"error_count"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ListDailyReports(ctx context.Context, limit int) ([]DailyReport, error) {
	const query = `SELECT report.account_id, acct.account_name, report.workspace_day, report.timezone_name, report.opening_balance, report.closing_balance,
spend_amount, currency_code, input_tokens, output_tokens, total_tokens, request_count, reconciliation_status
FROM llm_daily_account_report report
JOIN llm_account acct ON acct.id = report.account_id
ORDER BY report.workspace_day DESC, acct.account_name ASC
LIMIT $1`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DailyReport{}
	for rows.Next() {
		var row DailyReport
		if err := rows.Scan(
			&row.AccountID,
			&row.AccountName,
			&row.WorkspaceDay,
			&row.TimezoneName,
			&row.OpeningBalance,
			&row.ClosingBalance,
			&row.SpendAmount,
			&row.CurrencyCode,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.RequestCount,
			&row.ReconciliationStatus,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListUsageEvents(ctx context.Context, limit int) ([]UsageEvent, error) {
	const query = `SELECT evt.id, evt.account_id, acct.account_name, evt.profile_id, evt.record_id, evt.provider, evt.model_name, evt.prompt_name,
evt.call_reason, evt.call_loc, request_started_at, input_tokens, output_tokens, total_tokens, latency_ms, error_message
FROM llm_usage_event evt
JOIN llm_account acct ON acct.id = evt.account_id
ORDER BY evt.request_started_at DESC
LIMIT $1`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageEvent{}
	for rows.Next() {
		var row UsageEvent
		var recordID sql.NullInt64
		if err := rows.Scan(
			&row.ID,
			&row.AccountID,
			&row.AccountName,
			&row.ProfileID,
			&recordID,
			&row.Provider,
			&row.ModelName,
			&row.PromptName,
			&row.CallReason,
			&row.CallLoc,
			&row.RequestStartedAt,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.LatencyMS,
			&row.ErrorMessage,
		); err != nil {
			return nil, err
		}
		if recordID.Valid {
			row.RecordID = &recordID.Int64
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListCurrentBalances(ctx context.Context, limit int) ([]CurrentBalance, error) {
	const query = `SELECT snap.account_id, acct.account_name, acct.provider, snap.workspace_day,
snap.captured_at, snap.balance_amount, snap.currency_code
FROM llm_balance_snapshot snap
JOIN llm_account acct ON acct.id = snap.account_id
JOIN (
    SELECT account_id, MAX(captured_at) AS max_captured_at
    FROM llm_balance_snapshot
    GROUP BY account_id
) latest ON latest.account_id = snap.account_id AND latest.max_captured_at = snap.captured_at
ORDER BY snap.captured_at DESC, acct.account_name ASC
LIMIT $1`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []CurrentBalance{}
	for rows.Next() {
		var row CurrentBalance
		if err := rows.Scan(
			&row.AccountID,
			&row.AccountName,
			&row.Provider,
			&row.WorkspaceDay,
			&row.CapturedAt,
			&row.BalanceAmount,
			&row.CurrencyCode,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) GetTodaySummary(ctx context.Context, workspaceDay time.Time, timezoneName string) (TodaySummary, error) {
	summary := TodaySummary{
		WorkspaceDay: workspaceDay.Format("2006-01-02"),
		TimezoneName: timezoneName,
		CurrencyCode: "USD",
	}

	const spendQuery = `SELECT
COALESCE(SUM(spend_amount), 0),
COALESCE(MAX(NULLIF(currency_code, '')), 'USD')
FROM llm_daily_account_report
WHERE workspace_day = $1`
	if err := s.db.QueryRowContext(ctx, spendQuery, workspaceDay).Scan(&summary.SpendAmount, &summary.CurrencyCode); err != nil {
		return TodaySummary{}, err
	}

	const usageQuery = `SELECT
COALESCE(COUNT(*), 0),
COALESCE(SUM(total_tokens), 0),
COALESCE(SUM(CASE WHEN NULLIF(error_message, '') IS NOT NULL THEN 1 ELSE 0 END), 0)
FROM llm_usage_event
WHERE workspace_day = $1`
	if err := s.db.QueryRowContext(ctx, usageQuery, workspaceDay).Scan(&summary.RequestCount, &summary.TotalTokens, &summary.ErrorCount); err != nil {
		return TodaySummary{}, err
	}

	return summary, nil
}

func (s *Store) GenerateDailyUsageReport(ctx context.Context, workspaceDay time.Time, timezoneName string) (int64, error) {
	const stmt = `INSERT INTO llm_daily_account_report (
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
    updated_at = NOW()`
	res, err := s.db.ExecContext(ctx, stmt, workspaceDay, timezoneName)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
