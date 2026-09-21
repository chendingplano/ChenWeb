package llmreporthandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
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

// BalanceHistory is an immutable provider-reported account balance.  It is
// deliberately account/API-key scoped: DeepSeek does not report a balance per
// model, so this must never be allocated to Flash or Pro.
type BalanceHistory struct {
	AccountID     string    `json:"account_id"`
	AccountName   string    `json:"account_name"`
	Provider      string    `json:"provider"`
	CapturedAt    time.Time `json:"captured_at"`
	BalanceAmount float64   `json:"balance_amount"`
	CurrencyCode  string    `json:"currency_code"`
}

// HourlyBalanceReport combines the two official balance currencies with the
// CNY spending derived from the immediately preceding hourly CNY balance.
// It never allocates an account balance to a model.
type HourlyBalanceReport struct {
	AccountID        string    `json:"account_id"`
	AccountName      string    `json:"account_name"`
	Provider         string    `json:"provider"`
	HourStartedAt    time.Time `json:"hour_started_at"`
	BalanceUSD       *float64  `json:"balance_usd"`
	BalanceCNY       *float64  `json:"balance_cny"`
	SpendingCNY      *float64  `json:"spending_cny"`
	TotalSpendingCNY *float64  `json:"total_spending_cny"`
	TotalSpendingUSD *float64  `json:"total_spending_usd"`
}

type ModelActivityReport struct {
	Provider              string  `json:"provider"`
	ModelName             string  `json:"model_name"`
	APIKeyName            string  `json:"api_key_name"`
	CurrencyCode          string  `json:"currency_code"`
	WorkspaceDay          string  `json:"workspace_day"`
	SpendAmount           float64 `json:"spend_amount"`
	PromptCacheHitTokens  int64   `json:"prompt_cache_hit_tokens"`
	PromptCacheMissTokens int64   `json:"prompt_cache_miss_tokens"`
	OutputTokens          int64   `json:"output_tokens"`
	TotalTokens           int64   `json:"total_tokens"`
	// Beijing-time peak/off-peak split of the totals above, used by
	// applyDeepSeekLocalCNYPricing to price each bucket at its own rate.
	PromptCacheHitTokensPeak     int64 `json:"prompt_cache_hit_tokens_peak"`
	PromptCacheHitTokensOffPeak  int64 `json:"prompt_cache_hit_tokens_offpeak"`
	PromptCacheMissTokensPeak    int64 `json:"prompt_cache_miss_tokens_peak"`
	PromptCacheMissTokensOffPeak int64 `json:"prompt_cache_miss_tokens_offpeak"`
	OutputTokensPeak             int64 `json:"output_tokens_peak"`
	OutputTokensOffPeak          int64 `json:"output_tokens_offpeak"`
	RequestCount                 int64 `json:"request_count"`
}

