package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditTermReadersClassifiesApplicationSQLAndSkipsTests(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"current.go":   "package fixture\nconst current = `SELECT term_id FROM kb.ontology_terms WHERE status = 'active'`\n",
		"history.go":   "package fixture\nconst history = `SELECT term_id, revision FROM kb.ontology_terms ORDER BY revision DESC`\n",
		"writer.go":    "package fixture\nconst writer = `UPDATE kb.ontology_terms SET modify_time = NOW()`\n",
		"skip_test.go": "package fixture\nconst testSQL = `SELECT * FROM kb.ontology_terms`\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := AuditTermReaders(root)
	if err != nil {
		t.Fatalf("AuditTermReaders: %v", err)
	}
	if len(got.References) != 3 {
		t.Fatalf("references = %#v, want 3 application references", got.References)
	}
	if got.CurrentState != 1 || got.Historical != 1 || got.WritePath != 1 {
		t.Fatalf("summary = %+v, want one in each classification", got)
	}
	byClass := make(map[string]TermReaderReference, len(got.References))
	for _, ref := range got.References {
		byClass[ref.Classification] = ref
	}
	if byClass["current_state"].MigrationTarget != "kb.ontology_terms_current" {
		t.Fatalf("current-state target = %+v", byClass["current_state"])
	}
	if byClass["historical"].MigrationTarget != "append-only term revisions" {
		t.Fatalf("historical target = %+v", byClass["historical"])
	}
	if byClass["write_path"].MigrationTarget != "stable term header and revision stores" {
		t.Fatalf("write-path target = %+v", byClass["write_path"])
	}
}

func TestAuditTermReadersIgnoresCommentsAndNonSQLStrings(t *testing.T) {
	root := t.TempDir()
	body := `package fixture
// kb.ontology_terms is documented here but not queried.
const pattern = "kb.ontology_terms"
`
	if err := os.WriteFile(filepath.Join(root, "notes.go"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := AuditTermReaders(root)
	if err != nil {
		t.Fatalf("AuditTermReaders: %v", err)
	}
	if len(got.References) != 0 {
		t.Fatalf("references = %#v, want no SQL readers", got.References)
	}
}
