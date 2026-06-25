package docreviews

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Every core tool must expose valid JSON-Schema parameters and the registry must
// contain exactly the nine document-intrinsic tools.
func TestBuildToolRegistryShapeAndSchemas(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	reg := buildToolRegistry(db)
	if len(reg) != len(coreToolNames) {
		t.Fatalf("registry size=%d, want %d", len(reg), len(coreToolNames))
	}
	for _, name := range coreToolNames {
		tool, ok := reg[name]
		if !ok {
			t.Fatalf("registry missing %q", name)
		}
		if tool.Execute == nil {
			t.Fatalf("%q has nil Execute", name)
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters, &schema); err != nil {
			t.Fatalf("%q parameters not valid JSON: %v", name, err)
		}
		if schema["type"] != "object" {
			t.Fatalf("%q schema type=%v, want object", name, schema["type"])
		}
		if _, ok := schema["properties"]; !ok {
			t.Fatalf("%q schema missing properties", name)
		}
	}
	// No cross-document / P5 tool leaked in.
	for _, forbidden := range []string{"search_reference_docs", "get_reference_roster", "check_entity_in_reference"} {
		if _, ok := reg[forbidden]; ok {
			t.Fatalf("registry must not expose cross-document tool %q", forbidden)
		}
	}
}

func TestSelectToolsSubsetAndAll(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	reg := buildToolRegistry(db)

	all := selectTools(reg, nil)
	if len(all) != len(coreToolNames) {
		t.Fatalf("selectTools(nil) len=%d, want %d", len(all), len(coreToolNames))
	}

	subset := selectTools(reg, []string{"search_entities", "get_metric", "unknown_tool"})
	if len(subset) != 2 {
		t.Fatalf("selectTools subset len=%d, want 2 (unknown skipped)", len(subset))
	}
}

// search_entities must scope its query to the caller's record id.
func TestSearchEntitiesIsRecordScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	reg := buildToolRegistry(db)
	tool := reg["search_entities"]

	rows := sqlmock.NewRows([]string{"entity_id", "entity", "entity_en", "entity_type", "aliases", "desc_text", "line_spans", "confidence"}).
		AddRow("42_ent_1", "Sterilizer", "Sterilizer", "device", []byte(`["autoclave"]`), "desc", []byte(`[]`), 0.9)

	// Expect the query to carry input_record_id = 42 as the first arg.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.entities")).
		WithArgs(int64(42), "sterilizer", 10).
		WillReturnRows(rows)

	res, err := tool.Execute(context.Background(), 42, map[string]any{"query": "sterilizer"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	list, ok := res.([]map[string]any)
	if !ok || len(list) != 1 {
		t.Fatalf("result=%#v, want 1 row", res)
	}
	if list[0]["entity_id"] != "42_ent_1" {
		t.Fatalf("entity_id=%v", list[0]["entity_id"])
	}
	// aliases is JSONB → should be parsed into a slice, not a raw string.
	if _, ok := list[0]["aliases"].([]any); !ok {
		t.Fatalf("aliases not parsed as JSON array: %#v", list[0]["aliases"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSearchEntitiesRequiresQuery(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()
	tool := buildToolRegistry(db)["search_entities"]
	if _, err := tool.Execute(context.Background(), 1, map[string]any{}); err == nil {
		t.Fatalf("expected error when query missing")
	}
}
