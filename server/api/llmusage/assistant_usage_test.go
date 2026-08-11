package llmusage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestParseAssistantUsageFileAggregates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token-usage-2026-08.jsonl")
	content := `{"localDate":"2026-08-02","model":"qwen3.7-max","inputTokens":100,"outputTokens":10,"cachedTokens":50,"thoughtsTokens":5,"totalTokens":110}
{"localDate":"2026-08-02","model":"qwen3.7-max","inputTokens":200,"outputTokens":20,"cachedTokens":100,"thoughtsTokens":10,"totalTokens":220}
{"localDate":"2026-08-03","model":"qwen3.8-max","inputTokens":300,"outputTokens":30,"cachedTokens":0,"thoughtsTokens":0,"totalTokens":330}
{"localDate":"","model":"qwen3.7-max","inputTokens":1,"outputTokens":1,"cachedTokens":0,"thoughtsTokens":0,"totalTokens":2}
not-json
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	rows, skipped, err := parseAssistantUsageFile(path)
	if err != nil {
		t.Fatalf("parseAssistantUsageFile: %v", err)
	}
	if skipped != 2 {
		t.Fatalf("skipped = %d, want 2", skipped)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2", rows)
	}

	first := rows[0]
	if first.UsageDate != "2026-08-02" || first.Model != "qwen3.7-max" ||
		first.Requests != 2 || first.InputTokens != 300 || first.OutputTokens != 30 ||
		first.CachedTokens != 150 || first.ThinkingTokens != 15 || first.TotalTokens != 330 {
		t.Fatalf("first row = %+v, unexpected aggregate", first)
	}
	second := rows[1]
	if second.UsageDate != "2026-08-03" || second.Model != "qwen3.8-max" ||
		second.Requests != 1 || second.TotalTokens != 330 {
		t.Fatalf("second row = %+v, unexpected aggregate", second)
	}
}

func TestCollectAssistantUsageUpsertsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "token-usage-2026-08.jsonl")
	content := `{"localDate":"2026-08-02","model":"qwen3.7-max","inputTokens":100,"outputTokens":10,"cachedTokens":50,"thoughtsTokens":5,"totalTokens":110}
{"localDate":"2026-08-02","model":"qwen3.7-max","inputTokens":200,"outputTokens":20,"cachedTokens":100,"thoughtsTokens":10,"totalTokens":220}
{"localDate":"2026-08-03","model":"qwen3.8-max","inputTokens":300,"outputTokens":30,"cachedTokens":0,"thoughtsTokens":0,"totalTokens":330}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mock.ExpectExec("INSERT INTO kb\\.llm_usage").
		WithArgs("qwen", "2026-08-02", "qwen3.7-max", 2, int64(300), int64(30), int64(150), int64(15), int64(330)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO kb\\.llm_usage").
		WithArgs("qwen", "2026-08-03", "qwen3.8-max", 1, int64(300), int64(30), int64(0), int64(0), int64(330)).
		WillReturnResult(sqlmock.NewResult(2, 1))

	summary, err := CollectAssistantUsage(context.Background(), db, dir, "qwen")
	if err != nil {
		t.Fatalf("CollectAssistantUsage: %v", err)
	}
	if summary.Files != 1 || summary.Rows != 2 || summary.Requests != 3 || summary.TotalTokens != 660 || summary.Skipped != 0 {
		t.Fatalf("summary = %+v, unexpected", summary)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestCollectAssistantUsageNoFiles(t *testing.T) {
	summary, err := CollectAssistantUsage(context.Background(), nil, t.TempDir(), "qwen")
	if err != nil {
		t.Fatalf("CollectAssistantUsage: %v", err)
	}
	if summary.Files != 0 || summary.Rows != 0 || summary.Requests != 0 || summary.TotalTokens != 0 {
		t.Fatalf("summary = %+v, want empty", summary)
	}
}
