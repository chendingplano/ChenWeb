package docprocessing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type fakeInventoryItemsStore struct {
	exists       bool
	existsErr    error
	deleteCalled int
	existCalled  int
}

func (f *fakeInventoryItemsStore) InventoryItemsExist(_ context.Context, _ int64) (bool, error) {
	f.existCalled++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	return f.exists, nil
}

func (f *fakeInventoryItemsStore) DeleteInventoryItemsByInputRecordID(_ context.Context, _ int64) (int64, error) {
	f.deleteCalled++
	return 0, nil
}

func (f *fakeInventoryItemsStore) DeleteInventoryItemDuplicatesByInputRecordID(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (f *fakeInventoryItemsStore) SaveInventoryItems(_ context.Context, _ SaveInventoryItemsRequest) (int64, error) {
	return 0, nil
}

func (f *fakeInventoryItemsStore) SaveInventoryItemDuplicates(_ context.Context, _ SaveInventoryItemDuplicatesRequest) (int64, error) {
	return 0, nil
}

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
		"item_name":       "Bosch Pump 1.5kW",
		"item_categories": []any{"pump"},
		"manufacturer":    "Bosch",
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
	if key := normalized[0]["dedupe_key"]; key != "pump|boschpump15kw|bosch|||power=1500W" {
		t.Fatalf("dedupe_key=%q, want pump|boschpump15kw|bosch|||power=1500W", key)
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
			"item_name":       "Bearing 10",
			"item_categories": []any{"bearing"},
			"lines":           []any{"42"},
			"confidence":      0.52,
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

func TestNormalizeInventoryItemRowsPreservesObjects(t *testing.T) {
	dict := inventoryDictionary{Version: "dict-v1", Categories: map[string]inventoryCategorySchema{"pump": {}}}
	rows := normalizeInventoryItemRows([]any{
		map[string]any{
			"item_name":       "Feed pump",
			"canonical_name":  "Feed pump",
			"item_categories": []any{"pump"},
			"lines":           []any{"42"},
			"confidence":      0.91,
			"objects": []any{map[string]any{
				"object_name":       "Boiler feed system",
				"object_role":       "parent_system",
				"object_type":       "system",
				"source_line_spans": []any{"42"},
				"confidence":        0.8,
			}},
		},
	}, 7, dict)
	if len(rows) != 1 {
		t.Fatalf("rows len=%d, want 1", len(rows))
	}
	objects := objectItemsFromValue(rows[0]["objects"])
	if len(objects) != 1 || objects[0]["object_name"] != "Boiler feed system" {
		t.Fatalf("objects=%#v", rows[0]["objects"])
	}
}

func TestDedupeKeepsDistinctSpeclessItemsInSameCategory(t *testing.T) {
	dict := inventoryDictionary{Version: "dict-v1", Categories: map[string]inventoryCategorySchema{"material": {}}}
	rows := normalizeInventoryItemRows([]any{
		map[string]any{"item_name": "氢氧化钙", "item_categories": []any{"material"}, "lines": []any{"590"}, "confidence": 0.95},
		map[string]any{"item_name": "氯化钠", "item_categories": []any{"material"}, "lines": []any{"591"}, "confidence": 0.80},
	}, 1, dict)
	if len(rows) != 2 {
		t.Fatalf("normalized rows=%d, want 2", len(rows))
	}
	if rows[0]["dedupe_key"] == rows[1]["dedupe_key"] {
		t.Fatalf("distinct materials share dedupe_key %q — would collapse", rows[0]["dedupe_key"])
	}
	survivors, dupes := dedupeInventoryItemRows(rows)
	if len(survivors) != 2 {
		t.Fatalf("dedupe collapsed distinct items: got %d survivors, want 2", len(survivors))
	}
	if len(dupes) != 0 {
		t.Fatalf("dedupe produced %d duplicates, want 0", len(dupes))
	}
}

func TestDedupeCollapsesIdenticalItems(t *testing.T) {
	dict := inventoryDictionary{Version: "dict-v1", Categories: map[string]inventoryCategorySchema{"material": {}}}
	rows := normalizeInventoryItemRows([]any{
		map[string]any{"item_name": "氢氧化钙", "item_categories": []any{"material"}, "lines": []any{"590"}, "confidence": 0.80, "aliases": []any{"消石灰"}},
		map[string]any{"item_name": "氢氧化钙", "item_categories": []any{"material"}, "lines": []any{"596"}, "confidence": 0.95, "aliases": []any{"Ca(OH)2"}},
	}, 1, dict)
	survivors, dupes := dedupeInventoryItemRows(rows)
	if len(survivors) != 1 {
		t.Fatalf("identical items not collapsed: got %d survivors, want 1", len(survivors))
	}
	if len(dupes) != 1 {
		t.Fatalf("expected 1 discarded duplicate, got %d", len(dupes))
	}
	survivor := survivors[0]
	if got := toFloat(survivor["confidence"]); got != 0.95 {
		t.Fatalf("kept confidence=%v, want 0.95 (higher-confidence wins)", got)
	}
	// Provenance from both mentions must be merged into the survivor.
	if got := survivor["mention_count"]; got != 2 {
		t.Fatalf("mention_count=%v, want 2", got)
	}
	spans := toStringList(survivor["source_line_spans"])
	if !reflect.DeepEqual(spans, []string{"590", "596"}) {
		t.Fatalf("merged source_line_spans=%#v, want [590 596]", spans)
	}
	aliases := toStringList(survivor["aliases"])
	if !reflect.DeepEqual(aliases, []string{"Ca(OH)2", "消石灰"}) {
		t.Fatalf("merged aliases=%#v, want [Ca(OH)2 消石灰]", aliases)
	}
	// The discarded duplicate keeps its own original confidence for audit.
	if got := toFloat(dupes[0]["confidence"]); got != 0.80 {
		t.Fatalf("duplicate confidence=%v, want 0.80 (original retained)", got)
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

func TestRefreshInventoryItemsArtifactFileIncludesConnectedArtifacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta(`
SELECT inventory_item_id, item_name, canonical_name, item_categories, manufacturer, brand,
       model_number, part_number, normalized_specs, raw_specs, standards, aliases,
       evidence_quote, source_line_spans, validation_flags, missing_required_attrs,
       dedupe_key, schema_version, dictionary_version, confidence, confidence_reason,
       kb.connected_artifacts(input_record_id, 'inventory_item', id), ext_info
FROM kb.inventory_items
WHERE input_record_id = $1
ORDER BY id`)).
		WithArgs(int64(100)).
		WillReturnRows(sqlmock.NewRows([]string{
			"inventory_item_id", "item_name", "canonical_name", "item_categories", "manufacturer", "brand",
			"model_number", "part_number", "normalized_specs", "raw_specs", "standards", "aliases",
			"evidence_quote", "source_line_spans", "validation_flags", "missing_required_attrs",
			"dedupe_key", "schema_version", "dictionary_version", "confidence", "confidence_reason",
			"connected_artifacts", "ext_info",
		}).AddRow(
			"100_i_1",
			"Bosch Pump",
			"Bosch Pump",
			[]byte(`["pump"]`),
			"Bosch",
			"",
			"",
			"",
			[]byte(`[{"name":"power","value":1500,"unit":"W"}]`),
			[]byte(`[{"name":"power","value":1500,"unit":"w"}]`),
			[]byte(`["ISO 9001"]`),
			[]byte(`["Pmp"]`),
			"Pump, Bosch, 1500w",
			[]byte(`["5"]`),
			[]byte(`[]`),
			[]byte(`[]`),
			"pump|bosch|||power=1500W",
			inventoryItemsSchemaVersion,
			"dict-v1",
			0.9,
			"explicit",
			[]byte(`{"chunks":["100_ch_1"],"semantic_projects":["100_prj_1"],"topics":[],"scenes":[],"provisions":[],"entities":[],"metrics":[]}`),
			[]byte(`{"chunk_seq_no":1,"mention_count":2}`),
		))

	tmp := t.TempDir()
	rec := DocMetadataInputRecord{StagingFilename: "manual.pdf", ParserName: "opendata"}
	if err := refreshInventoryItemsArtifactFile(context.Background(), db, tmp, 100, rec); err != nil {
		t.Fatalf("refreshInventoryItemsArtifactFile: %v", err)
	}

	path := filepath.Join(tmp, "0", "100", "manual_opendata.inventory_items")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		t.Fatalf("unmarshal artifact: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("artifact rows len=%d, want 1", len(rows))
	}
	got, ok := rows[0]["connected_artifacts"].(map[string]any)
	if !ok {
		t.Fatalf("connected_artifacts missing or wrong type: %#v", rows[0]["connected_artifacts"])
	}
	if !reflect.DeepEqual(got["chunks"], []any{"100_ch_1"}) {
		t.Fatalf("connected_artifacts.chunks=%#v, want [100_ch_1]", got["chunks"])
	}
	if rows[0]["mention_count"] != float64(2) {
		t.Fatalf("mention_count=%v, want 2", rows[0]["mention_count"])
	}
	if rows[0]["chunk_seq_no"] != float64(1) {
		t.Fatalf("chunk_seq_no=%v, want 1", rows[0]["chunk_seq_no"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInventoryItemsProcessorUsesStructuredContractAndNormalizes(t *testing.T) {
	fake := &fakeJSONExtractor{out: map[string]any{
		"language": "en",
		"items": []any{
			map[string]any{
				"item_name":       "Pump, Bosch, 1500w",
				"canonical_name":  "Bosch pump",
				"item_categories": []any{"pump"},
				"manufacturer":    "Bosch",
				"raw_specs":       []any{map[string]any{"name": "power", "value": 1500, "unit": "w"}},
				"evidence_quote":  "Pump, Bosch, 1500w",
				"lines":           []any{"5"},
				"confidence":      0.9,
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
	}}, "")
	if err != nil {
		t.Fatalf("extractInventoryItemsFromChunks: %v", err)
	}
	if fake.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", fake.structuredCalledCount)
	}
	if got := fake.contractNames[0]; got != "chenweb_inventory_items_extraction" {
		t.Fatalf("contract=%q", got)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-extract-inventory-items-v1.md" {
		t.Fatalf("promptNames=%v, want [prompt-extract-inventory-items-v1.md]", fake.promptNames)
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
			"item_categories":        []string{"pump"},
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

func TestInventoryItemDuplicatesSQLStoreSaveAndDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	store := InventoryItemsSQLStore{DB: db}

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO kb.inventory_item_duplicates").
		WithArgs(
			"evt-1", int64(7), "7_d_1", "7_i_1", "en",
			"氢氧化钙", "氢氧化钙", `["material"]`, "", "", "", "",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "用 Ca(OH)2",
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), "material|氢氧化钙||||",
			inventoryItemsSchemaVersion, "dict-v1", 0.80, "lower conf", "gpt-test",
			"prompt-extract-inventory-items-v1.md", sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))
	inserted, err := store.SaveInventoryItemDuplicates(context.Background(), SaveInventoryItemDuplicatesRequest{
		InputRecordID: 7,
		EventID:       "evt-1",
		Language:      "en",
		ModelName:     "gpt-test",
		PromptName:    "prompt-extract-inventory-items-v1.md",
		Items: []map[string]any{{
			"inventory_item_id":  "7_d_1",
			"duplicate_of":       "7_i_1",
			"item_name":          "氢氧化钙",
			"canonical_name":     "氢氧化钙",
			"item_categories":    []string{"material"},
			"evidence_quote":     "用 Ca(OH)2",
			"dedupe_key":         "material|氢氧化钙||||",
			"schema_version":     inventoryItemsSchemaVersion,
			"dictionary_version": "dict-v1",
			"confidence":         0.80,
			"confidence_reason":  "lower conf",
		}},
	})
	if err != nil {
		t.Fatalf("SaveInventoryItemDuplicates: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("inserted=%d, want 1", inserted)
	}

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("DELETE FROM kb.inventory_item_duplicates WHERE input_record_id =").
		WithArgs(int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 2))
	deleted, err := store.DeleteInventoryItemDuplicatesByInputRecordID(context.Background(), 7)
	if err != nil {
		t.Fatalf("DeleteInventoryItemDuplicatesByInputRecordID: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted=%d, want 2", deleted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestInventoryItemsProcessor_InitChunkBatch_DeletesExistingWhenForced(t *testing.T) {
	store := &fakeInventoryItemsStore{}
	p := &InventoryItemsProcessor{
		Store:      store,
		Logger:     loggerutil.CreateDefaultLogger("TEST_INV"),
		PromptText: "test prompt",
		PromptRef:  "test-prompt.md",
		Now:        time.Now,
	}
	ctx := withDocProcessorFlags(context.Background(), true, true)
	if err := p.InitChunkBatch(ctx, 173, nil, ""); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	if store.deleteCalled != 1 {
		t.Fatalf("deleteCalled=%d, want 1", store.deleteCalled)
	}
}

func TestInventoryItemsProcessor_InitChunkBatch_SkipsWhenExistsAndNotForced(t *testing.T) {
	store := &fakeInventoryItemsStore{exists: true}
	p := &InventoryItemsProcessor{
		Store:      store,
		Logger:     loggerutil.CreateDefaultLogger("TEST_INV"),
		PromptText: "test prompt",
		PromptRef:  "test-prompt.md",
		Now:        time.Now,
	}
	ctx := withDocProcessorFlags(context.Background(), false, false)
	if err := p.InitChunkBatch(ctx, 173, nil, ""); err != nil {
		t.Fatalf("InitChunkBatch: %v", err)
	}
	if !p.batchSkip {
		t.Fatalf("expected batchSkip=true")
	}
	if store.existCalled != 1 {
		t.Fatalf("existCalled=%d, want 1", store.existCalled)
	}
}
