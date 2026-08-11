package llmusage

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// assistantUsageRecord is one request line of a Qwen Code
// token-usage-YYYY-MM.jsonl file (~/.qwen/usage/). The local files aggregate
// every request of the month, so past days never change and re-running the
// collector is safe.
type assistantUsageRecord struct {
	LocalDate     string `json:"localDate"`
	Model         string `json:"model"`
	InputTokens   int64  `json:"inputTokens"`
	OutputTokens  int64  `json:"outputTokens"`
	CachedTokens  int64  `json:"cachedTokens"`
	ThoughtTokens int64  `json:"thoughtsTokens"`
	TotalTokens   int64  `json:"totalTokens"`
}

// AssistantUsageRow is one daily aggregate upserted into kb.llm_usage.
type AssistantUsageRow struct {
	Assistant      string
	UsageDate      string // YYYY-MM-DD (Qwen's localDate)
	Model          string
	Requests       int
	InputTokens    int64
	OutputTokens   int64
	CachedTokens   int64
	ThinkingTokens int64
	TotalTokens    int64
}

// AssistantUsageSummary describes one CollectAssistantUsage run.
type AssistantUsageSummary struct {
	Files       int
	Rows        int
	Requests    int
	TotalTokens int64
	Skipped     int
}

// CollectAssistantUsage reads every token-usage-*.jsonl file under usageDir,
// aggregates requests per (usage_date, model), and upserts the daily rows
// into kb.llm_usage under the given assistant name. Upserts replace the
// stored aggregate with the freshly computed value, so repeated runs (e.g. a
// daily schedule plus manual backfills) never double-count.
func CollectAssistantUsage(ctx context.Context, db *sql.DB, usageDir, assistant string) (AssistantUsageSummary, error) {
	files, err := filepath.Glob(filepath.Join(usageDir, "token-usage-*.jsonl"))
	if err != nil {
		return AssistantUsageSummary{}, fmt.Errorf("list usage files: %w", err)
	}

	agg := map[string]*AssistantUsageRow{}
	summary := AssistantUsageSummary{}
	for _, path := range files {
		rows, skipped, err := parseAssistantUsageFile(path)
		if err != nil {
			return AssistantUsageSummary{}, err
		}
		summary.Files++
		summary.Skipped += skipped
		for i := range rows {
			key := rows[i].UsageDate + "\x00" + rows[i].Model
			existing, ok := agg[key]
			if !ok {
				rows[i].Assistant = assistant
				agg[key] = &rows[i]
				continue
			}
			existing.Requests += rows[i].Requests
			existing.InputTokens += rows[i].InputTokens
			existing.OutputTokens += rows[i].OutputTokens
			existing.CachedTokens += rows[i].CachedTokens
			existing.ThinkingTokens += rows[i].ThinkingTokens
			existing.TotalTokens += rows[i].TotalTokens
		}
	}

	rows := make([]AssistantUsageRow, 0, len(agg))
	for _, row := range agg {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UsageDate != rows[j].UsageDate {
			return rows[i].UsageDate < rows[j].UsageDate
		}
		return rows[i].Model < rows[j].Model
	})

	summary.Rows = len(rows)
	for _, row := range rows {
		summary.Requests += row.Requests
		summary.TotalTokens += row.TotalTokens
	}
	if db == nil || len(rows) == 0 {
		return summary, nil
	}

	const stmt = `INSERT INTO kb.llm_usage
    (assistant, usage_date, model, requests, input_tokens, output_tokens, cached_tokens, thinking_tokens, total_tokens)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (assistant, usage_date, model) DO UPDATE SET
    requests = EXCLUDED.requests,
    input_tokens = EXCLUDED.input_tokens,
    output_tokens = EXCLUDED.output_tokens,
    cached_tokens = EXCLUDED.cached_tokens,
    thinking_tokens = EXCLUDED.thinking_tokens,
    total_tokens = EXCLUDED.total_tokens,
    updated_at = NOW()`
	for _, row := range rows {
		if _, err := db.ExecContext(ctx, stmt,
			row.Assistant, row.UsageDate, row.Model,
			row.Requests, row.InputTokens, row.OutputTokens,
			row.CachedTokens, row.ThinkingTokens, row.TotalTokens); err != nil {
			return AssistantUsageSummary{}, fmt.Errorf("upsert %s %s %s: %w", row.Assistant, row.UsageDate, row.Model, err)
		}
	}
	return summary, nil
}

// parseAssistantUsageFile parses one token-usage jsonl file into per
// (date, model) aggregates, sorted by (date, model). Malformed lines and
// records missing localDate or model are skipped and counted.
func parseAssistantUsageFile(path string) ([]AssistantUsageRow, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()

	agg := map[string]*AssistantUsageRow{}
	skipped := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec assistantUsageRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			skipped++
			continue
		}
		if rec.LocalDate == "" || rec.Model == "" {
			skipped++
			continue
		}
		key := rec.LocalDate + "\x00" + rec.Model
		row, ok := agg[key]
		if !ok {
			agg[key] = &AssistantUsageRow{
				UsageDate: rec.LocalDate,
				Model:     rec.Model,
			}
			row = agg[key]
		}
		row.Requests++
		row.InputTokens += rec.InputTokens
		row.OutputTokens += rec.OutputTokens
		row.CachedTokens += rec.CachedTokens
		row.ThinkingTokens += rec.ThoughtTokens
		row.TotalTokens += rec.TotalTokens
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	rows := make([]AssistantUsageRow, 0, len(agg))
	for _, row := range agg {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UsageDate != rows[j].UsageDate {
			return rows[i].UsageDate < rows[j].UsageDate
		}
		return rows[i].Model < rows[j].Model
	})
	return rows, skipped, nil
}
