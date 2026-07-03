package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListInventoryItemsReturnsRows(t *testing.T) {
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

	mock.ExpectQuery("FROM kb.inventory_items").
		WithArgs(int64(12)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "inventory_item_id", "item_name", "canonical_name", "item_categories",
			"manufacturer", "brand", "model_number", "part_number",
			"normalized_specs", "raw_specs", "standards", "aliases",
			"evidence_quote", "source_line_spans", "validation_flags", "missing_required_attrs",
			"dedupe_key", "schema_version", "dictionary_version", "confidence", "confidence_reason",
			"model_name", "prompt_name", "create_time", "modify_time",
		}).AddRow(
			int64(1), "12_i_1", "Bosch Pump", "Bosch pump", `["pump"]`,
			"Bosch", "Bosch", "", "",
			`[{"name":"power","value":1500,"unit":"W"}]`, `[{"name":"power","value":1500,"unit":"w"}]`, `["ISO 9001"]`, `["Pmp Bsch"]`,
			"Pump, Bosch, 1500w", `["5"]`, `[]`, `[]`,
			"pump|bosch|||power=1500W", "schema-v1", "dict-v1", 0.9, "explicit",
			"gpt-test", "prompt-extract-inventory-items-v1.md", mustParseTestTime("2026-05-31T00:00:00Z"), mustParseTestTime("2026-05-31T00:00:00Z"),
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inventory-items?input_record_id=12", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := ListInventoryItems(c); err != nil {
		t.Fatalf("ListInventoryItems: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !regexp.MustCompile(`"inventory_item_id":"12_i_1"`).MatchString(rec.Body.String()) {
		t.Fatalf("response missing inventory item: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSearchInventoryItemsAppliesInventoryFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.search_artifacts sa WHERE").
		WithArgs("pump", "inventory_item", "pump", "Bosch", "Bosch", "1500").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("WITH query_input AS").
		WithArgs("pump", "inventory_item", "pump", "Bosch", "Bosch", "1500", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"artifact_type", "artifact_id", "input_record_id", "primary_label", "secondary_label",
			"source_title", "source_filename", "source_line_spans", "semantic_payload", "keywords", "score", "snippet",
		}).AddRow(
			"inventory_item", "12_inv_1", int64(12), "Bosch Pump", "pump",
			"manual.pdf", "manual.pdf", `["5"]`,
			`{"item_categories":["pump"],"manufacturer":"Bosch","brand":"Bosch","model_number":"1500"}`,
			`{pump}`, 0.8, "Bosch Pump",
		))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/inventory-items/search?q=pump&item_categories=pump&manufacturer=Bosch&brand=Bosch&model_number=1500", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := SearchInventoryItems(c); err != nil {
		t.Fatalf("SearchInventoryItems: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !regexp.MustCompile(`"artifact_type":"inventory_item"`).MatchString(rec.Body.String()) {
		t.Fatalf("response missing inventory search result: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestBuildRegistrySearchWhereClauseIncludesInventoryFilters(t *testing.T) {
	filters := artifactSearchFilters{
		InventoryCategory: "pump",
		Manufacturer:      "Bosch",
		Brand:             "Bosch",
		ModelNumber:       "1500",
		ValidationStatus:  "valid",
	}
	where, args := buildRegistrySearchWhereClause("inventory_item", "pump", filters, registrySearchConfig{dictionary: "simple", phraseFriendly: true})
	for _, fragment := range []string{
		"sa.artifact_type = $2",
		"semantic_payload->'item_categories'",
		"semantic_payload->>'manufacturer'",
		"semantic_payload->>'brand'",
		"semantic_payload->>'model_number'",
		"semantic_payload->>'validation_status'",
	} {
		if !regexp.MustCompile(regexp.QuoteMeta(fragment)).MatchString(where) {
			t.Fatalf("where missing %q: %s", fragment, where)
		}
	}
	if len(args) != 7 {
		t.Fatalf("args len=%d, want 7: %#v", len(args), args)
	}
}

func mustParseTestTime(raw string) any {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		panic(err)
	}
	return t
}
