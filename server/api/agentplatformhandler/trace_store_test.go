package agentplatformhandler

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/agenttrace"
)

func TestUpsertTaskRunTraceStoresNormalizedTrace(t *testing.T) {
	db, mock, cleanup := newTraceMockDB(t)
	defer cleanup()

	trace := agenttrace.Trace{
		AgentKind:       "codex",
		ProviderTraceID: "thread-1",
		RunID:           "run-1",
		Input:           "do work",
		Output:          "done",
		ToolCalls:       []agenttrace.ToolCall{{Name: "command_execution"}},
		Usage:           agenttrace.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		TotalLatencyMS:  123,
	}
	raw, _ := json.Marshal(trace)

	mock.ExpectExec("INSERT INTO ap_agent_trace").
		WithArgs(
			"workspace-1", "run-1", "codex", "thread-1",
			"do work", "done", int64(1),
			int64(10), int64(0), int64(5), int64(0), int64(15),
			int64(123), sqlmock.AnyArg(), string(raw),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := upsertTaskRunTrace(db, "workspace-1", "run-1", trace); err != nil {
		t.Fatalf("upsertTaskRunTrace returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestGetTaskRunTraceReadsNormalizedTrace(t *testing.T) {
	db, mock, cleanup := newTraceMockDB(t)
	defer cleanup()

	trace := agenttrace.Trace{
		AgentKind: "claude_code",
		RunID:     "run-2",
		Output:    "done",
	}
	raw, _ := json.Marshal(trace)
	created := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	modified := created.Add(time.Minute)

	mock.ExpectQuery("SELECT id, workspace_id, task_run_id").
		WithArgs("workspace-1", "run-2").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "task_run_id", "agent_kind", "provider_trace_id",
			"input_text", "output_text", "tool_call_count",
			"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens",
			"total_latency_ms", "total_cost_usd", "trace_json", "created_at", "updated_at",
		}).AddRow(
			"trace-1", "workspace-1", "run-2", "claude_code", "",
			"", "done", int64(0),
			int64(0), int64(0), int64(0), int64(0), int64(0),
			sql.NullInt64{}, sql.NullFloat64{}, string(raw), created, modified,
		))

	rec, err := getTaskRunTrace(db, "workspace-1", "run-2")
	if err != nil {
		t.Fatalf("getTaskRunTrace returned error: %v", err)
	}
	if rec.Trace.AgentKind != "claude_code" || rec.Trace.Output != "done" {
		t.Fatalf("unexpected trace record: %#v", rec)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestListTaskRunTracesReturnsRecentSummaries(t *testing.T) {
	db, mock, cleanup := newTraceMockDB(t)
	defer cleanup()

	created := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT id, workspace_id, task_run_id").
		WithArgs("workspace-1", "codex", 2).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "task_run_id", "agent_kind", "provider_trace_id",
			"output_text", "tool_call_count",
			"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens",
			"total_latency_ms", "total_cost_usd", "created_at", "updated_at",
		}).AddRow(
			"trace-1", "workspace-1", "run-1", "codex", "thread-1",
			"done", int64(1),
			int64(10), int64(3), int64(5), int64(0), int64(15),
			sql.NullInt64{Int64: 123, Valid: true}, sql.NullFloat64{}, created, created,
		))

	items, err := listTaskRunTraces(db, "workspace-1", TraceListFilter{AgentKind: "codex", Limit: 2})
	if err != nil {
		t.Fatalf("listTaskRunTraces returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].TaskRunID != "run-1" || items[0].Usage.TotalTokens != 15 || items[0].TotalLatencyMS == nil {
		t.Fatalf("unexpected item: %#v", items[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestBuildTraceEvaluationReportUsesRequestScorers(t *testing.T) {
	trace := agenttrace.Trace{
		Output:    "Created summary.",
		ToolCalls: []agenttrace.ToolCall{{Name: "command_execution"}},
		Usage:     agenttrace.TokenUsage{TotalTokens: 42},
	}
	report := buildTraceEvaluationReport(trace, TraceEvaluationRequest{
		ContainsAnswer: []string{"summary"},
		UsedTools:      []string{"command_execution"},
		AvoidedTools:   []string{"send_email"},
		MaxTokens:      intPtr(100),
	})
	if !report.Passed || report.OverallScore != 1 {
		t.Fatalf("expected passing report: %#v", report)
	}
}

func newTraceMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	return db, mock, func() { _ = db.Close() }
}
