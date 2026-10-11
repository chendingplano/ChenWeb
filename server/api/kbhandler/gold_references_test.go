package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func withMockProjectDB(t *testing.T) sqlmock.Sqlmock {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() {
		ApiTypes.ProjectDBHandle = old
		db.Close()
	})
	return mock
}

func TestListGoldReferencesRequiresRun(t *testing.T) {
	withMockProjectDB(t)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/references?input_record_id=416&source=testbed&skill_version=1.0.0", nil), rec)
	if err := ListReferences(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListGoldReferencesReadsRun(t *testing.T) {
	mock := withMockProjectDB(t)
	args := []any{int64(416), "1.0.0", "claude-opus-5-5", "20261011_095742"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.ref_mentions m")).WithArgs(args[0], args[1], args[2], args[3]).WillReturnRows(
		sqlmock.NewRows([]string{"id", "ref_id", "section", "source_line_spans", "evidence", "cited_raw", "source_table_rows", "target_type", "target_subtype", "target_title", "target_version", "target_content", "normativity", "list_kind", "list_mismatch", "identifiers", "ref_form", "mechanism", "in_requirement", "internal", "confidence"}).
			AddRow(int64(7), "416_gref_2", "2", `["22"]`, "GB 14554 恶臭污染物排放标准", "GB 14554", []byte(`[]`), "standard", "national_standard", "恶臭污染物排放标准", "", "general", "normative", "normative", "", `["standard: GB 14554"]`, "listed", "standard_number", false, false, 0.95))
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.ref_mentions_dropped")).WithArgs(args[0], args[1], args[2], args[3]).WillReturnRows(
		sqlmock.NewRows([]string{"id", "candidate_id", "decision", "drop_reason", "candidate_data"}).
			AddRow(int64(3), "c1", "excluded", "X3", `{"reason":"ICS code","cited_raw":"ICS 13.030.10","source_line_spans":["1"],"ref_ids":[]}`))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/references?input_record_id=416&source=testbed&skill_version=1.0.0&model_name=claude-opus-5-5&run_id=20261011_095742", nil), rec)
	if err := ListReferences(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response listReferencesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].RefForm != "listed" || response.Results[0].Identifiers[0] != "standard: GB 14554" ||
		response.Results[0].Confidence == nil || *response.Results[0].Confidence != 0.95 {
		t.Fatalf("unexpected results: %s", rec.Body.String())
	}
	if len(response.Dropped) != 1 || response.Dropped[0].Reason != "ICS code" || response.Dropped[0].SourceLineSpans[0] != "1" {
		t.Fatalf("unexpected dropped: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListGoldReferenceOptionsReadsReferenceRuns(t *testing.T) {
	mock := withMockProjectDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.reference_benchmark_runs")).WillReturnRows(
		sqlmock.NewRows([]string{"skill_version", "model_name", "record_count", "run_count"}).AddRow("1.0.0", "claude-opus-5-5", 1, 1))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/references/gold-options", nil), rec)
	if err := ListGoldReferenceOptions(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"skill_versions":["1.0.0"]`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildWhereClauseGoldKind(t *testing.T) {
	whereSQL, _, err := buildWhereClause(listInputsFilters{ParseState: "all", HasGoldMetrics: true, GoldKind: "references"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(whereSQL, "EXISTS (SELECT 1 FROM testbed.reference_benchmark_runs t WHERE t.input_record_id = i.id)") {
		t.Fatalf("where=%s", whereSQL)
	}
	if _, _, err := buildWhereClause(listInputsFilters{ParseState: "all", HasGoldMetrics: true, GoldKind: "testbed.metrics; --"}); err == nil {
		t.Fatal("expected an error for an unknown gold_kind")
	}
}
