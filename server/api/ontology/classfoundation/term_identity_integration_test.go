package classfoundation

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// TestTermIdentityMigrationPreservesHistoryAndLegacyReferences exercises the
// additive migration against a disposable database. It is intentionally
// opt-in because it drops and recreates the kb schema.
func TestTermIdentityMigrationPreservesHistoryAndLegacyReferences(t *testing.T) {
	dsn := os.Getenv("TERM_IDENTITY_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set TERM_IDENTITY_MIGRATION_TEST_DSN to a disposable PostgreSQL database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `
DROP SCHEMA IF EXISTS kb CASCADE;
CREATE SCHEMA kb;
CREATE TABLE kb.ontology_terms (
    id BIGSERIAL PRIMARY KEY,
    term_id TEXT NOT NULL,
    version INT NOT NULL,
    term_kind TEXT NOT NULL,
    module_id TEXT NOT NULL,
    status TEXT NOT NULL,
    definition TEXT,
    scope TEXT,
    source_candidate_id BIGINT,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by TEXT,
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by TEXT,
    released_in_release_id BIGINT,
    value_type TEXT,
    range_type TEXT,
    permitted_unit_term_ids JSONB,
    UNIQUE (term_id, version)
);
CREATE TABLE kb.legacy_term_references (
    id BIGSERIAL PRIMARY KEY,
    term_row_id BIGINT NOT NULL REFERENCES kb.ontology_terms (id)
);
INSERT INTO kb.ontology_terms (term_id, version, term_kind, module_id, status, definition)
VALUES ('core:amount', 1, 'class', 'core', 'draft', 'initial'),
       ('core:amount', 2, 'class', 'core', 'approved', 'approved');
INSERT INTO kb.legacy_term_references (term_row_id) VALUES (1);`); err != nil {
		t.Fatalf("prepare legacy term state: %v", err)
	}
	defer db.ExecContext(ctx, `DROP SCHEMA IF EXISTS kb CASCADE`)

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000009_create_ontology_term_identity_foundation.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, filepath.Base(migrationPath)), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	goose.SetLogger(goose.NopLogger())
	goose.SetTableName("goose_term_identity_test_version")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(ctx, `DROP TABLE IF EXISTS public.goose_term_identity_test_version`)
	if err := goose.UpContext(ctx, db, dir); err != nil {
		t.Fatalf("goose Up: %v", err)
	}

	var revisions, legacyReferences int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.ontology_term_revisions WHERE term_id = 'core:amount'`).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatalf("historical revisions = %d, err = %v", revisions, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM kb.legacy_term_references r JOIN kb.ontology_terms t ON t.id = r.term_row_id WHERE t.term_id = 'core:amount' AND t.version = 1`).Scan(&legacyReferences); err != nil || legacyReferences != 1 {
		t.Fatalf("legacy references = %d, err = %v", legacyReferences, err)
	}

	var currentVersion, currentSourceID int
	if err := db.QueryRowContext(ctx, `SELECT version, id FROM kb.ontology_terms_current WHERE term_id = 'core:amount'`).Scan(&currentVersion, &currentSourceID); err != nil || currentVersion != 2 || currentSourceID != 2 {
		t.Fatalf("current compatibility row = version %d id %d, err = %v", currentVersion, currentSourceID, err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO kb.ontology_terms (term_id, version, term_kind, module_id, status, definition) VALUES ('core:amount', 3, 'class', 'core', 'included_in_release', 'released')`); err != nil {
		t.Fatalf("insert compatibility version: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT version FROM kb.ontology_terms_current WHERE term_id = 'core:amount'`).Scan(&currentVersion); err != nil || currentVersion != 3 {
		t.Fatalf("triggered current version = %d, err = %v", currentVersion, err)
	}
}
