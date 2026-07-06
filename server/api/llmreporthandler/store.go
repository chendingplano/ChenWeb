package llmreporthandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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
	ID                    string    `json:"id"`
	AccountID             string    `json:"account_id"`
	AccountName           string    `json:"account_name"`
	ProfileID             string    `json:"profile_id"`
	RecordID              *int64    `json:"record_id"`
	Provider              string    `json:"provider"`
	ModelName             string    `json:"model_name"`
	PromptName            string    `json:"prompt_name"`
	CallReason            string    `json:"call_reason"`
	CallLoc               string    `json:"call_loc"`
	RequestStartedAt      time.Time `json:"request_started_at"`
	InputTokens           int64     `json:"input_tokens"`
	OutputTokens          int64     `json:"output_tokens"`
	TotalTokens           int64     `json:"total_tokens"`
	PromptCacheHitTokens  int64     `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int64     `json:"prompt_cache_miss_tokens"`
	LatencyMS             int64     `json:"latency_ms"`
	ErrorMessage          string    `json:"error_message"`
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

type ModelActivityReport struct {
	Provider     string  `json:"provider"`
	ModelName    string  `json:"model_name"`
	CurrencyCode string  `json:"currency_code"`
	WorkspaceDay string  `json:"workspace_day"`
	SpendAmount  float64 `json:"spend_amount"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	TotalTokens  int64   `json:"total_tokens"`
	RequestCount int64   `json:"request_count"`
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
evt.call_reason, evt.call_loc, request_started_at, input_tokens, output_tokens, total_tokens, prompt_cache_hit_tokens, prompt_cache_miss_tokens, latency_ms, error_message
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
			&row.PromptCacheHitTokens,
			&row.PromptCacheMissTokens,
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

