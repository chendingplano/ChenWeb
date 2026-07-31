package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListDocProcLogs_UsesMSUsedResponseShape(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE entry_type = $1")).
		WithArgs("doc_proc_summary").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+WHERE entry_type = \$1\s+ORDER BY create_time DESC\s+LIMIT \$2 OFFSET \$3`
	mock.ExpectQuery(listQuery).
		WithArgs("doc_proc_summary", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
			"prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
		}).AddRow(
			int64(17), "summary run", "generate_topics", `{topic-model}`, "topic-prompt", int64(81), "66% (2/3)", "doc_proc_summary",
			nil, nil, nil, nil, nil, `{"topics_generated":5}`, int64(1800), nil, "2026-05-27T12:30:00+00:00",
			nil, nil, nil,
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?entry_type=doc_proc_summary", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	results, ok := payload["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results=%#v", payload["results"])
	}
	row, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("row=%#v", results[0])
	}
	if got := row["run_id"]; got != nil {
		t.Fatalf("run_id=%v, want omitted when unset", got)
	}
	if _, ok := row["ms_used"]; !ok {
		t.Fatalf("expected ms_used in response row: %#v", row)
	}
	if got := row["record_id"]; got != float64(81) {
		t.Fatalf("record_id=%v, want 81", got)
	}
	if got := row["proc_progress"]; got != "66% (2/3)" {
		t.Fatalf("proc_progress=%v, want 66%% (2/3)", got)
	}
	if _, ok := row["duration_ms"]; ok {
		t.Fatalf("did not expect duration_ms in response row: %#v", row)
	}
	if _, ok := row["start_time"]; ok {
		t.Fatalf("did not expect start_time in response row: %#v", row)
	}
	if _, ok := row["end_time"]; ok {
		t.Fatalf("did not expect end_time in response row: %#v", row)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcLogs_AcceptsOrderParams(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs ")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+ORDER BY doc_proc_name ASC\s+LIMIT \$1 OFFSET \$2`
	mock.ExpectQuery(listQuery).
		WithArgs(50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
			"prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
		}).AddRow(
			int64(18), "summary run", "blocking", `{}`, "topic-prompt", int64(18), nil, "doc_proc_summary",
			nil, nil, nil, nil, nil, `{}`, int64(0), nil, "2026-05-27T12:30:00+00:00",
			nil, nil, nil,
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?order_by=doc_proc_name&order_dir=asc", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcLogs_FiltersByRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE doc_proc_name = $1 AND record_id = $2")).
		WithArgs("extract_entity_relation", int64(170)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+WHERE doc_proc_name = \$1 AND record_id = \$2\s+ORDER BY create_time DESC\s+LIMIT \$3 OFFSET \$4`
	mock.ExpectQuery(listQuery).
		WithArgs("extract_entity_relation", int64(170), 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
			"prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
		}).AddRow(
			int64(21), "extract entity relation", "extract_entity_relation", `{entity-model}`, "entity-prompt", int64(170), "67% (8/12)", "doc_proc_summary",
			nil, nil, nil, nil, nil, `{}`, int64(1200), nil, "2026-05-30T15:43:04+00:00",
			nil, nil, nil,
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?doc_proc_name=extract_entity_relation&record_id=170", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcLogs_FiltersByRunIDActivityAndCreateTime(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	countQuery := regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_proc_logs WHERE activity_name = $1 AND run_id = $2 AND create_time >= $3 AND create_time <= $4")
	mock.ExpectQuery(countQuery).
		WithArgs("extract_metrics_finish", int64(44), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT id, COALESCE\(call_reason,''\), doc_proc_name,.*ms_used, log_loc, COALESCE\(to_char\(create_time, .*?\), ''\),\s+prompt_cache_hit_tokens, prompt_cache_miss_tokens, run_id\s+FROM kb\.doc_proc_logs\s+WHERE activity_name = \$1 AND run_id = \$2 AND create_time >= \$3 AND create_time <= \$4\s+ORDER BY create_time DESC\s+LIMIT \$5 OFFSET \$6`
	mock.ExpectQuery(listQuery).
		WithArgs("extract_metrics_finish", int64(44), sqlmock.AnyArg(), sqlmock.AnyArg(), 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "call_reason", "doc_proc_name", "model_names", "prompt_name", "record_id", "proc_progress", "entry_type",
			"pass", "llm_call_id", "activity_name", "artifact", "errors", "extra_info", "ms_used", "log_loc", "create_time",
			"prompt_cache_hit_tokens", "prompt_cache_miss_tokens", "run_id",
		}).AddRow(
			int64(22), "extract_metrics", "extract_metrics", `{deepseek-v4-flash}`, "metric-prompt", int64(7), "100%", "extract_metrics_finish",
			nil, nil, "extract_metrics_finish", nil, nil, `{"total_metrics":57}`, int64(1200), nil, "2026-07-12T10:30:00+00:00",
			nil, nil, int64(44),
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-logs?activity_name=extract_metrics_finish&run_id=44&create_start_time=2026-07-12T00:00&create_end_time=2026-07-12T23:59", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcLogs(c); err != nil {
		t.Fatalf("ListDocProcLogs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetLatestDocProcessPlanByRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := `SELECT p.run_id, p.record_id, p.plan_facts::text, p.plan_steps::text, p.pipeline_selection::text, p.pipeline_binding::text, COALESCE\(p.pipeline_spec::text, '\{\}'::text\), COALESCE\(r.mode, ''\), COALESCE\(r.status, ''\), COALESCE\(r.processors::text, '\[\]'::text\), COALESCE\(r.parameters::text, '\{\}'::text\), COALESCE\(to_char\(p.create_time, .*?\), ''\)\s+FROM kb\.doc_process_plans p\s+JOIN kb\.doc_process_runs r ON r.id = p.run_id\s+WHERE p.record_id = \$1\s+ORDER BY p.create_time DESC, p.id DESC\s+LIMIT 1`
	mock.ExpectQuery(query).
		WithArgs(int64(4821)).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "record_id", "plan_facts", "plan_steps", "pipeline_selection", "pipeline_binding", "pipeline_spec", "mode", "status", "processors", "parameters", "create_time",
		}).AddRow(
			int64(19),
			int64(4821),
			`{"RequestedProcessors":["extract_metrics"],"RequestedPipeline":"","StoreBoundPipeline":"","KnowledgeStoreID":0,"KnowledgeStoreType":"","InputDocType":"pdf","SourceLanguage":"en","DocumentNumber":"YY 9706.252-2021","ParserName":"opendata","DocumentTitle":"","RoutingFacets":{"KnowledgeStoreBinding":"absent","InputDocType":"pdf","SourceLanguage":"en","HasDocumentNumber":true}}`,
			`[{"Name":"static_analyzer","Phase":"A","DependsOn":[],"Reason":"mandatory_baseline"},{"Name":"chunking","Phase":"A","DependsOn":["static_analyzer"],"Reason":"implicit_dependency"},{"Name":"extract_metrics","Phase":"B","DependsOn":["chunking"],"Reason":"explicit_request"}]`,
			`{"PipelineName":"legacy_default","Reason":"system_default"}`,
			`{"RequestedPipeline":"","StoreBoundPipeline":"","Source":"system_default","SelectedPipeline":"legacy_default"}`,
			`{"Name":"legacy_default","DisplayName":"Legacy Default","Processors":null,"LegacyEquivalent":true}`,
			"auto",
			"success",
			`["extract_metrics"]`,
			`{"processor_pipeline_selection":{"PipelineName":"legacy_default","Reason":"system_default"}}`,
			"2026-07-31T14:22:00+00:00",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-plans/latest?record_id=4821", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetLatestDocProcessPlan(c); err != nil {
		t.Fatalf("GetLatestDocProcessPlan returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if payload["status"] != true {
		t.Fatalf("payload status=%v", payload["status"])
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("result=%#v", payload["result"])
	}
	if got := result["run_id"]; got != float64(19) {
		t.Fatalf("run_id=%v want 19", got)
	}
	if got := result["mode"]; got != "auto" {
		t.Fatalf("mode=%v want auto", got)
	}
	if got := result["status"]; got != "success" {
		t.Fatalf("status=%v want success", got)
	}
	selection, ok := result["pipeline_selection"].(map[string]any)
	if !ok {
		t.Fatalf("pipeline_selection=%#v", result["pipeline_selection"])
	}
	if got := selection["PipelineName"]; got != "legacy_default" {
		t.Fatalf("pipeline_selection.PipelineName=%v", got)
	}
	binding, ok := result["pipeline_binding"].(map[string]any)
	if !ok {
		t.Fatalf("pipeline_binding=%#v", result["pipeline_binding"])
	}
	if got := binding["Source"]; got != "system_default" {
		t.Fatalf("pipeline_binding.Source=%v", got)
	}
	steps, ok := result["plan_steps"].([]any)
	if !ok || len(steps) != 3 {
		t.Fatalf("plan_steps=%#v", result["plan_steps"])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcessPlansByRecordID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	countQuery := `SELECT COUNT\(\*\)\s+FROM kb\.doc_process_plans p\s+JOIN kb\.doc_process_runs r ON r.id = p.run_id\s+WHERE p.record_id = \$1`
	mock.ExpectQuery(countQuery).
		WithArgs(int64(4821)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	listQuery := `SELECT p.run_id, p.record_id, p.plan_facts::text, p.plan_steps::text, p.pipeline_selection::text, p.pipeline_binding::text, COALESCE\(p.pipeline_spec::text, '\{\}'::text\), COALESCE\(r.mode, ''\), COALESCE\(r.status, ''\), COALESCE\(r.processors::text, '\[\]'::text\), COALESCE\(r.parameters::text, '\{\}'::text\), COALESCE\(to_char\(p.create_time, .*?\), ''\)\s+FROM kb\.doc_process_plans p\s+JOIN kb\.doc_process_runs r ON r.id = p.run_id\s+WHERE p.record_id = \$1\s+ORDER BY p.create_time DESC, p.id DESC\s+LIMIT \$2 OFFSET \$3`
	mock.ExpectQuery(listQuery).
		WithArgs(int64(4821), 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "record_id", "plan_facts", "plan_steps", "pipeline_selection", "pipeline_binding", "pipeline_spec", "mode", "status", "processors", "parameters", "create_time",
		}).AddRow(
			int64(20),
			int64(4821),
			`{"RequestedProcessors":["extract_metrics"]}`,
			`[{"Name":"extract_metrics","Phase":"B","DependsOn":["chunking"],"Reason":"explicit_request"}]`,
			`{"PipelineName":"legacy_default","Reason":"system_default"}`,
			`{"RequestedPipeline":"","StoreBoundPipeline":"","Source":"system_default","SelectedPipeline":"legacy_default"}`,
			`{"Name":"legacy_default","DisplayName":"Legacy Default","Processors":null,"LegacyEquivalent":true}`,
			"auto",
			"success",
			`["extract_metrics"]`,
			`{"processor_pipeline_selection":{"PipelineName":"legacy_default","Reason":"system_default"}}`,
			"2026-07-31T14:23:00+00:00",
		).AddRow(
			int64(19),
			int64(4821),
			`{"RequestedProcessors":["generate_topics"]}`,
			`[{"Name":"generate_topics","Phase":"B","DependsOn":["chunking"],"Reason":"explicit_request"}]`,
			`{"PipelineName":"legacy_default","Reason":"system_default"}`,
			`{"RequestedPipeline":"","StoreBoundPipeline":"","Source":"system_default","SelectedPipeline":"legacy_default"}`,
			`{"Name":"legacy_default","DisplayName":"Legacy Default","Processors":null,"LegacyEquivalent":true}`,
			"auto",
			"failed",
			`["generate_topics"]`,
			`{"processor_pipeline_selection":{"PipelineName":"legacy_default","Reason":"system_default"}}`,
			"2026-07-31T14:22:00+00:00",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-plans?record_id=4821&page=1&page_size=50", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcessPlans(c); err != nil {
		t.Fatalf("ListDocProcessPlans returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if payload["status"] != true {
		t.Fatalf("payload status=%v", payload["status"])
	}
	if got := payload["total"]; got != float64(2) {
		t.Fatalf("total=%v want 2", got)
	}
	results, ok := payload["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("results=%#v", payload["results"])
	}
	first, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("first=%#v", results[0])
	}
	if got := first["run_id"]; got != float64(20) {
		t.Fatalf("first run_id=%v want 20", got)
	}
	if got := first["status"]; got != "success" {
		t.Fatalf("first status=%v want success", got)
	}
	second, ok := results[1].(map[string]any)
	if !ok {
		t.Fatalf("second=%#v", results[1])
	}
	if got := second["run_id"]; got != float64(19) {
		t.Fatalf("second run_id=%v want 19", got)
	}
	if got := second["status"]; got != "failed" {
		t.Fatalf("second status=%v want failed", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcessPlansByRecordID_AcceptsStatusModeAndPipelineFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	countQuery := `SELECT COUNT\(\*\)\s+FROM kb\.doc_process_plans p\s+JOIN kb\.doc_process_runs r ON r.id = p.run_id\s+WHERE p.record_id = \$1 AND COALESCE\(r.status, ''\) = \$2 AND COALESCE\(r.mode, ''\) = \$3 AND p.pipeline_selection->>'PipelineName' = \$4`
	mock.ExpectQuery(countQuery).
		WithArgs(int64(4821), "success", "auto", "legacy_default").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	listQuery := `SELECT p.run_id, p.record_id, p.plan_facts::text, p.plan_steps::text, p.pipeline_selection::text, p.pipeline_binding::text, COALESCE\(p.pipeline_spec::text, '\{\}'::text\), COALESCE\(r.mode, ''\), COALESCE\(r.status, ''\), COALESCE\(r.processors::text, '\[\]'::text\), COALESCE\(r.parameters::text, '\{\}'::text\), COALESCE\(to_char\(p.create_time, .*?\), ''\)\s+FROM kb\.doc_process_plans p\s+JOIN kb\.doc_process_runs r ON r.id = p.run_id\s+WHERE p.record_id = \$1 AND COALESCE\(r.status, ''\) = \$2 AND COALESCE\(r.mode, ''\) = \$3 AND p.pipeline_selection->>'PipelineName' = \$4\s+ORDER BY p.create_time DESC, p.id DESC\s+LIMIT \$5 OFFSET \$6`
	mock.ExpectQuery(listQuery).
		WithArgs(int64(4821), "success", "auto", "legacy_default", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"run_id", "record_id", "plan_facts", "plan_steps", "pipeline_selection", "pipeline_binding", "pipeline_spec", "mode", "status", "processors", "parameters", "create_time",
		}).AddRow(
			int64(20),
			int64(4821),
			`{"RequestedProcessors":["extract_metrics"]}`,
			`[{"Name":"extract_metrics","Phase":"B","DependsOn":["chunking"],"Reason":"explicit_request"}]`,
			`{"PipelineName":"legacy_default","Reason":"system_default"}`,
			`{"RequestedPipeline":"","StoreBoundPipeline":"","Source":"system_default","SelectedPipeline":"legacy_default"}`,
			`{"Name":"legacy_default","DisplayName":"Legacy Default","Processors":null,"LegacyEquivalent":true}`,
			"auto",
			"success",
			`["extract_metrics"]`,
			`{"processor_pipeline_selection":{"PipelineName":"legacy_default","Reason":"system_default"}}`,
			"2026-07-31T14:23:00+00:00",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-proc-plans?record_id=4821&status=success&mode=auto&pipeline_name=legacy_default&page=1&page_size=50", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDocProcessPlans(c); err != nil {
		t.Fatalf("ListDocProcessPlans returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if payload["status"] != true {
		t.Fatalf("payload status=%v", payload["status"])
	}
	if got := payload["total"]; got != float64(1) {
		t.Fatalf("total=%v want 1", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestPipelineProcessorsMatchExecuted(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		spec     docprocessing.ProductionPipelineSpec
		executed []string
		want     *bool
	}{
		{
			name:     "nil when pipeline declares no explicit processors",
			spec:     docprocessing.ProductionPipelineSpec{Name: "legacy_default"},
			executed: []string{"static_analyzer", "chunking", "extract_metrics"},
			want:     nil,
		},
		{
			name:     "matches regardless of order",
			spec:     docprocessing.ProductionPipelineSpec{Name: "narrative_default", Processors: []string{"extract_metrics", "extract_provisions"}},
			executed: []string{"extract_provisions", "extract_metrics"},
			want:     &trueVal,
		},
		{
			name:     "mismatch when executed set differs",
			spec:     docprocessing.ProductionPipelineSpec{Name: "narrative_default", Processors: []string{"extract_metrics", "extract_provisions"}},
			executed: []string{"extract_metrics"},
			want:     &falseVal,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pipelineProcessorsMatchExecuted(tc.spec, tc.executed)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("got=%v want=nil", *got)
				}
				return
			}
			if got == nil || *got != *tc.want {
				t.Fatalf("got=%v want=%v", got, *tc.want)
			}
		})
	}
}
