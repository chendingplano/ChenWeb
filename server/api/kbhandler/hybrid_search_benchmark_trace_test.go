package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestGetHybridSearchBenchmarkTrace(t *testing.T) {
	_, mock := installPolicyDB(t)
	mock.ExpectQuery("SELECT target_artifact_id FROM testbed\\.hybrid_search_benchmark_results").WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"target_artifact_id"}).AddRow("416_mtc_9"))
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
		Target  string                  `json:"target_artifact_id"`
		Queries []hybridBenchTraceQuery `json:"queries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Target != "416_mtc_9" || len(body.Queries) != 2 {
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
	mock.ExpectQuery("SELECT target_artifact_id").WithArgs(int64(6)).WillReturnRows(sqlmock.NewRows([]string{"target_artifact_id"}))
	if rec := callHybridBenchTrace(t, "6", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("missing result: status=%d", rec.Code)
	}
	// A result stored before tracing: no queries, no candidate lookups.
	mock.ExpectQuery("SELECT target_artifact_id").WithArgs(int64(7)).WillReturnRows(sqlmock.NewRows([]string{"target_artifact_id"}).AddRow("t"))
	mock.ExpectQuery("FROM testbed\\.hybrid_search_benchmark_queries").WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "seq", "source", "attempt", "query_text", "keywords", "error_msg"}))
	if rec := callHybridBenchTrace(t, "7", ""); rec.Code != http.StatusOK || rec.Body.String() != "{\"limit\":20,\"queries\":[],\"result_id\":7,\"target_artifact_id\":\"t\"}\n" {
		t.Fatalf("old result: status=%d body=%s", rec.Code, rec.Body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
