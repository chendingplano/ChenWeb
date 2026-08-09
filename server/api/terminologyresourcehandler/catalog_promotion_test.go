package terminologyresourcehandler

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

// noopLogger discards everything; used where a JimoLogger is required but
// this test doesn't care what gets logged.
type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Line(string, ...any)  {}
func (noopLogger) Trace(string)         {}
func (noopLogger) Close()               {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}

// TestTriggerCatalogAutoPromotionRespectsPolicy exercises the real
// triggerCatalogAutoPromotion function (not a re-implementation of it)
// against a real test DB, using the real terminology.ResourceUCUM catalog
// entry with a synthetic release so it can never collide with real UCUM
// data. Verifies both halves of the gate: a disabled policy prevents any
// concept from appearing, and an enabled one lets promotion happen -- all
// through the background/fire-and-forget path, polled for since it's async
// by design.
func TestTriggerCatalogAutoPromotionRespectsPolicy(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	source := "ucum" // a real registered resource; only the release is synthetic
	release := "test-trigger-" + suffix
	scope := "display" // terminology.ResourceUCUM's real AllowedScopes[0]
	entryID := "https://example.test/ucum-entry-" + suffix
	label := "Test Trigger Quantity " + suffix

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM kb.keyword_surfaces WHERE surface = $1`, label)
		_, _ = db.Exec(`DELETE FROM kb.keyword_concepts WHERE pref_label = $1`, label)
		_, _ = db.Exec(`DELETE FROM kb.keyword_source_promotion_policy WHERE source = $1`, source)
	})

	if _, err := db.ExecContext(ctx,
		`INSERT INTO kb.keyword_sources (source, release, license, notes) VALUES ($1, $2, '', 'test fixture')`,
		source, release,
	); err != nil {
		t.Fatalf("seed keyword_sources: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO kb.keyword_catalog_entries (source, release, external_id, entry_status) VALUES ($1, $2, $3, 'current')`,
		source, release, entryID,
	); err != nil {
		t.Fatalf("seed keyword_catalog_entries: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO kb.keyword_catalog_labels (source, release, external_id, language, label_role, label) VALUES ($1, $2, $3, 'en', 'preferred', $4)`,
		source, release, entryID, label,
	); err != nil {
		t.Fatalf("seed keyword_catalog_labels: %v", err)
	}

	st := terminology.FetchStatus{Release: release}

	// Disabled: no concept should ever appear, even after waiting.
	if _, err := (keywords.PromotionPolicyStore{DB: db}).Set(ctx, source, false, "tester"); err != nil {
		t.Fatalf("disable policy: %v", err)
	}
	triggerCatalogAutoPromotion(ctx, db, terminology.ResourceUCUM, st, noopLogger{})
	time.Sleep(300 * time.Millisecond)
	if count := countConceptsByLabel(t, db, label); count != 0 {
		t.Fatalf("concept count with policy disabled = %d, want 0", count)
	}

	// Enabled: the concept should appear, polled for since promotion is async.
	if _, err := (keywords.PromotionPolicyStore{DB: db}).Set(ctx, source, true, "tester"); err != nil {
		t.Fatalf("enable policy: %v", err)
	}
	triggerCatalogAutoPromotion(ctx, db, terminology.ResourceUCUM, st, noopLogger{})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if countConceptsByLabel(t, db, label) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if count := countConceptsByLabel(t, db, label); count != 1 {
		t.Fatalf("concept count with policy enabled = %d, want 1", count)
	}
	var gs string
	if err := db.QueryRowContext(ctx,
		`SELECT gloss_source FROM kb.keyword_concepts WHERE pref_label = $1 AND scope = $2`, label, scope,
	).Scan(&gs); err != nil {
		t.Fatalf("query gloss_source: %v", err)
	}
	if gs != "auto:import:ucum" {
		t.Fatalf("gloss_source = %q, want auto:import:ucum", gs)
	}
}

func countConceptsByLabel(t *testing.T, db *sql.DB, label string) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM kb.keyword_concepts WHERE pref_label = $1`, label,
	).Scan(&count); err != nil {
		t.Fatalf("count concepts: %v", err)
	}
	return count
}
