package keywords

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestPromoteCatalogEntriesCreatesFlaggedConceptsThenConverges seeds real
// kb.keyword_sources/kb.keyword_catalog_entries/kb.keyword_catalog_labels
// rows and calls PromoteCatalogEntries against the raw *sql.DB (not a
// wrapping transaction: PromoteCatalogEntries relies on each concept-create
// attempt being its own auto-committed statement so a collision can be
// recovered from by a following SELECT -- inside one shared transaction,
// Postgres aborts everything after the first failed INSERT).
//
// kb.keyword_sources/kb.keyword_catalog_entries/kb.keyword_catalog_labels
// are immutable by trigger (UPDATE/DELETE always raises), so this test's
// fixture rows are never cleaned up -- this matches production reality
// (these are permanent evidence tables; nothing in the real system deletes
// them either) rather than being an oversight. Only the mutable side
// (kb.keyword_concepts/kb.keyword_surfaces) is cleaned up. The source name
// embeds a per-run timestamp so repeated runs never collide.
func TestPromoteCatalogEntriesCreatesFlaggedConceptsThenConverges(t *testing.T) {
	db := openReconcileTestDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	source := "testsrc_" + suffix
	release := "1.0.0"
	scope := "testscope_" + suffix
	entryA := "https://example.test/entry/a-" + suffix
	entryB := "https://example.test/entry/b-" + suffix
	labelA := "Test Quantity A " + suffix
	labelB := "Test Quantity B " + suffix

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM kb.keyword_surfaces WHERE scope = $1`, scope)
		_, _ = db.Exec(`DELETE FROM kb.keyword_concepts WHERE scope = $1`, scope)
	})

	if _, err := db.ExecContext(ctx,
		`INSERT INTO kb.keyword_sources (source, release, license, notes) VALUES ($1, $2, '', 'test fixture')`,
		source, release,
	); err != nil {
		t.Fatalf("seed keyword_sources: %v", err)
	}
	for _, e := range []struct{ id, label string }{{entryA, labelA}, {entryB, labelB}} {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO kb.keyword_catalog_entries (source, release, external_id, entry_status) VALUES ($1, $2, $3, 'current')`,
			source, release, e.id,
		); err != nil {
			t.Fatalf("seed keyword_catalog_entries %s: %v", e.id, err)
		}
		if _, err := db.ExecContext(ctx,
			`INSERT INTO kb.keyword_catalog_labels (source, release, external_id, language, label_role, label) VALUES ($1, $2, $3, 'en', 'preferred', $4)`,
			source, release, e.id, e.label,
		); err != nil {
			t.Fatalf("seed keyword_catalog_labels %s: %v", e.id, err)
		}
	}
	// A deprecated entry must never be promoted.
	deprecatedID := "https://example.test/entry/deprecated-" + suffix
	if _, err := db.ExecContext(ctx,
		`INSERT INTO kb.keyword_catalog_entries (source, release, external_id, entry_status) VALUES ($1, $2, $3, 'deprecated')`,
		source, release, deprecatedID,
	); err != nil {
		t.Fatalf("seed deprecated catalog entry: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO kb.keyword_catalog_labels (source, release, external_id, language, label_role, label) VALUES ($1, $2, $3, 'en', 'preferred', $4)`,
		source, release, deprecatedID, "Deprecated Quantity "+suffix,
	); err != nil {
		t.Fatalf("seed deprecated catalog label: %v", err)
	}

	counts, err := PromoteCatalogEntries(ctx, db, source, release, scope)
	if err != nil {
		t.Fatalf("PromoteCatalogEntries: %v", err)
	}
	if counts.EntriesScanned != 2 || counts.ConceptsCreated != 2 || counts.ConceptsConverged != 0 || counts.Errors != 0 {
		t.Fatalf("first pass counts = %+v, want {EntriesScanned:2 ConceptsCreated:2 ConceptsConverged:0 Errors:0}", counts)
	}

	glossSource := "auto:import:" + source
	for _, label := range []string{labelA, labelB} {
		var status, gs string
		if err := db.QueryRowContext(ctx,
			`SELECT status, gloss_source FROM kb.keyword_concepts WHERE scope = $1 AND pref_label = $2`,
			scope, label,
		).Scan(&status, &gs); err != nil {
			t.Fatalf("query concept for %q: %v", label, err)
		}
		if status != "provisional" || gs != glossSource {
			t.Fatalf("concept for %q: status=%q gloss_source=%q, want provisional/%q", label, status, gs, glossSource)
		}
		var surfaceProvenance string
		if err := db.QueryRowContext(ctx,
			`SELECT provenance FROM kb.keyword_surfaces WHERE scope = $1 AND surface = $2`,
			scope, label,
		).Scan(&surfaceProvenance); err != nil {
			t.Fatalf("query surface for %q: %v", label, err)
		}
		if surfaceProvenance != glossSource {
			t.Fatalf("surface provenance for %q = %q, want %q", label, surfaceProvenance, glossSource)
		}
	}
	var deprecatedCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM kb.keyword_concepts WHERE scope = $1 AND pref_label = $2`,
		scope, "Deprecated Quantity "+suffix,
	).Scan(&deprecatedCount); err != nil {
		t.Fatalf("query deprecated concept count: %v", err)
	}
	if deprecatedCount != 0 {
		t.Fatalf("deprecated entry was promoted: %d concepts found", deprecatedCount)
	}

	// Re-running must converge on the same concepts, not duplicate them.
	counts2, err := PromoteCatalogEntries(ctx, db, source, release, scope)
	if err != nil {
		t.Fatalf("PromoteCatalogEntries (retry): %v", err)
	}
	if counts2.ConceptsCreated != 0 || counts2.ConceptsConverged != 2 {
		t.Fatalf("retry counts = %+v, want ConceptsCreated:0 ConceptsConverged:2", counts2)
	}
	var conceptCount int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM kb.keyword_concepts WHERE scope = $1`, scope,
	).Scan(&conceptCount); err != nil {
		t.Fatalf("query concept count: %v", err)
	}
	if conceptCount != 2 {
		t.Fatalf("concept count after retry = %d, want 2 (no duplicates)", conceptCount)
	}
}
