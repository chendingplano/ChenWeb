package terminologyresourcehandler

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// testDB connects to a real Postgres instance for integration-style
// coverage of the governed-term write and release/activate flow, following
// the TEST_DATABASE_URL convention used elsewhere in this repo (e.g.
// api/doc-benchmark/migration_test.go). Skips when not configured.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func cleanupQuantityModule(t *testing.T, db *sql.DB) {
	t.Helper()
	cleanup := func() {
		for _, stmt := range []string{
			`DELETE FROM kb.ontology_active_releases WHERE module_id = 'quantity'`,
			`DELETE FROM kb.ontology_module_releases WHERE module_id = 'quantity'`,
			`DELETE FROM kb.ontology_mappings WHERE module_id = 'quantity'`,
			`DELETE FROM kb.ontology_term_labels WHERE term_id LIKE 'quantity:%'`,
			`DELETE FROM kb.ontology_terms WHERE module_id = 'quantity'`,
			`DELETE FROM kb.ontology_modules WHERE module_id = 'quantity'`,
		} {
			if _, err := db.Exec(stmt); err != nil {
				t.Logf("cleanup %q: %v", stmt, err)
			}
		}
	}
	cleanup() // in case a prior failed run left rows behind
	t.Cleanup(cleanup)
}

// ensureCoreModuleReleased seeds a minimal, released `core` module if the
// test database doesn't already have one: releasing `quantity` requires its
// declared dependency (`core`) to be a registered module with at least one
// release to pin against, mirroring the real deployment where `core` is
// already released (ADR 2026072901 §15.2). Only cleans up what it created,
// so it's a no-op against a dev DB that already has `core`.
func ensureCoreModuleReleased(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	ms := modules.ModuleStore{DB: db}
	if _, err := ms.GetModule(ctx, "core"); err == nil {
		return
	}
	if _, err := ms.CreateModule(ctx, modules.Module{
		ModuleID: "core", Title: "test core module", Owner: "test", Status: "active",
		CreateBy: "test", ModifyBy: "test",
	}); err != nil {
		t.Fatalf("seed core module: %v", err)
	}
	if _, err := (terms.TermStore{DB: db}).CreateTerm(ctx, terms.Term{
		TermID: "core:test_referent", TermKind: "class", ModuleID: "core", Status: "approved",
		CreateBy: "test", ModifyBy: "test",
	}); err != nil {
		t.Fatalf("seed core term: %v", err)
	}
	rs := modules.ReleaseStore{DB: db}
	rel, err := rs.CreateRelease(ctx, "core", "1.0.0", "test")
	if err != nil {
		t.Fatalf("seed core release: %v", err)
	}
	if _, err := rs.Activate(ctx, "core", rel.ID, "test"); err != nil {
		t.Fatalf("activate core release: %v", err)
	}
	t.Cleanup(func() {
		for _, stmt := range []string{
			`DELETE FROM kb.ontology_active_releases WHERE module_id = 'core'`,
			`DELETE FROM kb.ontology_module_releases WHERE module_id = 'core'`,
			`DELETE FROM kb.ontology_terms WHERE module_id = 'core'`,
			`DELETE FROM kb.ontology_modules WHERE module_id = 'core'`,
		} {
			if _, err := db.Exec(stmt); err != nil {
				t.Logf("cleanup core %q: %v", stmt, err)
			}
		}
	})
}

func sampleQUDTTerms() []terminology.QUDTImportedTerm {
	return []terminology.QUDTImportedTerm{
		{TermID: "quantity:unit_M", Kind: "unit", SourceIRI: "http://qudt.org/vocab/unit/M", PrefLabel: "Metre", Symbol: "m"},
		{TermID: "quantity:qk_Luminance", Kind: "quantity_kind", SourceIRI: "http://qudt.org/vocab/quantitykind/Luminance", PrefLabel: "Luminance"},
		{TermID: "quantity:dim_L", Kind: "dimension", SourceIRI: "http://qudt.org/vocab/dimensionvector/L", PrefLabel: "Length"},
	}
}

