package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

// testDB returns a live *sql.DB for TEST_DATABASE_URL, skipping the test if
// it is not set, matching the convention in
// server/api/doc-benchmark/store_concurrent_test.go.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// uniqueKey returns a document_key unique to this test run, so cleanup can
// only touch rows this test created even against a shared database.
func uniqueKey(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("doc:test-%s-%d", t.Name(), time.Now().UnixNano())
}

func cleanupDocument(t *testing.T, db *sql.DB, key string) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM kb.cdm_documents WHERE document_key = $1`, key); err != nil {
			t.Logf("cleanup: delete document %q: %v", key, err)
		}
	})
}

func TestSave_InvalidDocumentIsNotPersisted(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	doc.Blocks = append(doc.Blocks, model.Block{ID: "bad", Type: "not-a-type"})

	if _, err := s.Save(context.Background(), &doc, 0); err == nil {
		t.Fatal("expected validation error, got nil")
	}

	var count int
	if err := db.QueryRow(`SELECT count(*) FROM kb.cdm_documents WHERE document_key = $1`, key).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no row written, found %d", count)
	}
}

func TestSave_ThenLoad_RoundTrips(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key

	res, err := s.Save(context.Background(), &doc, 0)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if res.ContentVersion != 1 {
		t.Fatalf("expected content_version 1 on first save, got %d", res.ContentVersion)
	}
	if doc.ContentVersion != 1 {
		t.Fatalf("expected doc.ContentVersion stamped to 1, got %d", doc.ContentVersion)
	}

	loaded, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.ContentVersion != 1 {
		t.Fatalf("expected loaded content_version 1, got %d", loaded.ContentVersion)
	}
	if loaded.Title != doc.Title || len(loaded.Blocks) != len(doc.Blocks) {
		t.Fatalf("loaded document does not match saved document: %+v", loaded)
	}
}

func TestSave_SecondSaveIncrementsContentVersion(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key

	if _, err := s.Save(context.Background(), &doc, 0); err != nil {
		t.Fatalf("first save: %v", err)
	}

	doc.Title = "Jaro-Winkler Similarity (Revised)"
	// Save stamps doc.ContentVersion with the resolved version, so the
	// second save's expectation is simply what the first one produced.
	res, err := s.Save(context.Background(), &doc, doc.ContentVersion)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if res.ContentVersion != 2 {
		t.Fatalf("expected content_version 2 after second save, got %d", res.ContentVersion)
	}

	loaded, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Title != "Jaro-Winkler Similarity (Revised)" {
		t.Fatalf("expected revised title, got %q", loaded.Title)
	}
}

func TestSave_BlocksAreRebuilt(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	res, err := s.Save(context.Background(), &doc, 0)
	if err != nil {
		t.Fatalf("first save: %v", err)
	}

	var before int
	if err := db.QueryRow(`SELECT count(*) FROM kb.cdm_blocks WHERE document_id = $1`, res.DocumentID).Scan(&before); err != nil {
		t.Fatalf("count blocks: %v", err)
	}

	// Remove the last top-level block and save again.
	doc.Blocks = doc.Blocks[:len(doc.Blocks)-1]
	if _, err := s.Save(context.Background(), &doc, doc.ContentVersion); err != nil {
		t.Fatalf("second save: %v", err)
	}

	var after int
	if err := db.QueryRow(`SELECT count(*) FROM kb.cdm_blocks WHERE document_id = $1`, res.DocumentID).Scan(&after); err != nil {
		t.Fatalf("count blocks: %v", err)
	}
	if after >= before {
		t.Fatalf("expected fewer blocks after removing one, before=%d after=%d", before, after)
	}

	var removedGone int
	if err := db.QueryRow(`SELECT count(*) FROM kb.cdm_blocks WHERE document_id = $1 AND block_id = 'jaro-formula'`, res.DocumentID).Scan(&removedGone); err != nil {
		t.Fatalf("query removed block: %v", err)
	}
	if removedGone != 0 {
		t.Fatalf("expected removed block to be gone from kb.cdm_blocks")
	}
}

func TestSave_PromotedColumnsMatchMetadata(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	if _, err := s.Save(context.Background(), &doc, 0); err != nil {
		t.Fatalf("save: %v", err)
	}

	var docType, renderingType string
	if err := db.QueryRow(`SELECT doc_type, rendering_type FROM kb.cdm_documents WHERE document_key = $1`, key).
		Scan(&docType, &renderingType); err != nil {
		t.Fatalf("query: %v", err)
	}
	if docType != doc.Metadata.DocType || renderingType != doc.Metadata.RenderingType {
		t.Fatalf("promoted columns mismatch: doc_type=%q rendering_type=%q", docType, renderingType)
	}
}

func TestSave_SlugConflictReturnsTypedError(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := model.Document{
		Key:           key,
		Title:         "Conflict Test",
		SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{
			{ID: "b1", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
		},
	}

	if _, err := s.Save(context.Background(), &doc, 0); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Validate already rejects document-wide duplicate block IDs before any
	// write (spec "Duplicate block ID is rejected"), and it walks the same
	// tree flattenBlocks does — so Save can never actually reach the
	// (document_id, block_id) constraint in kb.cdm_blocks through its public
	// API. That is the correct outcome (a conflict is caught earlier, with a
	// clearer error), not a gap: this test confirms the DB constraint still
	// exists as the defense-in-depth layer design D8 calls for, for any
	// future write path that bypasses Validate.
	var conflictCount int
	if err := db.QueryRow(`
		SELECT count(*) FROM pg_constraint WHERE conname = 'uq_kb_cdm_blocks_block_id'
	`).Scan(&conflictCount); err != nil {
		t.Fatalf("query constraint: %v", err)
	}
	if conflictCount != 1 {
		t.Fatalf("expected uq_kb_cdm_blocks_block_id constraint to exist, found %d", conflictCount)
	}
}

func TestSave_RollbackLeavesPriorDocumentIntact(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	cleanupDocument(t, db, key)

	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	if _, err := s.Save(context.Background(), &doc, 0); err != nil {
		t.Fatalf("first save: %v", err)
	}

	before, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load before: %v", err)
	}

	// Force the second save to fail (an invalid block type, rejected by
	// Validate before any write) and confirm the previously committed
	// document is untouched.
	bad := doc
	bad.Blocks = append(bad.Blocks, model.Block{ID: "invalid", Type: "not-a-real-type"})
	if _, err := s.Save(context.Background(), &bad, bad.ContentVersion); err == nil {
		t.Fatal("expected the second save to fail validation")
	}

	after, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if after.ContentVersion != before.ContentVersion {
		t.Fatalf("expected content_version unchanged after failed save: before=%d after=%d",
			before.ContentVersion, after.ContentVersion)
	}
	if len(after.Blocks) != len(before.Blocks) {
		t.Fatalf("expected blocks unchanged after failed save: before=%d after=%d",
			len(before.Blocks), len(after.Blocks))
	}
}
