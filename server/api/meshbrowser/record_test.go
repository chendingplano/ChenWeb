package meshbrowser

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
)

func TestRecordImportedRelease(t *testing.T) {
	dsn := os.Getenv("MESH_TEST_DSN")
	if dsn == "" {
		t.Skip("set MESH_TEST_DSN to check an imported release")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := Store{DB: db}
	r, err := s.Record(context.Background(), "D000229")
	if err != nil {
		t.Fatal(err)
	}
	if r.Entity.Label != "Dependovirus" || len(r.Concepts) != 2 || len(r.Qualifiers) == 0 || len(r.Trees) != 1 {
		t.Fatalf("incomplete record: %+v", r)
	}
	if len(r.Trees[0].Path) != 6 || r.Trees[0].Path[5].RecordID != "D000229" {
		t.Fatalf("incomplete hierarchy: %+v", r.Trees[0])
	}
	preferred := false
	for _, c := range r.Concepts {
		if c.Preferred {
			preferred = c.Entity.ID == "M0000354" && len(c.Terms) == 2
		}
	}
	if !preferred {
		t.Fatal("missing preferred concept/terms")
	}
	if _, err := s.Record(context.Background(), "D9999999999999"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing record: %v", err)
	}
	leaf, err := s.Record(context.Background(), "T000686")
	if err != nil || len(leaf.Concepts) != 0 || len(leaf.Trees) != 0 || len(leaf.Qualifiers) != 0 {
		t.Fatalf("term record: %+v, %v", leaf, err)
	}
	concept, err := s.Record(context.Background(), "M0000354")
	if err != nil || len(concept.Concepts) != 1 || len(concept.Concepts[0].Terms) != 2 {
		t.Fatalf("direct concept record: %+v, %v", concept, err)
	}
}
