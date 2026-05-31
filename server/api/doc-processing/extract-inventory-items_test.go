package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestLoadInventoryDictionaryDirAndValidateItem(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "category_schemas.json"), []byte(`{
		"version": "dict-v1",
		"categories": {
			"pump": {
				"required_attrs": ["manufacturer", "power"],
				"specs": {
					"power": {"canonical_unit": "W", "aliases": ["wattage"]}
				}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write category schemas: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "units.json"), []byte(`{
		"version": "units-v1",
		"units": {
			"kw": {"canonical": "W", "factor": 1000},
			"w": {"canonical": "W", "factor": 1}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write units: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aliases.json"), []byte(`{
		"version": "aliases-v1",
		"aliases": {"pump": ["pmp"]}
	}`), 0o644); err != nil {
		t.Fatalf("write aliases: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "standards.json"), []byte(`{
		"version": "standards-v1",
		"standards": {"pump": ["ISO 9001"]}
	}`), 0o644); err != nil {
		t.Fatalf("write standards: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plausible_ranges.json"), []byte(`{
		"version": "ranges-v1",
		"ranges": {
			"pump": {
				"power": {"min": 1, "max": 2000, "unit": "W"}
			}
		}
	}`), 0o644); err != nil {
		t.Fatalf("write plausible ranges: %v", err)
	}

	dict, err := loadInventoryDictionaryDir(dir)
	if err != nil {
		t.Fatalf("loadInventoryDictionaryDir: %v", err)
	}
	if dict.Version != "dict-v1" {
		t.Fatalf("Version=%q, want dict-v1", dict.Version)
	}
	if dict.Aliases["pmp"] != "pump" {
		t.Fatalf("alias pmp=%q, want pump", dict.Aliases["pmp"])
	}
	if len(dict.Standards["pump"]) != 1 || dict.Standards["pump"][0] != "ISO 9001" {
		t.Fatalf("standards=%#v, want ISO 9001", dict.Standards["pump"])
	}

	row := map[string]any{
		"item_name":     "Bosch Pump 1.5kW",
		"item_category": "pump",
		"manufacturer":  "Bosch",
		"raw_specs": []any{
			map[string]any{"name": "wattage", "value": 1.5, "unit": "kw"},
		},
		"lines":      []any{"10"},
		"confidence": 0.91,
	}
	normalized := normalizeInventoryItemRows([]any{row}, 2, dict)
	if len(normalized) != 1 {
		t.Fatalf("normalized len=%d, want 1: %#v", len(normalized), normalized)
	}
	gotSpecs := normalized[0]["normalized_specs"].([]map[string]any)
	if len(gotSpecs) != 1 {
		t.Fatalf("normalized_specs len=%d, want 1", len(gotSpecs))
	}
	if gotSpecs[0]["name"] != "power" || gotSpecs[0]["value"] != float64(1500) || gotSpecs[0]["unit"] != "W" {
		t.Fatalf("normalized spec=%#v, want power=1500 W", gotSpecs[0])
	}
	if flags := normalized[0]["validation_flags"].([]string); len(flags) != 0 {
		t.Fatalf("validation_flags=%#v, want empty", flags)
	}
	if key := normalized[0]["dedupe_key"]; key != "pump|bosch|||power=1500W" {
		t.Fatalf("dedupe_key=%q, want pump|bosch|||power=1500W", key)
	}

	row["raw_specs"] = []any{map[string]any{"name": "power", "value": 3, "unit": "kw"}}
	outOfRange := normalizeInventoryItemRows([]any{row}, 2, dict)
	if flags := outOfRange[0]["validation_flags"].([]string); !reflect.DeepEqual(flags, []string{"implausible_power"}) {
		t.Fatalf("validation_flags=%#v, want implausible_power", flags)
	}
}

func TestNormalizeInventoryItemRowsMarksMissingRequiredAttrs(t *testing.T) {
	dict := inventoryDictionary{
		Version: "dict-v1",
		Categories: map[string]inventoryCategorySchema{
			"bearing": {RequiredAttrs: []string{"manufacturer", "inner_diameter"}},
		},
	}
	rows := normalizeInventoryItemRows([]any{
		map[string]any{
			"item_name":     "Bearing 10",
			"item_category": "bearing",
			"lines":         []any{"42"},
			"confidence":    0.52,
		},
	}, 7, dict)
	if len(rows) != 1 {
		t.Fatalf("rows len=%d, want 1", len(rows))
	}
	if got := rows[0]["missing_required_attrs"]; !reflect.DeepEqual(got, []string{"manufacturer", "inner_diameter"}) {
		t.Fatalf("missing_required_attrs=%#v", got)
	}
	if got := rows[0]["validation_flags"]; !reflect.DeepEqual(got, []string{"missing_required_attrs", "low_confidence"}) {
		t.Fatalf("validation_flags=%#v", got)
	}
}

func TestInventoryItemsExtractionContractShape(t *testing.T) {
	contract := inventoryItemsExtractionContract()
	if contract.Name != "chenweb_inventory_items_extraction" {
		t.Fatalf("contract name=%q", contract.Name)
	}
	var schema map[string]any
	if err := json.Unmarshal(contract.Schema, &schema); err != nil {
		t.Fatalf("schema unmarshal: %v", err)
	}
	props, _ := schema["properties"].(map[string]any)
	for _, key := range []string{"language", "items"} {
		if _, ok := props[key]; !ok {
			t.Fatalf("schema missing %q: %#v", key, props)
		}
	}
}

func TestAppendInventoryItemsStatusReplacesExisting(t *testing.T) {
	existing := `[{"operation":"extract_inventory_items","proc_status":"failed","error":"old"}]`
	out, err := appendInventoryItemsStatus(existing, inventoryItemsStatusParams{
		RecordID:   42,
		FileType:   "pdf",
		Start:      time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC),
		DurationMs: 88,
	})
	if err != nil {
		t.Fatalf("appendInventoryItemsStatus: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("len=%d, want 1", len(arr))
	}
	if arr[0]["operation"] != "extract_inventory_items" || arr[0]["proc_status"] != "success" {
		t.Fatalf("status row=%#v", arr[0])
	}
	if _, ok := arr[0]["error"]; ok {
		t.Fatalf("stale error should be removed: %#v", arr[0])
	}
}

func TestWriteInventoryItemsArtifactFile(t *testing.T) {
	tmp := t.TempDir()
	rec := DocMetadataInputRecord{StagingFilename: "manual.pdf", ParserName: "opendata"}
	rows := []map[string]any{{"inventory_item_id": "100_i_1", "item_name": "Bosch Pump"}}
	if err := writeInventoryItemsArtifactFile(tmp, 100, rec, rows); err != nil {
		t.Fatalf("writeInventoryItemsArtifactFile: %v", err)
	}
	path := filepath.Join(tmp, "0", "100", "manual_opendata.inventory_items")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
}

func TestInventoryItemsProcessorUsesStructuredContractAndNormalizes(t *testing.T) {
	fake := &fakeJSONExtractor{out: map[string]any{
		"language": "en",
		"items": []any{
			map[string]any{
				"item_name":      "Pump, Bosch, 1500w",
				"canonical_name": "Bosch pump",
				"item_category":  "pump",
				"manufacturer":   "Bosch",
				"raw_specs":      []any{map[string]any{"name": "power", "value": 1500, "unit": "w"}},
				"evidence_quote": "Pump, Bosch, 1500w",
				"lines":          []any{"5"},
				"confidence":     0.9,
			},
		},
	}}
	p := &InventoryItemsProcessor{
		Logger:     loggerutil.CreateDefaultLogger("TEST_INV_ITEMS"),
		Extractor:  fake,
		ModelName:  "model-a",
		ModelCfg:   structureModelConfig{ModelName: "model-a"},
		PromptText: "extract inventory items",
		PromptRef:  "prompt-extract-inventory-items-v1.md",
		Dictionary: inventoryDictionary{Version: "dict-v1"},
		Now:        time.Now,
	}
	result, err := p.extractInventoryItemsFromChunks(context.Background(), 88, []Chunk{{
		SeqNo: 1,
		Lines: []MarkedLine{{
			Mark: "r",
			Line: Line{LineNo: 5, PageNo: 1, LineType: "text", Content: "Pump, Bosch, 1500w"},
		}},
	}})
	if err != nil {
		t.Fatalf("extractInventoryItemsFromChunks: %v", err)
	}
	if fake.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", fake.structuredCalledCount)
	}
	if got := fake.contractNames[0]; got != "chenweb_inventory_items_extraction" {
		t.Fatalf("contract=%q", got)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items len=%d", len(result.Items))
	}
	if result.Items[0]["dedupe_key"] == "" {
		t.Fatalf("dedupe_key should be populated: %#v", result.Items[0])
	}
}

func TestInventoryItemsSQLStoreSaveExistAndDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	store := InventoryItemsSQLStore{DB: db}

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO kb.inventory_items").
		WillReturnResult(sqlmock.NewResult(1, 1))
	inserted, err := store.SaveInventoryItems(context.Background(), SaveInventoryItemsRequest{
		InputRecordID: 7,
		EventID:       "evt-1",
		Language:      "en",
		ModelName:     "gpt-test",
		PromptName:    "prompt-extract-inventory-items-v1.md",
		Items: []map[string]any{{
			"inventory_item_id":      "7_i_1",
			"item_name":              "Bosch Pump",
			"canonical_name":         "Bosch Pump",
			"item_category":          "pump",
			"manufacturer":           "Bosch",
			"normalized_specs":       []map[string]any{{"name": "power", "value": 1500, "unit": "W"}},
			"raw_specs":              []map[string]any{{"name": "power", "value": 1500, "unit": "w"}},
			"standards":              []string{"ISO 9001"},
			"aliases":                []string{"Pmp"},
			"source_line_spans":      []string{"5"},
			"validation_flags":       []string{},
			"missing_required_attrs": []string{},
			"dedupe_key":             "pump|bosch|||power=1500W",
			"schema_version":         inventoryItemsSchemaVersion,
			"dictionary_version":     "dict-v1",
			"confidence":             0.9,
			"confidence_reason":      "explicit",
		}},
	})
	if err != nil {
		t.Fatalf("SaveInventoryItems: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted=%d, want 1", inserted)
	}

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT 1 FROM kb.inventory_items").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"one"}).AddRow(1))
	exists, err := store.InventoryItemsExist(context.Background(), 7)
	if err != nil {
		t.Fatalf("InventoryItemsExist: %v", err)
	}
	if !exists {
		t.Fatalf("exists=false, want true")
	}

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM kb.inventory_items WHERE input_record_id =").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	deleted, err := store.DeleteInventoryItemsByInputRecordID(context.Background(), 7)
	if err != nil {
		t.Fatalf("DeleteInventoryItemsByInputRecordID: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