type ModelActivityReportFilters struct {
	From       *time.Time
	To         *time.Time
	APIKeyRef  string
	ModelNames []string
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

func (s *Store) ListBalanceHistory(ctx context.Context, limit int) ([]BalanceHistory, error) {
	const query = `SELECT snap.account_id, acct.account_name, acct.provider,
snap.captured_at, snap.balance_amount, snap.currency_code
FROM llm_balance_snapshot snap
JOIN llm_account acct ON acct.id = snap.account_id
ORDER BY snap.captured_at DESC, acct.account_name ASC
LIMIT $1`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BalanceHistory{}
	for rows.Next() {
		var row BalanceHistory
		if err := rows.Scan(&row.AccountID, &row.AccountName, &row.Provider, &row.CapturedAt, &row.BalanceAmount, &row.CurrencyCode); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListHourlyBalanceReports(ctx context.Context, limit int, frequency string, filters ModelActivityReportFilters) ([]HourlyBalanceReport, error) {
	bucket := "hour"
	if frequency == "daily" {
		bucket = "day"
	} else if frequency == "monthly" {
		bucket = "month"
	}
	// spending_cny is derived from the persisted total_spending column rather
	// than recomputed from raw balances: it is simply the change in
	// total_spending between this bucket and the previous one. That is the
	// same query for every granularity -- only the date_trunc unit (bucket)
	// differs between hourly, daily, and monthly.
	query := fmt.Sprintf(`WITH latest_balance AS (
    SELECT DISTINCT ON (account_id, currency_code, date_trunc('%s', captured_at))
        account_id, currency_code, date_trunc('%s', captured_at) AS bucket_start, balance_amount
    FROM llm_balance_snapshot
    WHERE workspace_day >= $2::date AND workspace_day <= $3::date
      AND ($4 = '' OR account_id IN (SELECT id FROM llm_account WHERE api_key_ref = $4))
      AND entry_kind = 'provider_balance'
    ORDER BY account_id, currency_code, date_trunc('%s', captured_at), captured_at DESC
), latest_total AS (
    SELECT DISTINCT ON (account_id, currency_code, date_trunc('%s', captured_at))
        account_id, currency_code, date_trunc('%s', captured_at) AS bucket_start, total_spending
    FROM llm_balance_snapshot
    WHERE workspace_day >= $2::date AND workspace_day <= $3::date
      AND ($4 = '' OR account_id IN (SELECT id FROM llm_account WHERE api_key_ref = $4))
    ORDER BY account_id, currency_code, date_trunc('%s', captured_at), captured_at DESC
), balance_pivot AS (
    SELECT account_id, bucket_start,
        MAX(balance_amount) FILTER (WHERE UPPER(currency_code) = 'USD') AS balance_usd,
        MAX(balance_amount) FILTER (WHERE UPPER(currency_code) = 'CNY') AS balance_cny
    FROM latest_balance
    GROUP BY account_id, bucket_start
), total_pivot AS (
    SELECT account_id, bucket_start,
        MAX(total_spending) FILTER (WHERE UPPER(currency_code) = 'CNY') AS total_spending_cny,
        MAX(total_spending) FILTER (WHERE UPPER(currency_code) = 'USD') AS total_spending_usd
    FROM latest_total
    GROUP BY account_id, bucket_start
), combined AS (
    SELECT COALESCE(b.account_id, t.account_id) AS account_id,
           COALESCE(b.bucket_start, t.bucket_start) AS hour_started_at,
           b.balance_usd, b.balance_cny, t.total_spending_cny, t.total_spending_usd
    FROM balance_pivot b
    FULL OUTER JOIN total_pivot t ON t.account_id = b.account_id AND t.bucket_start = b.bucket_start
), with_previous AS (
    SELECT *, LAG(total_spending_cny) OVER (PARTITION BY account_id ORDER BY hour_started_at) AS previous_total_spending_cny
    FROM combined
)
SELECT report.account_id, acct.account_name, acct.provider, report.hour_started_at,
       report.balance_usd, report.balance_cny,
       CASE WHEN report.previous_total_spending_cny IS NULL OR report.total_spending_cny IS NULL THEN NULL
            ELSE GREATEST(report.total_spending_cny - report.previous_total_spending_cny, 0) END AS spending_cny,
       report.total_spending_cny,
       report.total_spending_usd
FROM with_previous report
JOIN llm_account acct ON acct.id = report.account_id
ORDER BY report.hour_started_at DESC, acct.account_name ASC
LIMIT $1`, bucket, bucket, bucket, bucket, bucket, bucket)
	rows, err := s.db.QueryContext(ctx, query, limit, filters.From, filters.To, filters.APIKeyRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HourlyBalanceReport{}
	for rows.Next() {
		var row HourlyBalanceReport
		var usd, cny, spend, totalCNY, totalUSD sql.NullFloat64
		if err := rows.Scan(&row.AccountID, &row.AccountName, &row.Provider, &row.HourStartedAt, &usd, &cny, &spend, &totalCNY, &totalUSD); err != nil {
			return nil, err
		}
		if usd.Valid {
			row.BalanceUSD = &usd.Float64
		}
		if cny.Valid {
			row.BalanceCNY = &cny.Float64
		}
		if spend.Valid {
			row.SpendingCNY = &spend.Float64
		}
		if totalCNY.Valid {
			row.TotalSpendingCNY = &totalCNY.Float64
		}
		if totalUSD.Valid {
			row.TotalSpendingUSD = &totalUSD.Float64
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) ListModelActivityReports(ctx context.Context, limit int, filters ModelActivityReportFilters) ([]ModelActivityReport, error) {
	const query = `WITH recent_days AS (
    SELECT DISTINCT workspace_day
    FROM llm_daily_account_report
    WHERE ($2::date IS NULL OR workspace_day >= $2::date)
      AND ($3::date IS NULL OR workspace_day <= $3::date)
    ORDER BY workspace_day DESC
    LIMIT CASE WHEN $2::date IS NULL AND $3::date IS NULL THEN $1 ELSE 10000 END
),
model_usage AS (
    SELECT
        evt.workspace_day,
        evt.account_id,
        evt.provider,
        evt.model_name,
        acct.api_key_ref,
        COALESCE(SUM(evt.prompt_cache_hit_tokens), 0) AS prompt_cache_hit_tokens,
        COALESCE(SUM(evt.prompt_cache_miss_tokens), 0) AS prompt_cache_miss_tokens,
        COALESCE(SUM(evt.output_tokens), 0) AS output_tokens,
        COALESCE(SUM(evt.total_tokens), 0) AS total_tokens,
        -- Beijing-time peak hours are 09:00-12:00 and 14:00-18:00 (DeepSeek CNY billing).
        COALESCE(SUM(evt.prompt_cache_hit_tokens) FILTER (
            WHERE EXTRACT(HOUR FROM evt.request_started_at AT TIME ZONE 'Asia/Shanghai') IN (9, 10, 11, 14, 15, 16, 17)
        ), 0) AS prompt_cache_hit_tokens_peak,
        COALESCE(SUM(evt.prompt_cache_hit_tokens) FILTER (
            WHERE EXTRACT(HOUR FROM evt.request_started_at AT TIME ZONE 'Asia/Shanghai') NOT IN (9, 10, 11, 14, 15, 16, 17)
        ), 0) AS prompt_cache_hit_tokens_offpeak,
        COALESCE(SUM(evt.prompt_cache_miss_tokens) FILTER (
            WHERE EXTRACT(HOUR FROM evt.request_started_at AT TIME ZONE 'Asia/Shanghai') IN (9, 10, 11, 14, 15, 16, 17)
        ), 0) AS prompt_cache_miss_tokens_peak,
        COALESCE(SUM(evt.prompt_cache_miss_tokens) FILTER (
            WHERE EXTRACT(HOUR FROM evt.request_started_at AT TIME ZONE 'Asia/Shanghai') NOT IN (9, 10, 11, 14, 15, 16, 17)
        ), 0) AS prompt_cache_miss_tokens_offpeak,
        COALESCE(SUM(evt.output_tokens) FILTER (
            WHERE EXTRACT(HOUR FROM evt.request_started_at AT TIME ZONE 'Asia/Shanghai') IN (9, 10, 11, 14, 15, 16, 17)
        ), 0) AS output_tokens_peak,
        COALESCE(SUM(evt.output_tokens) FILTER (
            WHERE EXTRACT(HOUR FROM evt.request_started_at AT TIME ZONE 'Asia/Shanghai') NOT IN (9, 10, 11, 14, 15, 16, 17)
        ), 0) AS output_tokens_offpeak,
        COUNT(*) AS request_count
    FROM llm_usage_event evt
    JOIN llm_account acct ON acct.id = evt.account_id
    JOIN recent_days days ON days.workspace_day = evt.workspace_day
    WHERE ($4 = '' OR acct.api_key_ref = $4)
      AND ($5::text[] IS NULL OR evt.model_name = ANY($5::text[]))
    GROUP BY evt.workspace_day, evt.account_id, evt.provider, evt.model_name, acct.api_key_ref
)
SELECT
    mu.provider,
    mu.model_name,
    mu.api_key_ref,
    COALESCE(MAX(NULLIF(report.currency_code, '')), 'USD') AS currency_code,
    COALESCE(TO_CHAR(mu.workspace_day, 'YYYY-MM-DD'), '') AS workspace_day,
    0::double precision AS spend_amount,
    COALESCE(SUM(mu.prompt_cache_hit_tokens), 0) AS prompt_cache_hit_tokens,
    COALESCE(SUM(mu.prompt_cache_miss_tokens), 0) AS prompt_cache_miss_tokens,
    COALESCE(SUM(mu.output_tokens), 0) AS output_tokens,
    COALESCE(SUM(mu.total_tokens), 0) AS total_tokens,
    COALESCE(SUM(mu.prompt_cache_hit_tokens_peak), 0) AS prompt_cache_hit_tokens_peak,
    COALESCE(SUM(mu.prompt_cache_hit_tokens_offpeak), 0) AS prompt_cache_hit_tokens_offpeak,
    COALESCE(SUM(mu.prompt_cache_miss_tokens_peak), 0) AS prompt_cache_miss_tokens_peak,
    COALESCE(SUM(mu.prompt_cache_miss_tokens_offpeak), 0) AS prompt_cache_miss_tokens_offpeak,
    COALESCE(SUM(mu.output_tokens_peak), 0) AS output_tokens_peak,
    COALESCE(SUM(mu.output_tokens_offpeak), 0) AS output_tokens_offpeak,
    COALESCE(SUM(mu.request_count), 0) AS request_count
FROM model_usage mu
LEFT JOIN llm_daily_account_report report
  ON report.account_id = mu.account_id
 AND report.workspace_day = mu.workspace_day
GROUP BY mu.provider, mu.model_name, mu.api_key_ref, mu.workspace_day
ORDER BY mu.workspace_day DESC, mu.provider ASC, mu.model_name ASC, mu.api_key_ref ASC`

	var from, to any
	if filters.From != nil {
		from = filters.From
	}
	if filters.To != nil {
		to = filters.To
	}
	var modelNames any
	if len(filters.ModelNames) > 0 {
		modelNames = pq.Array(filters.ModelNames)
	}
	rows, err := s.db.QueryContext(ctx, query, limit, from, to, filters.APIKeyRef, modelNames)
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
			&row.APIKeyName,
			&row.CurrencyCode,
			&row.WorkspaceDay,
			&row.SpendAmount,
			&row.PromptCacheHitTokens,
			&row.PromptCacheMissTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.PromptCacheHitTokensPeak,
			&row.PromptCacheHitTokensOffPeak,
			&row.PromptCacheMissTokensPeak,
			&row.PromptCacheMissTokensOffPeak,
			&row.OutputTokensPeak,
			&row.OutputTokensOffPeak,
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
	RunID                 *int64          `json:"run_id"`
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
	RunID       *int64
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
	if f.RunID != nil {
		add("evt.run_id = $%d", *f.RunID)
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
	query := fmt.Sprintf(`SELECT evt.id, evt.account_id, acct.account_name, evt.profile_id, evt.record_id, evt.run_id,
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
		var recordID, runID sql.NullInt64
		var metadataJSON []byte
		if err := rows.Scan(
			&row.ID, &accountID, &accountName, &profileID, &recordID, &runID,
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
		if runID.Valid {
			row.RunID = &runID.Int64
		}
		row.MetadataJSON = json.RawMessage(metadataJSON)
		out = append(out, row)
	}
	return out, total, rows.Err()
}

// ListUsageEventsByIDs loads llm_usage_event rows for a set of ids, e.g. the
// ids recorded in kb.doc_review_logs.detail.llm_usage_event_ids. Unknown ids
// are silently omitted from the result rather than erroring.
func (s *Store) ListUsageEventsByIDs(ctx context.Context, ids []string) ([]UsageEventAdmin, error) {
	if len(ids) == 0 {
		return []UsageEventAdmin{}, nil
	}
	const query = `SELECT evt.id, evt.account_id, acct.account_name, evt.profile_id, evt.record_id,
evt.provider, evt.model_name, evt.prompt_name, evt.call_reason, evt.call_loc,
evt.request_started_at, evt.input_tokens, evt.output_tokens, evt.total_tokens,
evt.prompt_cache_hit_tokens, evt.prompt_cache_miss_tokens, evt.latency_ms, evt.error_message,
COALESCE(evt.input_body_ref, ''), COALESCE(evt.output_body_ref, ''), COALESCE(evt.metadata_json, '{}'::jsonb)
FROM llm_usage_event evt
LEFT JOIN llm_account acct ON acct.id = evt.account_id
WHERE evt.id = ANY($1)
ORDER BY evt.request_started_at DESC`

	rows, err := s.db.QueryContext(ctx, query, pq.Array(ids))
	if err != nil {
		return nil, err
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
			return nil, err
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
	return out, rows.Err()
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