// TestWriteQUDTOntologyTermsInsertsAllThreeKindsThenSkipsOnRetry covers
// spec.md's "First-time approve populates all three term kinds" and
// "Re-approve with unchanged content" scenarios at the write-function level.
func TestWriteQUDTOntologyTermsInsertsAllThreeKindsThenSkipsOnRetry(t *testing.T) {
	db := testDB(t)
	cleanupQuantityModule(t, db)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := writeQUDTOntologyTerms(ctx, tx, sampleQUDTTerms(), "tester")
	if err != nil {
		t.Fatalf("writeQUDTOntologyTerms: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// unit_M gets a prefLabel + a symbol altLabel; the other two get only a
	// prefLabel: 2 + 1 + 1 = 4 labels. Every term gets exactly one mapping.
	if !got.OK || got.TermsInserted != 3 || got.LabelsInserted != 4 || got.MappingsInserted != 3 {
		t.Fatalf("first write = %+v, want OK with terms=3 labels=4 mappings=3", got)
	}

	var kind, status string
	if err := db.QueryRowContext(ctx,
		`SELECT term_kind, status FROM kb.ontology_terms WHERE term_id = 'quantity:unit_M'`,
	).Scan(&kind, &status); err != nil {
		t.Fatal(err)
	}
	if kind != "unit" || status != "approved" {
		t.Fatalf("unit_M kind=%q status=%q, want unit/approved", kind, status)
	}
	var toIRI, relation, approval string
	if err := db.QueryRowContext(ctx,
		`SELECT to_iri, relation, approval_status FROM kb.ontology_mappings WHERE from_term_id = 'quantity:unit_M'`,
	).Scan(&toIRI, &relation, &approval); err != nil {
		t.Fatal(err)
	}
	if toIRI != "http://qudt.org/vocab/unit/M" || relation != "exact" || approval != "approved" {
		t.Fatalf("mapping to_iri=%q relation=%q approval=%q, want the source IRI / exact / approved", toIRI, relation, approval)
	}

	// Re-approving identical content must not duplicate anything.
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := writeQUDTOntologyTerms(ctx, tx2, sampleQUDTTerms(), "tester")
	if err != nil {
		t.Fatalf("writeQUDTOntologyTerms (retry): %v", err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	if !got2.OK || got2.TermsInserted != 0 || got2.LabelsInserted != 0 || got2.MappingsInserted != 0 {
		t.Fatalf("retry write = %+v, want a no-op (0/0/0)", got2)
	}
}

// TestReleaseQUDTIfPendingCreatesActivatesAndIsIdempotent covers spec.md's
// "New terms trigger a release and activation" and "No new terms means no
// new release" scenarios, plus repeated-release versioning (tasks 3.1/3.2).
func TestReleaseQUDTIfPendingCreatesActivatesAndIsIdempotent(t *testing.T) {
	db := testDB(t)
	cleanupQuantityModule(t, db)
	ensureCoreModuleReleased(t, db)
	ctx := context.Background()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeQUDTOntologyTerms(ctx, tx, sampleQUDTTerms(), "tester"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	version, err := releaseQUDTIfPending(ctx, db, "tester")
	if err != nil {
		t.Fatalf("releaseQUDTIfPending: %v", err)
	}
	if version != "1.0.0" {
		t.Fatalf("version = %q, want 1.0.0", version)
	}

	var status string
	if err := db.QueryRowContext(ctx,
		`SELECT status FROM kb.ontology_terms WHERE term_id = 'quantity:unit_M'`,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "included_in_release" {
		t.Fatalf("status = %q, want included_in_release", status)
	}

	var activeVersion string
	if err := db.QueryRowContext(ctx, `
SELECT r.version FROM kb.ontology_active_releases ar
JOIN kb.ontology_module_releases r ON r.id = ar.release_id
WHERE ar.module_id = 'quantity' AND ar.deactivated_at IS NULL`,
	).Scan(&activeVersion); err != nil {
		t.Fatal(err)
	}
	if activeVersion != "1.0.0" {
		t.Fatalf("active release version = %q, want 1.0.0", activeVersion)
	}

	// Nothing pending now: must be a true no-op, not an error.
	version2, err := releaseQUDTIfPending(ctx, db, "tester")
	if err != nil {
		t.Fatalf("releaseQUDTIfPending (idempotent): %v", err)
	}
	if version2 != "" {
		t.Fatalf("second call version = %q, want empty (nothing pending)", version2)
	}

	// New content arrives later: the next release must bump the patch version.
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	more := []terminology.QUDTImportedTerm{
		{TermID: "quantity:unit_KG", Kind: "unit", SourceIRI: "http://qudt.org/vocab/unit/KiloGM", PrefLabel: "Kilogram"},
	}
	if _, err := writeQUDTOntologyTerms(ctx, tx2, more, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	version3, err := releaseQUDTIfPending(ctx, db, "tester")
	if err != nil {
		t.Fatalf("releaseQUDTIfPending (second release): %v", err)
	}
	if version3 != "1.0.1" {
		t.Fatalf("version3 = %q, want 1.0.1", version3)
	}
}

// TestReleaseQUDTIfPendingResumesStrandedApprovedTerms covers spec.md's
// "Retry after a failed ontology write completes the work" scenario for the
// narrower Step-B-only failure case design.md Decision 4 targets: terms
// committed by Step A in an earlier call, but Step B never ran.
func TestReleaseQUDTIfPendingResumesStrandedApprovedTerms(t *testing.T) {
	db := testDB(t)
	cleanupQuantityModule(t, db)
	ensureCoreModuleReleased(t, db)
	ctx := context.Background()

	// Simulate Step A having already succeeded (e.g. in a prior request whose
	// Step B then failed): a term sits at status='approved' with no release.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeQUDTOntologyTerms(ctx, tx, sampleQUDTTerms()[:1], "tester"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	// This retry's Step A finds the term already present (0 new insertions)...
	tx2, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := writeQUDTOntologyTerms(ctx, tx2, sampleQUDTTerms()[:1], "tester")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx2.Commit(); err != nil {
		t.Fatal(err)
	}
	if got.TermsInserted != 0 {
		t.Fatalf("expected 0 new terms on retry, got %d", got.TermsInserted)
	}

	// ...but Step B must still find and release the term stranded from before.
	version, err := releaseQUDTIfPending(ctx, db, "tester")
	if err != nil {
		t.Fatalf("releaseQUDTIfPending: %v", err)
	}
	if version == "" {
		t.Fatal("expected a release to be created for the stranded approved term")
	}
}
