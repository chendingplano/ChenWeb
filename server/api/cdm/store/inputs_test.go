package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

func cleanupInput(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM kb.inputs WHERE id = $1`, id); err != nil {
			t.Logf("cleanup: delete input %d: %v", id, err)
		}
	})
}

// parseWorklistCount mirrors the literal WHERE fragment from
// server/api/kbhandler/handler.go:679 (parse_state = 'pending').
func parseWorklistCount(t *testing.T, db *sql.DB, inputID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.inputs WHERE id = $1 AND parse_state = 'pending'
	`, inputID).Scan(&n); err != nil {
		t.Fatalf("parse worklist query: %v", err)
	}
	return n
}

// docProcessingWorklistCount mirrors the literal WHERE fragment from
// server/api/kbhandler/handler.go:748 ("parsed_not_started" filter:
// parse_state = 'parsed_success' AND pipeline_state = 'pending').
func docProcessingWorklistCount(t *testing.T, db *sql.DB, inputID int64) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`
		SELECT count(*) FROM kb.inputs
		WHERE id = $1 AND parse_state = 'parsed_success' AND pipeline_state = 'pending'
	`, inputID).Scan(&n); err != nil {
		t.Fatalf("doc-processing worklist query: %v", err)
	}
	return n
}

func TestCreateDraft_DerivedStatesAreTerminal(t *testing.T) {
	db := testDB(t)
	r := store.NewInputRegistrar(db)

	id, err := r.CreateDraft(context.Background(), store.DraftInput{
		TenantID: "tenant-x",
		Title:    "Draft Doc",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	cleanupInput(t, db, id)

	st, err := r.LoadInputState(context.Background(), id)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.ParseState != "parsed_success" {
		t.Errorf("expected parse_state parsed_success, got %q", st.ParseState)
	}
	if st.PipelineState != "success" {
		t.Errorf("expected pipeline_state success, got %q", st.PipelineState)
	}
}

func TestCreateDraft_InvisibleToBothWorklists(t *testing.T) {
	db := testDB(t)
	r := store.NewInputRegistrar(db)

	id, err := r.CreateDraft(context.Background(), store.DraftInput{TenantID: "tenant-x"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	cleanupInput(t, db, id)

	if n := parseWorklistCount(t, db, id); n != 0 {
		t.Errorf("expected draft absent from parse worklist, found %d", n)
	}
	if n := docProcessingWorklistCount(t, db, id); n != 0 {
		t.Errorf("expected draft absent from doc-processing worklist, found %d", n)
	}
}

func TestPublish_EnqueuesForDocProcessingOnly(t *testing.T) {
	db := testDB(t)
	r := store.NewInputRegistrar(db)

	id, err := r.CreateDraft(context.Background(), store.DraftInput{TenantID: "tenant-x"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	cleanupInput(t, db, id)

	if err := r.Publish(context.Background(), id); err != nil {
		t.Fatalf("publish: %v", err)
	}

	st, err := r.LoadInputState(context.Background(), id)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if st.ParseState != "parsed_success" {
		t.Errorf("expected parse_state to remain parsed_success, got %q", st.ParseState)
	}
	if st.PipelineState != "pending" {
		t.Errorf("expected pipeline_state pending after publish, got %q", st.PipelineState)
	}

	if n := parseWorklistCount(t, db, id); n != 0 {
		t.Errorf("expected published doc still absent from parse worklist, found %d", n)
	}
	if n := docProcessingWorklistCount(t, db, id); n != 1 {
		t.Errorf("expected published doc on the doc-processing worklist, found %d", n)
	}
}

func TestPublish_NonCDMRowIsNotTouched(t *testing.T) {
	db := testDB(t)
	r := store.NewInputRegistrar(db)

	var id int64
	if err := db.QueryRow(`
		INSERT INTO kb.inputs (tenant_id, type, title, status)
		VALUES ('tenant-x', 'pdf', 'Uploaded Doc', '[]'::jsonb)
		RETURNING id
	`).Scan(&id); err != nil {
		t.Fatalf("insert uploaded row: %v", err)
	}
	cleanupInput(t, db, id)

	if err := r.Publish(context.Background(), id); err == nil {
		t.Fatal("expected Publish to refuse a non-cdm input row")
	}
}
