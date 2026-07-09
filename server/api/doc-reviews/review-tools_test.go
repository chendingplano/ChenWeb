package docreviews

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Every core tool must expose valid JSON-Schema parameters and the registry must
// contain exactly the nine document-intrinsic tools plus the cross-record
// get_artifact_context and get_document_metadata tools, which are not core tools.
func TestBuildToolRegistryShapeAndSchemas(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	reg := buildToolRegistry(db)
	if len(reg) != len(coreToolNames)+2 {
		t.Fatalf("registry size=%d, want %d", len(reg), len(coreToolNames)+2)
	}
	if _, ok := reg["get_artifact_context"]; !ok {
		t.Fatal("registry missing get_artifact_context")
	}
	if _, ok := reg["get_document_metadata"]; !ok {
		t.Fatal("registry missing get_document_metadata")
	}
	for _, name := range coreToolNames {
		if name == "get_artifact_context" || name == "get_document_metadata" {
			t.Fatalf("%s must not be in coreToolNames (selectTools(nil) must stay record-scoped)", name)
		}
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

func TestGetDocumentMetadataToolReturnsCompactMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	reg := buildToolRegistry(db)
	tool := reg["get_document_metadata"]
	if tool.Execute == nil {
		t.Fatal("get_document_metadata missing Execute")
	}

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.inputs")).
		WithArgs(int64(2002)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "title", "doc_no", "staging_filename", "file_name", "doc_metadata",
		}).AddRow(
			int64(2002),
			"Pipeline Design Standard",
			"GB/T 50316-2000",
			"gb50316.pdf",
			"GB_50316_pipe_design.pdf",
			[]byte(`{"doc_no":"GB/T 50316-2000","publish_date":"2000-01-01","implementation_date":"2000-07-01","language":"zh-cn","metadata":{"jurisdiction":"CN"}}`),
		))

	res, err := tool.Execute(context.Background(), 1, map[string]any{"record_id": float64(2002)})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("result=%#v, want map", res)
	}
	if m["found"] != true || m["record_id"] != int64(2002) {
		t.Fatalf("identity = found:%v record_id:%v", m["found"], m["record_id"])
	}
	if m["doc_no"] != "GB/T 50316-2000" || m["authority_class"] != "standard" {
		t.Fatalf("doc_no/authority = %v/%v", m["doc_no"], m["authority_class"])
	}
	if m["publish_date"] != "2000-01-01" || m["implementation_date"] != "2000-07-01" {
		t.Fatalf("dates = %v/%v", m["publish_date"], m["implementation_date"])
	}
	if meta, ok := m["doc_metadata"].(map[string]any); !ok || meta["language"] != "zh-cn" {
		t.Fatalf("doc_metadata = %#v", m["doc_metadata"])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
