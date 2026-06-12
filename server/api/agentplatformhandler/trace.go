package agentplatformhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/agenttrace"
	"github.com/labstack/echo/v4"
)

func ListTaskRunTraces(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_TRACE_LIST")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	filter := TraceListFilter{
		AgentKind: strings.TrimSpace(c.QueryParam("agent_kind")),
		Limit:     100,
	}
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			return jsonError(c, http.StatusBadRequest, "limit must be a positive integer")
		}
		filter.Limit = n
	}
	items, err := listTaskRunTraces(db, wid, filter)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list traces: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"traces": items})
}

func GetTaskRunTrace(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_TRACE_GET")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	runID := strings.TrimSpace(c.Param("id"))
	if runID == "" {
		return jsonError(c, http.StatusBadRequest, "run id required")
	}
	if _, err := getTaskRun(db, wid, runID); err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "run not found")
	} else if err != nil {
		return jsonError(c, http.StatusInternalServerError, "check run: "+err.Error())
	}
	rec, err := getTaskRunTrace(db, wid, runID)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "trace not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "get trace: "+err.Error())
	}
	return c.JSON(http.StatusOK, rec)
}

func EvaluateTaskRunTrace(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_TRACE_EVAL")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	runID := strings.TrimSpace(c.Param("id"))
	if runID == "" {
		return jsonError(c, http.StatusBadRequest, "run id required")
	}
	rec, err := getTaskRunTrace(db, wid, runID)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "trace not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "get trace: "+err.Error())
	}
	var req TraceEvaluationRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	return c.JSON(http.StatusOK, buildTraceEvaluationReport(rec.Trace, req))
}

func upsertTaskRunTrace(db *sql.DB, workspaceID, runID string, trace agenttrace.Trace) error {
	trace.RunID = runID
	trace.AgentKind = strings.TrimSpace(trace.AgentKind)
	traceJSON, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("marshal trace: %w", err)
	}
	stmt := `
		INSERT INTO ap_agent_trace (
			workspace_id, task_run_id, agent_kind, provider_trace_id,
			input_text, output_text, tool_call_count,
			input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens,
			total_latency_ms, total_cost_usd, trace_json
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb)
		ON CONFLICT (task_run_id) DO UPDATE SET
			workspace_id = EXCLUDED.workspace_id,
			agent_kind = EXCLUDED.agent_kind,
			provider_trace_id = EXCLUDED.provider_trace_id,
			input_text = EXCLUDED.input_text,
			output_text = EXCLUDED.output_text,
			tool_call_count = EXCLUDED.tool_call_count,
			input_tokens = EXCLUDED.input_tokens,
			cached_input_tokens = EXCLUDED.cached_input_tokens,
			output_tokens = EXCLUDED.output_tokens,
			reasoning_output_tokens = EXCLUDED.reasoning_output_tokens,
			total_tokens = EXCLUDED.total_tokens,
			total_latency_ms = EXCLUDED.total_latency_ms,
			total_cost_usd = EXCLUDED.total_cost_usd,
			trace_json = EXCLUDED.trace_json,
			updated_at = NOW()
	`
	_, err = db.Exec(stmt,
		workspaceID,
		runID,
		trace.AgentKind,
		trace.ProviderTraceID,
		trace.Input,
		trace.Output,
		int64(len(trace.ToolCalls)),
		int64(trace.Usage.InputTokens),
		int64(trace.Usage.CachedInputTokens),
		int64(trace.Usage.OutputTokens),
		int64(trace.Usage.ReasoningOutputTokens),
		int64(trace.Usage.TotalTokens),
		nullableInt64(trace.TotalLatencyMS),
		nullableFloat64(trace.TotalCostUSD),
		string(traceJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert agent trace: %w", err)
	}
	return nil
}

