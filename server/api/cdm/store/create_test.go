package store_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

// TestCreate_LinksDocumentToInputRow is the regression guard for the dead
// input_record_id column: kb.cdm_documents has had the column and its index
// since the Phase 1 migration, but no Go code wrote or read it, so a document
// had no discoverable link to the kb.inputs row carrying its tenant,
// publication state, and pipeline handoff. An HTTP API starting from a
// document_key in a URL cannot work without it (design D2).
func TestCreate_LinksDocumentToInputRow(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key

	res, err := s.Create(context.Background(), &doc, store.DraftInput{
		TenantID: "tenant-x",
		Title:    doc.Title,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cleanupInput(t, db, res.InputRecordID)

	if res.InputRecordID == 0 {
		t.Fatal("expected a non-zero input record id")
	}

	var linked sql.NullInt64
	if err := db.QueryRow(
		`SELECT input_record_id FROM kb.cdm_documents WHERE document_key = $1`, key,
	).Scan(&linked); err != nil {
		t.Fatalf("read input_record_id: %v", err)
	}
	if !linked.Valid {
		t.Fatal("input_record_id is NULL; the document has no link to its kb.inputs row")
	}
	if linked.Int64 != res.InputRecordID {
		t.Fatalf("input_record_id = %d, want %d", linked.Int64, res.InputRecordID)
	}
}

// TestCreate_DraftIsOffBothWorklists confirms Create writes the kb.inputs row
// in the draft form CDM §10.1 requires, not merely that it writes one.
func TestCreate_DraftIsOffBothWorklists(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	r := store.NewInputRegistrar(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key

	res, err := s.Create(context.Background(), &doc, store.DraftInput{
		TenantID: "tenant-x",
		Title:    doc.Title,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cleanupInput(t, db, res.InputRecordID)

	st, err := r.LoadInputState(context.Background(), res.InputRecordID)
	if err != nil {
		t.Fatalf("load input state: %v", err)
	}
	if st.ParseState != "parsed_success" || st.PipelineState != "success" {
		t.Fatalf("draft should be off both worklists, got parse_state=%q pipeline_state=%q",
			st.ParseState, st.PipelineState)
	}
}

// TestCreate_InvalidDocumentWritesNeitherRow asserts the two writes are one
// transaction. Validation runs before either insert, so an invalid document
// must leave no kb.inputs row orphaned behind it.
func TestCreate_InvalidDocumentWritesNeitherRow(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	var inputsBefore int
	if err := db.QueryRow(
		`SELECT count(*) FROM kb.inputs WHERE type = 'cdm' AND title = $1`, key,
	).Scan(&inputsBefore); err != nil {
		t.Fatalf("count inputs before: %v", err)
	}

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	// Duplicate block id: rejected by model.Validate (spec §1.2).
	doc.Blocks = append(doc.Blocks, model.Block{
		ID:      "intro",
		Type:    "paragraph",
		Content: []model.Inline{{Type: "text", Text: "duplicate"}},
	})

	if _, err := s.Create(context.Background(), &doc, store.DraftInput{
		TenantID: "tenant-x",
		Title:    key,
	}); err == nil {
		t.Fatal("expected create to fail validation")
	}

	var docCount int
	if err := db.QueryRow(
		`SELECT count(*) FROM kb.cdm_documents WHERE document_key = $1`, key,
	).Scan(&docCount); err != nil {
		t.Fatalf("count documents: %v", err)
	}
	if docCount != 0 {
		t.Errorf("expected no document row, found %d", docCount)
	}

	var inputsAfter int
	if err := db.QueryRow(
		`SELECT count(*) FROM kb.inputs WHERE type = 'cdm' AND title = $1`, key,
	).Scan(&inputsAfter); err != nil {
		t.Fatalf("count inputs after: %v", err)
	}
	if inputsAfter != inputsBefore {
		t.Errorf("expected no orphaned kb.inputs row, count went %d -> %d", inputsBefore, inputsAfter)
	}
}
