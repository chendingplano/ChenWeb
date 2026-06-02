package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListSemanticProjectionsReturnsLineSpans(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("to_regclass").
		WithArgs("kb.input", "kb.inputs").
		WillReturnRows(sqlmock.NewRows([]string{"singular", "plural"}).AddRow("kb.inputs", nil))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT file_name FROM kb.inputs WHERE id = $1`)).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{"file_name"}).AddRow("manual.pdf"))

	query := regexp.QuoteMeta(`
SELECT
	id, semantic_proj_id, input_record_id, event_id, language,
	descriptive_name, descriptive_name_en, keywords, keywords_en,
	semantic_projection, semantic_projection_en, category_paths, category_paths_en,
	line_spans, model_name, prompt_name, create_time
FROM kb.semantic_projections
WHERE input_record_id = $1
ORDER BY id`)
	mock.ExpectQuery(query).
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "semantic_proj_id", "input_record_id", "event_id", "language",
			"descriptive_name", "descriptive_name_en", "keywords", "keywords_en",
			"semantic_projection", "semantic_projection_en", "category_paths", "category_paths_en",
			"line_spans", "model_name", "prompt_name", "create_time",
		}).AddRow(
			int64(1), "12_0_1", int64(12), "evt-12", "en",
			"Pump identity", "Pump identity", `["pump"]`, `[]`,
			"Maps pump mentions to a semantic identity.", "",
			`[]`, `[]`, `["12:14","18"]`, "gpt-test", "prompt-enrich-semantic-projection-v1.md",
			mustParseTestTime("2026-06-02T12:00:00Z"),
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/semantic-projections?input_record_id=12", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := ListSemanticProjections(c); err != nil {
		t.Fatalf("ListSemanticProjections: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status  bool `json:"status"`
		Results []struct {
			SemanticProjID string   `json:"semantic_proj_id"`
			LineSpans      []string `json:"line_spans"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true")
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results len=%d, want 1", len(payload.Results))
	}
	if payload.Results[0].SemanticProjID != "12_0_1" {
		t.Fatalf("semantic_proj_id=%q, want 12_0_1", payload.Results[0].SemanticProjID)
	}
	if got := payload.Results[0].LineSpans; len(got) != 2 || got[0] != "12:14" || got[1] != "18" {
		t.Fatalf("line_spans=%v", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