func (s *Store) ListModelActivityReports(ctx context.Context, limit int) ([]ModelActivityReport, error) {
	const query = `WITH recent_days AS (
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
ORDER BY mu.workspace_day DESC, mu.provider ASC, mu.model_name ASC`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ModelActivityReport{}
	for rows.Next() {
		var row ModelActivityReport
		if err := rows.Scan(
			&row.Provider,
			&row.ModelName,
			&row.CurrencyCode,
			&row.WorkspaceDay,
			&row.SpendAmount,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.RequestCount,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

type UsageEventAdmin struct {
	ID                    string          `json:"id"`
	AccountID             *string         `json:"account_id"`
	AccountName           *string         `json:"account_name"`
	ProfileID             *string         `json:"profile_id"`
	RecordID              *int64          `json:"record_id"`
	Provider              string          `json:"provider"`
	ModelName             string          `json:"model_name"`
	PromptName            string          `json:"prompt_name"`
	CallReason            string          `json:"call_reason"`
	CallLoc               string          `json:"call_loc"`
	RequestStartedAt      time.Time       `json:"request_started_at"`
	InputTokens           int64           `json:"input_tokens"`
	OutputTokens          int64           `json:"output_tokens"`
	TotalTokens           int64           `json:"total_tokens"`
	PromptCacheHitTokens  int64           `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int64           `json:"prompt_cache_miss_tokens"`
	LatencyMS             int64           `json:"latency_ms"`
	ErrorMessage          string          `json:"error_message"`
	InputBodyRef          string          `json:"input_body_ref"`
	OutputBodyRef         string          `json:"output_body_ref"`
	MetadataJSON          json.RawMessage `json:"metadata_json"`
}

// UsageEventAdminFilters holds optional server-side filters for ListUsageEventsAdmin.
// Zero values mean "no filter" for that field.
type UsageEventAdminFilters struct {
	Model       string
	Prompt      string
	CallReason  string
	CallLoc     string
	StartedFrom *time.Time
	StartedTo   *time.Time
	InTokMin    *int64
	InTokMax    *int64
	OutTokMin   *int64
	OutTokMax   *int64
	MetaKey     string
	MetaValue   string
}

// buildUsageEventAdminWhere translates non-empty filters into a SQL WHERE clause
// (without the "WHERE" keyword) and its positional args, starting at $1.
func buildUsageEventAdminWhere(f UsageEventAdminFilters) (string, []any) {
	var clauses []string
	var args []any

	add := func(clause string, arg any) {
		args = append(args, arg)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}

	if f.Model != "" {
		add("evt.model_name ILIKE '%%' || $%d || '%%'", f.Model)
	}
	if f.Prompt != "" {
		add("evt.prompt_name ILIKE '%%' || $%d || '%%'", f.Prompt)
	}
	if f.CallReason != "" {
		add("evt.call_reason ILIKE '%%' || $%d || '%%'", f.CallReason)
	}
	if f.CallLoc != "" {
		add("evt.call_loc ILIKE '%%' || $%d || '%%'", f.CallLoc)
	}
	if f.StartedFrom != nil {
		add("evt.request_started_at >= $%d", *f.StartedFrom)
	}
	if f.StartedTo != nil {
		add("evt.request_started_at <= $%d", *f.StartedTo)
	}
	if f.InTokMin != nil {
		add("evt.input_tokens >= $%d", *f.InTokMin)
	}
	if f.InTokMax != nil {
		add("evt.input_tokens <= $%d", *f.InTokMax)
	}
	if f.OutTokMin != nil {
		add("evt.output_tokens >= $%d", *f.OutTokMin)
	}
	if f.OutTokMax != nil {
		add("evt.output_tokens <= $%d", *f.OutTokMax)
	}
	if f.MetaKey != "" && f.MetaValue != "" {
		args = append(args, f.MetaKey)
		keyArg := len(args)
		args = append(args, f.MetaValue)
		valueArg := len(args)
		clauses = append(clauses, fmt.Sprintf("evt.metadata_json ->> $%d ILIKE '%%' || $%d || '%%'", keyArg, valueArg))
	}

	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (s *Store) ListUsageEventsAdmin(ctx context.Context, page, pageSize int, filters UsageEventAdminFilters) ([]UsageEventAdmin, int64, error) {
	where, whereArgs := buildUsageEventAdminWhere(filters)

	var total int64
	countQuery := "SELECT COUNT(*) FROM llm_usage_event evt" + where
	if err := s.db.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	limitArg := len(whereArgs) + 1
	offsetArg := len(whereArgs) + 2
	query := fmt.Sprintf(`SELECT evt.id, evt.account_id, acct.account_name, evt.profile_id, evt.record_id,
evt.provider, evt.model_name, evt.prompt_name, evt.call_reason, evt.call_loc,
evt.request_started_at, evt.input_tokens, evt.output_tokens, evt.total_tokens,
evt.prompt_cache_hit_tokens, evt.prompt_cache_miss_tokens, evt.latency_ms, evt.error_message,
COALESCE(evt.input_body_ref, ''), COALESCE(evt.output_body_ref, ''), COALESCE(evt.metadata_json, '{}'::jsonb)
FROM llm_usage_event evt
LEFT JOIN llm_account acct ON acct.id = evt.account_id%s
ORDER BY evt.request_started_at DESC
LIMIT $%d OFFSET $%d`, where, limitArg, offsetArg)

	args := append(append([]any{}, whereArgs...), pageSize, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []UsageEventAdmin{}
	for rows.Next() {
		var row UsageEventAdmin
		var accountID, accountName, profileID sql.NullString
		var recordID sql.NullInt64
		var metadataJSON []byte
		if err := rows.Scan(
			&row.ID, &accountID, &accountName, &profileID, &recordID,
			&row.Provider, &row.ModelName, &row.PromptName, &row.CallReason, &row.CallLoc,
			&row.RequestStartedAt, &row.InputTokens, &row.OutputTokens, &row.TotalTokens,
			&row.PromptCacheHitTokens, &row.PromptCacheMissTokens, &row.LatencyMS, &row.ErrorMessage,
			&row.InputBodyRef, &row.OutputBodyRef, &metadataJSON,
		); err != nil {
			return nil, 0, err
		}
		if accountID.Valid {
			row.AccountID = &accountID.String
		}
		if accountName.Valid {
			row.AccountName = &accountName.String
		}
		if profileID.Valid {
			row.ProfileID = &profileID.String
		}
		if recordID.Valid {
			row.RecordID = &recordID.Int64
		}
		row.MetadataJSON = json.RawMessage(metadataJSON)
		out = append(out, row)
	}
	return out, total, rows.Err()
}

func (s *Store) GetUsageEventBodyRefs(ctx context.Context, id string) (inputRef, outputRef string, err error) {
	const query = `SELECT COALESCE(input_body_ref, ''), COALESCE(output_body_ref, '') FROM llm_usage_event WHERE id = $1`
	err = s.db.QueryRowContext(ctx, query, id).Scan(&inputRef, &outputRef)
	return
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
  AND account_id IS NOT NULL
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
    updated_at = NOW()`
	res, err := s.db.ExecContext(ctx, stmt, workspaceDay, timezoneName)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
