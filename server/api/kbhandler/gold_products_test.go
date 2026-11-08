package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestListGoldProductsRequiresRun(t *testing.T) {
	mock := withMockProjectDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("to_regclass")).WillReturnError(sqlmock.ErrCancelled)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/products?input_record_id=416&source=testbed&skill_version=1.0.0", nil), rec)
	if err := ListProducts(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestListGoldProductsReadsRun(t *testing.T) {
	mock := withMockProjectDB(t)
	// The input-table lookup for file_name is best effort.
	mock.ExpectQuery(regexp.QuoteMeta("to_regclass")).WillReturnError(sqlmock.ErrCancelled)
	args := []driver.Value{int64(416), "1.0.0", "claude-opus-5-5", "20261108_150000"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.products p")).WithArgs(args...).WillReturnRows(
		sqlmock.NewRows([]string{"id", "product_rel_id", "product_key", "section", "rule_id", "product_name",
			"product_name_en", "canonical_name", "canonical_name_en", "product_type", "relation_type",
			"relation_summary", "relation_summary_en", "evidence_quote", "evidence_lines", "obligation_level",
			"requirement_text", "requirement_text_en", "conditions", "exceptions", "parameters", "related_products",
			"responsible_actor", "confidence", "model_name", "created_at"}).
			AddRow(int64(9), "416_gprd_1", "416_gp_1", "7.1 投放要求", "R1", "垃圾分类收集容器",
				"Sorted waste collection container", "垃圾分类收集容器", "Sorted waste collection container",
				"equipment", "design_requirement", "文件要求容器有盖。", "", "垃圾分类收集容器应有盖", `["89"]`,
				"mandatory", "垃圾分类收集容器应有盖", "", `[]`, `[]`, `[]`, `[]`, "", 0.95, "claude-opus-5-5",
				time.Date(2026, 11, 8, 15, 0, 0, 0, time.UTC)))
	mock.ExpectQuery(regexp.QuoteMeta("FROM testbed.products_dropped")).WithArgs(args...).WillReturnRows(
		sqlmock.NewRows([]string{"id", "candidate_id", "decision", "drop_reason", "candidate_data"}).
			AddRow(int64(3), "c2", "excluded", "X1", `{"reason":"A station","text":"垃圾转运站","source_line_spans":["50"],"product_keys":[]}`))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/kb/products?input_record_id=416&source=testbed&skill_version=1.0.0&model_name=claude-opus-5-5&run_id=20261108_150000", nil), rec)
	if err := ListProducts(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response listProductsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Results) != 1 || response.Results[0].ProductKey != "416_gp_1" || response.Results[0].RuleID != "R1" ||
		response.Results[0].RelationType != "design_requirement" || string(response.Results[0].EvidenceLines) != `["89"]` {
		t.Fatalf("results=%+v", response.Results)
	}
	if len(response.Dropped) != 1 || response.Dropped[0].DropReason != "X1" || response.Dropped[0].Text != "垃圾转运站" ||
		len(response.Dropped[0].SourceLineSpans) != 1 || response.Dropped[0].ProductKeys == nil {
		t.Fatalf("dropped=%+v", response.Dropped)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGoldRunTablesIncludeProducts(t *testing.T) {
	if table, ok := goldRunTable("products"); !ok || table != "testbed.product_benchmark_runs" {
		t.Fatalf("products gold table = %q, %v", table, ok)
	}
}
