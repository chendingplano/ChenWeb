package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// This is an internal test (package store, not store_test) because it needs
// lockDocStateTx directly. Driving it through Store.Save cannot prove the row
// lock exists: a two-goroutine race passes with or without FOR UPDATE,
// because one goroutine usually finishes its whole transaction before the
// other reads, so the lost-update window never opens. Verified empirically —
// removing FOR UPDATE left TestSave_ConcurrentSavesExactlyOneWins green
// across repeated runs. Testing the lock directly is deterministic.

func internalTestDB(t *testing.T) *sql.DB {
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

// TestLockDocStateTx_BlocksConcurrentReader proves the FOR UPDATE clause in
// lockDocStateTx actually takes a row lock: a second transaction reading the
// same document must block until the first commits.
//
// This is the guard that makes optimistic concurrency correct. Without the
// lock, two savers can both read content_version 7, both find it matches what
// their client expected, and both increment — producing 8 and 9 and silently
// losing one client's edit.
func TestLockDocStateTx_BlocksConcurrentReader(t *testing.T) {
	db := internalTestDB(t)
	ctx := context.Background()

	key := fmt.Sprintf("doc:lock-test-%d", time.Now().UnixNano())
	var inputID int64
	err := db.QueryRow(`
		INSERT INTO kb.inputs (tenant_id, ks_store_id, type, title, status)
		VALUES ('tenant-x', NULL, 'cdm', 'Lock Test', $1::jsonb) RETURNING id
	`, draftStatus).Scan(&inputID)
	if err != nil {
		t.Fatalf("insert input: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM kb.cdm_documents WHERE document_key = $1`, key) //nolint:errcheck
		db.Exec(`DELETE FROM kb.inputs WHERE id = $1`, inputID)              //nolint:errcheck
	})

	if _, err := db.Exec(`
		INSERT INTO kb.cdm_documents
			(document_key, title, schema_version, content_version, input_record_id, semantic_document)
		VALUES ($1, 'Lock Test', '1.0', 1, $2, '{}'::jsonb)
	`, key, inputID); err != nil {
		t.Fatalf("insert document: %v", err)
	}

	// Transaction 1 takes the lock and holds it.
	tx1, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	defer tx1.Rollback() //nolint:errcheck

	st, err := lockDocStateTx(ctx, tx1, key)
	if err != nil {
		t.Fatalf("tx1 lock: %v", err)
	}
	if !st.exists || st.contentVersion != 1 || st.editVersion != 1 {
		t.Fatalf("tx1 read unexpected state: %+v", st)
	}

	// Transaction 2 attempts the same read; it must not complete while tx1
	// holds the lock.
	type result struct {
		st  docState
		err error
	}
	done := make(chan result, 1)
	go func() {
		tx2, err := db.BeginTx(ctx, nil)
		if err != nil {
			done <- result{err: err}
			return
		}
		defer tx2.Rollback() //nolint:errcheck
		st, err := lockDocStateTx(ctx, tx2, key)
		done <- result{st: st, err: err}
	}()

	select {
	case r := <-done:
		t.Fatalf("second reader completed while the lock was held (state %+v, err %v) — FOR UPDATE is not taking a row lock", r.st, r.err)
	case <-time.After(500 * time.Millisecond):
		// Expected: blocked.
	}

	// Bump the version, then release. The waiter must observe the new value,
	// which is exactly why it is safe for it to re-check the caller's
	// expected version afterwards.
	if _, err := tx1.ExecContext(ctx, `
		UPDATE kb.cdm_documents SET content_version = 2 WHERE document_key = $1
	`, key); err != nil {
		t.Fatalf("tx1 update: %v", err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("tx1 commit: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("second reader failed after the lock was released: %v", r.err)
		}
		if r.st.contentVersion != 2 {
			t.Fatalf("second reader saw content_version %d, want 2 — it read a stale value from before the first writer committed", r.st.contentVersion)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second reader never completed after the lock was released")
	}
}
