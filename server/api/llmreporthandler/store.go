package llmreporthandler

import (
	"context"
	"database/sql"
	"time"
)

type DailyReport struct {
	AccountID            string    `json:"account_id"`
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
	ProfileID        string    `json:"profile_id"`
	Provider         string    `json:"provider"`
	ModelName        string    `json:"model_name"`
	PromptName       string    `json:"prompt_name"`
	RequestStartedAt time.Time `json:"request_started_at"`
	InputTokens      int64     `json:"input_tokens"`
	OutputTokens     int64     `json:"output_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	LatencyMS        int64     `json:"latency_ms"`
	ErrorMessage     string    `json:"error_message"`
}

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) ListDailyReports(ctx context.Context, limit int) ([]DailyReport, error) {
	const query = `SELECT account_id, workspace_day, timezone_name, opening_balance, closing_balance,
spend_amount, currency_code, input_tokens, output_tokens, total_tokens, request_count, reconciliation_status
FROM llm_daily_account_report
ORDER BY workspace_day DESC, account_id ASC
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
	const query = `SELECT id, account_id, profile_id, provider, model_name, prompt_name,
request_started_at, input_tokens, output_tokens, total_tokens, latency_ms, error_message
FROM llm_usage_event
ORDER BY request_started_at DESC
LIMIT $1`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UsageEvent{}
	for rows.Next() {
		var row UsageEvent
		if err := rows.Scan(
			&row.ID,
			&row.AccountID,
			&row.ProfileID,
			&row.Provider,
			&row.ModelName,
			&row.PromptName,
			&row.RequestStartedAt,
			&row.InputTokens,
			&row.OutputTokens,
			&row.TotalTokens,
			&row.LatencyMS,
			&row.ErrorMessage,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