func listTaskRunTraces(db *sql.DB, workspaceID string, filter TraceListFilter) ([]AgentTraceSummary, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	agentKind := strings.TrimSpace(filter.AgentKind)
	stmt := `
		SELECT id, workspace_id, task_run_id, agent_kind, provider_trace_id,
		       output_text, tool_call_count,
		       input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens,
		       total_latency_ms, total_cost_usd, created_at, updated_at
		FROM ap_agent_trace
		WHERE workspace_id=$1 AND ($2 = '' OR agent_kind = $2)
		ORDER BY created_at DESC
		LIMIT $3
	`
	rows, err := db.Query(stmt, workspaceID, agentKind, limit)
	if err != nil {
		return nil, fmt.Errorf("list agent traces: %w", err)
	}
	defer rows.Close()

	out := make([]AgentTraceSummary, 0)
	for rows.Next() {
		var item AgentTraceSummary
		var inputTokens, cachedInputTokens, outputTokens, reasoningOutputTokens, totalTokens int64
		var latency sql.NullInt64
		var cost sql.NullFloat64
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.TaskRunID, &item.AgentKind, &item.ProviderTraceID,
			&item.OutputText, &item.ToolCallCount,
			&inputTokens, &cachedInputTokens, &outputTokens, &reasoningOutputTokens, &totalTokens,
			&latency, &cost, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if latency.Valid {
			v := latency.Int64
			item.TotalLatencyMS = &v
		}
		if cost.Valid {
			v := cost.Float64
			item.TotalCostUSD = &v
		}
		item.Usage = agenttrace.TokenUsage{
			InputTokens:           int(inputTokens),
			CachedInputTokens:     int(cachedInputTokens),
			OutputTokens:          int(outputTokens),
			ReasoningOutputTokens: int(reasoningOutputTokens),
			TotalTokens:           int(totalTokens),
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func getTaskRunTrace(db *sql.DB, workspaceID, runID string) (AgentTraceRecord, error) {
	const stmt = `
		SELECT id, workspace_id, task_run_id, agent_kind, provider_trace_id,
		       input_text, output_text, tool_call_count,
		       input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens, total_tokens,
		       total_latency_ms, total_cost_usd, trace_json, created_at, updated_at
		FROM ap_agent_trace
		WHERE workspace_id=$1 AND task_run_id=$2
	`
	var rec AgentTraceRecord
	var inputTokens, cachedInputTokens, outputTokens, reasoningOutputTokens, totalTokens int64
	var latency sql.NullInt64
	var cost sql.NullFloat64
	var raw string
	err := db.QueryRow(stmt, workspaceID, runID).Scan(
		&rec.ID, &rec.WorkspaceID, &rec.TaskRunID, &rec.AgentKind, &rec.ProviderTraceID,
		&rec.InputText, &rec.OutputText, &rec.ToolCallCount,
		&inputTokens, &cachedInputTokens, &outputTokens, &reasoningOutputTokens, &totalTokens,
		&latency, &cost, &raw, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return AgentTraceRecord{}, err
	}
	if latency.Valid {
		v := latency.Int64
		rec.TotalLatencyMS = &v
	}
	if cost.Valid {
		v := cost.Float64
		rec.TotalCostUSD = &v
	}
	rec.Usage = agenttrace.TokenUsage{
		InputTokens:           int(inputTokens),
		CachedInputTokens:     int(cachedInputTokens),
		OutputTokens:          int(outputTokens),
		ReasoningOutputTokens: int(reasoningOutputTokens),
		TotalTokens:           int(totalTokens),
	}
	if err := json.Unmarshal([]byte(raw), &rec.Trace); err != nil {
		return AgentTraceRecord{}, fmt.Errorf("unmarshal trace json: %w", err)
	}
	return rec, nil
}

func buildTraceEvaluationReport(trace agenttrace.Trace, req TraceEvaluationRequest) agenttrace.Report {
	scorers := make([]agenttrace.Scorer, 0)
	if len(req.ContainsAnswer) > 0 {
		scorers = append(scorers, agenttrace.ContainsAnswer(req.ContainsAnswer...))
	}
	if len(req.UsedTools) > 0 {
		scorers = append(scorers, agenttrace.UsedTools(req.UsedTools...))
	}
	if len(req.AvoidedTools) > 0 {
		scorers = append(scorers, agenttrace.AvoidedTools(req.AvoidedTools...))
	}
	if req.MaxTokens != nil {
		scorers = append(scorers, agenttrace.UnderTokenLimit(*req.MaxTokens))
	}
	return agenttrace.RunEvaluations([]agenttrace.EvalRun{{
		Case: agenttrace.TestCase{
			Name:    "ad_hoc_trace_evaluation",
			Scorers: scorers,
		},
		Trace: trace,
	}})
}

func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

func nullableFloat64(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}
