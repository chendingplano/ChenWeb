package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// testDB returns a live *sql.DB for TEST_DATABASE_URL, skipping the test if
// it is not set, matching the convention in
// server/api/cdm/store/store_test.go.
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

// scratchProductNamesTable creates a throwaway table shaped like
// kb.product_names under public (so this test needs no pre-existing kb
// schema/migrations on TEST_DATABASE_URL), and drops it on cleanup.
func scratchProductNamesTable(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id               BIGSERIAL PRIMARY KEY,
			seq_no           INTEGER NOT NULL,
			sub_catalog      TEXT NOT NULL DEFAULT '',
			category_l1      TEXT NOT NULL DEFAULT '',
			category_l2      TEXT NOT NULL DEFAULT '',
			description      TEXT NOT NULL DEFAULT '',
			intended_use     TEXT NOT NULL DEFAULT '',
			product_name     TEXT NOT NULL,
			product_name_en  TEXT NOT NULL DEFAULT '',
			regulatory_class TEXT NOT NULL DEFAULT '',
			aliases          JSONB NOT NULL DEFAULT '[]'::jsonb,
			keywords         JSONB NOT NULL DEFAULT '{}'::jsonb,
			source           TEXT NOT NULL,
			notes            TEXT NOT NULL DEFAULT '',
			extra_info       JSONB NOT NULL DEFAULT '{}'::jsonb,
			status           TEXT NOT NULL DEFAULT 'proposed',
			update_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (source, seq_no, product_name)
		)`, name))
	if err != nil {
		t.Fatalf("create scratch table %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + name); err != nil {
			t.Logf("cleanup: drop table %s: %v", name, err)
		}
	})
}

func scratchItem(table string) TableSyncItem {
	return TableSyncItem{
		ID:          "test_item",
		Table:       table,
		CursorCol:   "update_time",
		NaturalKey:  []string{"source", "seq_no", "product_name"},
		Columns:     testItem().Columns,
		JSONColumns: testItem().JSONColumns,
	}
}

// TestFetchChangesPageAgainstRealPostgres exercises the pull query against a
// real database (JSONB columns, TIMESTAMPTZ cursor, real driver value types)
// -- the part sqlmock-based tests cannot validate.
func TestFetchChangesPageAgainstRealPostgres(t *testing.T) {
	db := testDB(t)
	table := fmt.Sprintf("public.datasync_it_source_%d", time.Now().UnixNano())
	scratchProductNamesTable(t, db, table)
	item := scratchItem(table)

	_, err := db.Exec(fmt.Sprintf(`
		INSERT INTO %s (seq_no, product_name, product_name_en, source, aliases, keywords)
		VALUES (1, 'Widget', 'Widget EN', 'cn_nmpa_medical_device_classification_catalog', '["W"]'::jsonb, '{"k":1}'::jsonb)`, table))
	if err != nil {
		t.Fatalf("seed row: %v", err)
	}

	resp, err := fetchChangesPage(db, item, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage() error = %v", err)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("len(Rows) = %d, want 1", len(resp.Rows))
	}
	if resp.HasMore {
		t.Fatalf("HasMore = true, want false")
	}
	if resp.NextCursor == "" {
		t.Fatalf("NextCursor is empty, want a real timestamp")
	}
	if string(resp.Rows[0]["aliases"]) != `["W"]` {
		t.Fatalf("aliases = %s, want [\"W\"]", resp.Rows[0]["aliases"])
	}

	// A second page starting from NextCursor should be empty (no changes since).
	resp2, err := fetchChangesPage(db, item, resp.NextCursor, 100)
	if err != nil {
		t.Fatalf("fetchChangesPage() (second page) error = %v", err)
	}
	if len(resp2.Rows) != 0 {
		t.Fatalf("len(Rows) on second page = %d, want 0 (nothing changed since)", len(resp2.Rows))
	}
}

// TestUpsertRowsAgainstRealPostgres exercises the full preview/apply round
// trip against a real database: fetch from a "source" scratch table, apply
// (insert) into a "target" scratch table, then fetch+apply again after an
// update to confirm the natural-key ON CONFLICT path updates in place.
func TestUpsertRowsAgainstRealPostgres(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	sourceTable := fmt.Sprintf("public.datasync_it_src_%d", time.Now().UnixNano())
	targetTable := fmt.Sprintf("public.datasync_it_tgt_%d", time.Now().UnixNano())
	scratchProductNamesTable(t, db, sourceTable)
	scratchProductNamesTable(t, db, targetTable)
	sourceItem := scratchItem(sourceTable)
	targetItem := scratchItem(targetTable)

	_, err := db.Exec(fmt.Sprintf(`
		INSERT INTO %s (seq_no, product_name, product_name_en, source)
		VALUES (1, 'Widget', 'Widget EN', 'cn_nmpa_medical_device_classification_catalog')`, sourceTable))
	if err != nil {
		t.Fatalf("seed source row: %v", err)
	}

	// "Preview"/"apply" round 1: target has never synced (cursor = "").
	page1, err := fetchChangesPage(db, sourceItem, "", 100)
	if err != nil {
		t.Fatalf("fetchChangesPage() round 1 error = %v", err)
	}
	if err := upsertRows(ctx, db, targetItem, page1.Rows); err != nil {
		t.Fatalf("upsertRows() round 1 error = %v", err)
	}

	var count int
	var nameEN string
	if err := db.QueryRow(fmt.Sprintf("SELECT count(*), max(product_name_en) FROM %s", targetTable)).Scan(&count, &nameEN); err != nil {
		t.Fatalf("count target after round 1: %v", err)
	}
	if count != 1 || nameEN != "Widget EN" {
		t.Fatalf("target after round 1 = (count=%d, name_en=%q), want (1, \"Widget EN\")", count, nameEN)
	}

	// Change the source row (same natural key) and sync again -- this must
	// UPDATE the existing target row, not insert a second one. This scratch
	// table has no kb.set_update_time trigger (that's only migrated onto the
	// real kb.product_names table), so the update_time bump is done by hand
	// here to simulate what the trigger does in production.
	_, err = db.Exec(fmt.Sprintf(
		`UPDATE %s SET product_name_en = 'Widget EN v2', update_time = NOW() WHERE seq_no = 1`, sourceTable))
	if err != nil {
		t.Fatalf("update source row: %v", err)
	}

	page2, err := fetchChangesPage(db, sourceItem, page1.NextCursor, 100)
	if err != nil {
		t.Fatalf("fetchChangesPage() round 2 error = %v", err)
	}
	if len(page2.Rows) != 1 {
		t.Fatalf("len(Rows) round 2 = %d, want 1 (the updated row)", len(page2.Rows))
	}
	if err := upsertRows(ctx, db, targetItem, page2.Rows); err != nil {
		t.Fatalf("upsertRows() round 2 error = %v", err)
	}

	if err := db.QueryRow(fmt.Sprintf("SELECT count(*), max(product_name_en) FROM %s", targetTable)).Scan(&count, &nameEN); err != nil {
		t.Fatalf("count target after round 2: %v", err)
	}
	if count != 1 || nameEN != "Widget EN v2" {
		t.Fatalf("target after round 2 = (count=%d, name_en=%q), want (1, \"Widget EN v2\") -- upsert should update in place, not insert a duplicate", count, nameEN)
	}

	// A third preview from the round-2 cursor should show nothing pending.
	page3, err := fetchChangesPage(db, sourceItem, page2.NextCursor, 100)
	if err != nil {
		t.Fatalf("fetchChangesPage() round 3 error = %v", err)
	}
	if len(page3.Rows) != 0 {
		t.Fatalf("len(Rows) round 3 = %d, want 0", len(page3.Rows))
	}
}

// TestUpsertRowsSkipsExistingLogicalRowWhenNaturalKeyDiffers covers rows that
// existed before a portable UUID natural key was added. The source and target
// have different UUIDs, but the target's other unique identity and cursor are
// already current, so applying the source row must be a no-op rather than a
// duplicate-key failure.
func TestUpsertRowsSkipsExistingLogicalRowWhenNaturalKeyDiffers(t *testing.T) {
	db := testDB(t)
	table := fmt.Sprintf("public.datasync_it_uuid_%d", time.Now().UnixNano())
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			uid UUID NOT NULL UNIQUE,
			page_key TEXT NOT NULL UNIQUE,
			update_time TIMESTAMPTZ NOT NULL,
			title TEXT NOT NULL
		)`, table))
	if err != nil {
		t.Fatalf("create scratch table: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE IF EXISTS " + table) })

	item := TableSyncItem{
		ID:         "uuid-mismatch",
		Table:      table,
		CursorCol:  "update_time",
		NaturalKey: []string{"uid"},
		Columns:    []string{"uid", "page_key", "update_time", "title"},
	}
	if _, err := db.Exec(fmt.Sprintf(
		`INSERT INTO %s (uid, page_key, update_time, title) VALUES ('00000000-0000-0000-0000-000000000002', 'home3-knowledge', '2026-09-17T00:00:00Z', 'current')`, table)); err != nil {
		t.Fatalf("seed target row: %v", err)
	}

	row := Row{
		"uid":         json.RawMessage(`"00000000-0000-0000-0000-000000000001"`),
		"page_key":    json.RawMessage(`"home3-knowledge"`),
		"update_time": json.RawMessage(`"2026-09-17T00:00:00Z"`),
		"title":       json.RawMessage(`"current"`),
	}
	if err := upsertRows(context.Background(), db, item, []Row{row}); err != nil {
		t.Fatalf("upsertRows() with equal cursor and mismatched UUID: %v", err)
	}

	var uid, title string
	if err := db.QueryRow(fmt.Sprintf(`SELECT uid::text, title FROM %s`, table)).Scan(&uid, &title); err != nil {
		t.Fatalf("read target row: %v", err)
	}
	var count int
	if err := db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s`, table)).Scan(&count); err != nil {
		t.Fatalf("count target rows: %v", err)
	}
	if count != 1 || uid != "00000000-0000-0000-0000-000000000002" || title != "current" {
		t.Fatalf("target row after equal-cursor apply = (count=%d, uid=%q, title=%q), want the existing row unchanged", count, uid, title)
	}
}
