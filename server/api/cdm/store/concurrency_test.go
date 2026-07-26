package store_test

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/store"
)

// createTestDoc creates a document through Store.Create and registers cleanup
// for both rows, returning the document at content_version 1.
func createTestDoc(t *testing.T, db *sql.DB, s *store.Store, key string) int64 {
	t.Helper()
	doc := cdmfixtures.JaroWinkler()
	doc.Key = key
	cleanupDocument(t, db, key)

	res, err := s.Create(context.Background(), &doc, store.DraftInput{
		TenantID: "tenant-x",
		Title:    doc.Title,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	cleanupInput(t, db, res.InputRecordID)
	return res.InputRecordID
}

// TestSave_StaleVersionIsRejected is the core of DR6. Without an expected
// version, Store.Save unconditionally increments, so a client that loaded
// version 7, went away, and saved would silently overwrite whatever happened
// in between.
func TestSave_StaleVersionIsRejected(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	createTestDoc(t, db, s, key)

	doc, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	loadedVersion := doc.ContentVersion

	// First save succeeds and moves the stored version forward.
	if _, err := s.Save(context.Background(), doc, loadedVersion); err != nil {
		t.Fatalf("first save: %v", err)
	}

	// A second save still claiming the original version is stale.
	stale, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	stale.Title = "Changed By Stale Writer"

	_, err = s.Save(context.Background(), stale, loadedVersion)
	if err == nil {
		t.Fatal("expected a stale save to be rejected")
	}

	var sve *store.StaleVersionError
	if !errors.As(err, &sve) {
		t.Fatalf("expected *store.StaleVersionError, got %T: %v", err, err)
	}
	if sve.Expected != loadedVersion {
		t.Errorf("Expected = %d, want %d", sve.Expected, loadedVersion)
	}
	if sve.Actual <= loadedVersion {
		t.Errorf("Actual = %d, should be greater than the stale expected version %d",
			sve.Actual, loadedVersion)
	}

	// The rejected write must not have landed.
	after, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if after.Title == "Changed By Stale Writer" {
		t.Error("stale save was rejected but its content was written anyway")
	}
}

// TestSave_ConcurrentSavesExactlyOneWins checks that two savers starting from
// the same loaded version do not both succeed.
//
// This test does NOT prove the row lock is present: verified empirically that
// it stays green with FOR UPDATE removed, because one goroutine typically
// commits before the other reads, so the lost-update window never opens. The
// deterministic proof of the lock is
// TestLockDocStateTx_BlocksConcurrentReader in lock_internal_test.go. What
// this test covers is the end-to-end behavior through the public API — that a
// caller losing the race gets a StaleVersionError rather than silent success.
func TestSave_ConcurrentSavesExactlyOneWins(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	key := uniqueKey(t)
	createTestDoc(t, db, s, key)

	base, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	version := base.ContentVersion

	docA, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load A: %v", err)
	}
	docB, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load B: %v", err)
	}
	docA.Title = "Writer A"
	docB.Title = "Writer B"

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		failures []error
		wins     int
	)
	save := func(doc *model.Document) {
		defer wg.Done()
		_, err := s.Save(context.Background(), doc, version)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			failures = append(failures, err)
			return
		}
		wins++
	}

	wg.Add(2)
	go save(docA)
	go save(docB)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("expected exactly 1 concurrent save to succeed, got %d (failures: %v)", wins, failures)
	}
	if len(failures) != 1 {
		t.Fatalf("expected exactly 1 rejection, got %d", len(failures))
	}
	var sve *store.StaleVersionError
	if !errors.As(failures[0], &sve) {
		t.Fatalf("the losing save should report a stale version, got %T: %v", failures[0], failures[0])
	}
}

// TestSave_PublishedDocumentIsFrozen covers D8. The rule lives in the store
// rather than the handler so every future writer inherits it.
func TestSave_PublishedDocumentIsFrozen(t *testing.T) {
	db := testDB(t)
	s := store.New(db)
	r := store.NewInputRegistrar(db)
	key := uniqueKey(t)
	inputID := createTestDoc(t, db, s, key)

	doc, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// A draft is writable.
	if _, err := s.Save(context.Background(), doc, doc.ContentVersion); err != nil {
		t.Fatalf("saving a draft should succeed: %v", err)
	}

	if err := r.Publish(context.Background(), inputID); err != nil {
		t.Fatalf("publish: %v", err)
	}

	frozen, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	frozen.Title = "Edited After Publish"

	_, err = s.Save(context.Background(), frozen, frozen.ContentVersion)
	if err == nil {
		t.Fatal("expected saving a published document to be refused")
	}
	var fe *store.FrozenError
	if !errors.As(err, &fe) {
		t.Fatalf("expected *store.FrozenError, got %T: %v", err, err)
	}
	if fe.DocumentKey != key {
		t.Errorf("FrozenError.DocumentKey = %q, want %q", fe.DocumentKey, key)
	}

	after, err := s.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("load after: %v", err)
	}
	if after.Title == "Edited After Publish" {
		t.Error("published document was modified despite the frozen rule")
	}
}
