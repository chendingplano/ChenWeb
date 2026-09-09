package classcontractsearch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClassContractSearchMigrationShape(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test file")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(thisFile),
		"../../../../project_migrations/20260908000001_create_kb_ontology_class_contract_search.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compact := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, want := range []string{
		"create table if not exists kb.ontology_class_contract_search",
		"class_term_id text primary key",
		"references kb.ontology_term_headers (term_id)",
		"search_document text",
		"search_vector tsvector",
		"embedding vector(1536)",
		"using gin (search_vector)",
		"using hnsw (embedding vector_cosine_ops)",
		"-- +goose down",
		"drop table if exists kb.ontology_class_contract_search",
	} {
		if !strings.Contains(compact, want) {
			t.Errorf("migration missing %q", want)
		}
	}
}

func TestContractFacets(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    []string
	}{
		{"empty", "", nil},
		{"empty object", "{}", nil},
		{"null", "null", nil},
		{"malformed", "{not json", nil},
		{
			"value_type + one unit",
			`{"value_type":"number","permitted_unit_term_ids":["unit:times_per_day"]}`,
			[]string{"number", "unit:times_per_day"},
		},
		{
			"value_type + several units, blanks skipped",
			`{"value_type":"integer","permitted_unit_term_ids":["unit:a","","unit:b"]}`,
			[]string{"integer", "unit:a", "unit:b"},
		},
		{"no facets present", `{"other":"x"}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := contractFacets(tc.payload)
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("contractFacets(%q) = %v, want %v", tc.payload, got, tc.want)
			}
		})
	}
}

func TestLexemeQuery(t *testing.T) {
	if q := lexemeQuery(""); q != "" {
		t.Fatalf("empty doc -> %q, want empty", q)
	}
	if q := lexemeQuery("!!! ,,, ??"); q != "" {
		t.Fatalf("punctuation-only doc -> %q, want empty", q)
	}
	q := lexemeQuery("Collection Frequency\ncollection frequency\n收运频次")
	// lower-cased, de-duplicated, OR-joined; 1-rune tokens dropped.
	if !strings.Contains(q, "collection") || !strings.Contains(q, "frequency") || !strings.Contains(q, "收运频次") {
		t.Fatalf("lexemeQuery missing expected tokens: %q", q)
	}
	if strings.Count(q, "collection") != 1 {
		t.Fatalf("lexemeQuery did not de-duplicate: %q", q)
	}
	if !strings.Contains(q, " | ") {
		t.Fatalf("lexemeQuery not OR-joined: %q", q)
	}
}

func TestBuildMatchQueryShapes(t *testing.T) {
	// both lists available
	sqlText, args := buildMatchQuery("m:x", "a | b", "[0.1,0.2]", true, true, 20)
	if !strings.Contains(sqlText, "lexical AS") || !strings.Contains(sqlText, "semantic AS") ||
		!strings.Contains(sqlText, "FULL OUTER JOIN semantic") {
		t.Fatalf("both-lists query missing a CTE / join:\n%s", sqlText)
	}
	if len(args) != 4 || args[0] != "m:x" || args[3] != 20 {
		t.Fatalf("both-lists args = %v", args)
	}

	// lexical only
	sqlText, args = buildMatchQuery("m:x", "a | b", "", true, false, 5)
	if strings.Contains(sqlText, "semantic AS") || !strings.Contains(sqlText, "lexical AS") {
		t.Fatalf("lexical-only query wrong shape:\n%s", sqlText)
	}
	if len(args) != 3 || args[2] != 5 {
		t.Fatalf("lexical-only args = %v", args)
	}

	// semantic only
	sqlText, args = buildMatchQuery("m:x", "", "[0.1]", false, true, 7)
	if strings.Contains(sqlText, "lexical AS") || !strings.Contains(sqlText, "semantic AS") {
		t.Fatalf("semantic-only query wrong shape:\n%s", sqlText)
	}
	if len(args) != 3 || args[2] != 7 {
		t.Fatalf("semantic-only args = %v", args)
	}
}

func TestDedupeNonEmpty(t *testing.T) {
	got := dedupeNonEmpty([]string{" a ", "a", "", "b", "b", "  "})
	if strings.Join(got, ",") != "a,b" {
		t.Fatalf("dedupeNonEmpty = %v", got)
	}
}
