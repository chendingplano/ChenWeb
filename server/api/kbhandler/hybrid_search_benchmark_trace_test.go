package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func callHybridBenchTrace(t *testing.T, id, query string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/hybrid-search-benchmark/results/"+id+"/trace"+query, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(id)
	if err := GetHybridSearchBenchmarkTrace(c); err != nil {
		t.Fatalf("handler error: %v", err)
	}
	return rec
}

var hybridBenchResultRowColumns = []string{"id", "search_run_id", "question_id", "input_record_id", "hop_type", "lang", "question",
	"target_artifact_id", "target_artifact_type", "keywords", "lexical_rank", "semantic_rank", "hybrid_rank", "hybrid_top", "error_msg",
	"embedding_ms", "keywords_ms", "lexical_ms", "semantic_ms", "hybrid_ms",
	"effort_rank", "effort_found", "effort_attempts", "effort_ms", "effort_trace", "lexical_backend"}

// One stored result (low effort) plus its run's lexical backend.
func hybridBenchResultRow(id int64, target string) *sqlmock.Rows {
	return sqlmock.NewRows(hybridBenchResultRowColumns).AddRow(id, int64(612), int64(7), int64(416), "one_hop", "en", "q",
		target, "metric", "{cadmium}", int64(2), int64(1), int64(2), `["416_mtc_1"]`, "",
		int64(40), int64(900), int64(120), int64(110), int64(125),
		nil, nil, nil, nil, nil, "paradedb")
}

func TestGetHybridSearchBenchmarkTrace(t *testing.T) {
	_, mock := installPolicyDB(t)
	mock.ExpectQuery("SELECT u\\.lexical_backend FROM testbed\\.hybrid_search_benchmark_runs u WHERE u\\.id = r\\.run_id\\)\\s+FROM testbed\\.hybrid_search_benchmark_results r WHERE r\\.id = \\$1").
		WithArgs(int64(5)).WillReturnRows(hybridBenchResultRow(5, "416_mtc_9"))
	mock.ExpectQuery("FROM testbed\\.hybrid_search_benchmark_queries WHERE result_id = \\$1 ORDER BY seq").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "seq", "source", "attempt", "query_text", "keywords", "error_msg"}).
			AddRow(int64(9), 1, "question", nil, "q", "{cadmium}", "").
			AddRow(int64(10), 2, "effort", int64(1), "x", "{}", hybridBenchErrNoTrace))
	mock.ExpectQuery("SELECT c\\.query_id, c\\.stage, COUNT").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"query_id", "stage", "count"}).AddRow(int64(9), "lexical", 200))
	// First 5 rows per stage, plus the target wherever it ranks.
	mock.ExpectQuery("LEFT JOIN kb\\.search_artifacts sa .*c\\.stage_rank <= \\$2 OR c\\.artifact_id = \\$3").WithArgs(int64(5), 5, "416_mtc_9").
		WillReturnRows(sqlmock.NewRows([]string{"query_id", "stage", "stage_rank", "partition_name", "artifact_type", "artifact_id", "label", "score", "partition_rank", "lexical_rank", "semantic_rank"}).
			AddRow(int64(9), "lexical", 1, "kb.search_artifacts_metric", "metric", "416_mtc_1", "Lead", 30.5, int64(1), nil, nil).
			AddRow(int64(9), "lexical", 44, "kb.search_artifacts_metric", "metric", "416_mtc_9", "", 3.2, int64(44), nil, nil))

	rec := callHybridBenchTrace(t, "5", "?limit=5")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Result  hybridBenchResult       `json:"result"`
		Backend string                  `json:"lexical_backend"`
		Queries []hybridBenchTraceQuery `json:"queries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Result.TargetArtifactID != "416_mtc_9" || body.Result.ID != 5 || *body.Result.HybridRank != 2 || body.Backend != "paradedb" || len(body.Queries) != 2 {
		t.Fatalf("body=%s", rec.Body)
	}
	q := body.Queries[0]
	if q.Counts["lexical"] != 200 || len(q.Stages["lexical"]) != 2 || q.Stages["lexical"][1].Rank != 44 || *q.Stages["lexical"][1].PartitionRank != 44 {
		t.Fatalf("question query=%+v", q)
	}
	if e := body.Queries[1]; e.Attempt == nil || *e.Attempt != 1 || len(e.Stages) != 0 || e.ErrorMsg != hybridBenchErrNoTrace {
		t.Fatalf("effort query=%+v", e)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetHybridSearchBenchmarkTraceErrors(t *testing.T) {
	_, mock := installPolicyDB(t)
	if rec := callHybridBenchTrace(t, "5", "?limit=0"); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit 0: status=%d", rec.Code)
	}
	if rec := callHybridBenchTrace(t, "x", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: status=%d", rec.Code)
	}
	mock.ExpectQuery("FROM testbed\\.hybrid_search_benchmark_results r WHERE r\\.id").WithArgs(int64(6)).WillReturnRows(sqlmock.NewRows(hybridBenchResultRowColumns))
	if rec := callHybridBenchTrace(t, "6", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing result: status=%d", rec.Code)
	}
	// A result stored before tracing: no queries, no candidate lookups.
	mock.ExpectQuery("FROM testbed\\.hybrid_search_benchmark_results r WHERE r\\.id").WithArgs(int64(7)).WillReturnRows(hybridBenchResultRow(7, "t"))
	mock.ExpectQuery("FROM testbed\\.hybrid_search_benchmark_queries").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "seq", "source", "attempt", "query_text", "keywords", "error_msg"}))
	if rec := callHybridBenchTrace(t, "7", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"queries":[]`) || !strings.Contains(rec.Body.String(), `"target_artifact_id":"t"`) {
		t.Fatalf("old result: status=%d body=%s", rec.Code, rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
