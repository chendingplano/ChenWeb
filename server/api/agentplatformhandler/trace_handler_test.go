package agentplatformhandler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/agenttrace"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

func TestGetTaskRunTraceHandlerReturnsNormalizedTrace(t *testing.T) {
	_, mock, cleanup := newTraceHandlerMockDB(t)
	defer cleanup()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ap/w/trace-smoke/runs/run-1/trace", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug", "id")
	c.SetParamValues("trace-smoke", "run-1")

	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	trace := agenttrace.Trace{
		AgentKind:      "codex",
		RunID:          "run-1",
		Output:         "done",
		TotalLatencyMS: 603,
		Events: []agenttrace.TraceEvent{{
			Kind:    "stdout",
			Message: "dryrun: done",
			Fields:  map[string]any{"line_index": float64(3)},
		}},
		Metadata: map[string]any{"trace_format": "plain_text_fallback", "event_count": float64(4)},
	}
	raw, _ := json.Marshal(trace)

	expectResolveWorkspace(mock, "trace-smoke", "user-1", "workspace-1", "owner")
	expectGetTaskRun(mock, "workspace-1", "run-1", now)
	expectGetTrace(mock, "workspace-1", "run-1", "trace-1", trace, string(raw), now)

	if err := GetTaskRunTrace(c); err != nil {
		t.Fatalf("GetTaskRunTrace returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got AgentTraceRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.TaskRunID != "run-1" || got.Trace.Metadata["trace_format"] != "plain_text_fallback" || len(got.Trace.Events) != 1 {
		t.Fatalf("unexpected response: %#v", got)
	}
	assertTraceHandlerExpectations(t, mock)
}

func TestListTaskRunTracesHandlerFiltersByAgentKind(t *testing.T) {
	db, mock, cleanup := newTraceHandlerMockDB(t)
	defer cleanup()
	_ = db

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ap/w/trace-smoke/traces?agent_kind=codex&limit=2", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug")
	c.SetParamValues("trace-smoke")

	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	expectResolveWorkspace(mock, "trace-smoke", "user-1", "workspace-1", "viewer")
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
			int64(10), int64(0), int64(5), int64(0), int64(15),
			sql.NullInt64{Int64: 603, Valid: true}, sql.NullFloat64{}, now, now,
		))

	if err := ListTaskRunTraces(c); err != nil {
		t.Fatalf("ListTaskRunTraces returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Traces []AgentTraceSummary `json:"traces"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got.Traces) != 1 || got.Traces[0].AgentKind != "codex" || got.Traces[0].Usage.TotalTokens != 15 {
		t.Fatalf("unexpected response: %#v", got)
	}
	assertTraceHandlerExpectations(t, mock)
}

func TestEvaluateTaskRunTraceHandlerReturnsReport(t *testing.T) {
	db, mock, cleanup := newTraceHandlerMockDB(t)
	defer cleanup()
	_ = db

	body := []byte(`{"contains_answer":["summary"],"used_tools":["command_execution"],"max_tokens":100}`)
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ap/w/trace-smoke/runs/run-1/trace/evaluate", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("slug", "id")
	c.SetParamValues("trace-smoke", "run-1")

	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	trace := agenttrace.Trace{
		AgentKind: "codex",
		RunID:     "run-1",
		Output:    "summary created",
		ToolCalls: []agenttrace.ToolCall{{Name: "command_execution"}},
		Usage:     agenttrace.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
	}
	raw, _ := json.Marshal(trace)

	expectResolveWorkspace(mock, "trace-smoke", "user-1", "workspace-1", "viewer")
	expectGetTrace(mock, "workspace-1", "run-1", "trace-1", trace, string(raw), now)

	if err := EvaluateTaskRunTrace(c); err != nil {
		t.Fatalf("EvaluateTaskRunTrace returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got agenttrace.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.Passed || got.OverallScore != 1 || len(got.Cases) != 1 || len(got.Cases[0].ScorerResults) != 3 {
		t.Fatalf("unexpected report: %#v", got)
	}
	assertTraceHandlerExpectations(t, mock)
}

func newTraceHandlerMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, cleanup := newTraceMockDB(t)
	oldDB := ApiTypes.ProjectDBHandle
	oldAuth := EchoFactory.DefaultAuthenticator
	ApiTypes.ProjectDBHandle = db
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{UserId: "user-1", Email: "trace@example.com"}, nil
	}
	return db, mock, func() {
		ApiTypes.ProjectDBHandle = oldDB
		EchoFactory.DefaultAuthenticator = oldAuth
		cleanup()
	}
}

func expectResolveWorkspace(mock sqlmock.Sqlmock, slug, userID, workspaceID, role string) {
	mock.ExpectQuery("SELECT w.id, m.role").
		WithArgs(slug, userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role"}).AddRow(workspaceID, role))
}

func expectGetTaskRun(mock sqlmock.Sqlmock, workspaceID, runID string, now time.Time) {
	mock.ExpectQuery("SELECT").
		WithArgs(runID, workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "issue_id", "agent_id", "status",
			"queued_at", "claimed_at", "started_at", "finished_at",
			"exit_code", "error_message", "runner_version", "workdir_path", "lease_expires_at",
		}).AddRow(
			runID, workspaceID, "issue-1", "agent-1", "succeeded",
			now, sql.NullTime{}, sql.NullTime{}, sql.NullTime{},
			sql.NullInt32{}, sql.NullString{}, sql.NullString{}, sql.NullString{}, sql.NullTime{},
		))
}

func expectGetTrace(mock sqlmock.Sqlmock, workspaceID, runID, traceID string, trace agenttrace.Trace, raw string, now time.Time) {
	mock.ExpectQuery("SELECT id, workspace_id, task_run_id").
		WithArgs(workspaceID, runID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "workspace_id", "task_run_id", "agent_kind", "provider_trace_id",
			"input_text", "output_text", "tool_call_count",
			"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens", "total_tokens",
			"total_latency_ms", "total_cost_usd", "trace_json", "created_at", "updated_at",
		}).AddRow(
			traceID, workspaceID, runID, trace.AgentKind, trace.ProviderTraceID,
			trace.Input, trace.Output, int64(len(trace.ToolCalls)),
			int64(trace.Usage.InputTokens), int64(trace.Usage.CachedInputTokens), int64(trace.Usage.OutputTokens),
			int64(trace.Usage.ReasoningOutputTokens), int64(trace.Usage.TotalTokens),
			sql.NullInt64{Int64: trace.TotalLatencyMS, Valid: trace.TotalLatencyMS > 0},
			sql.NullFloat64{Float64: trace.TotalCostUSD, Valid: trace.TotalCostUSD > 0},
			raw, now, now,
		))
}

func assertTraceHandlerExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
