package datasync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// ensureDataSyncTables creates kb.data_sync_items / kb.data_sync_state if
// they don't already exist on TEST_DATABASE_URL's database. This package's
// convention (see integration_test.go) is that TEST_DATABASE_URL has no
// migrations applied -- other tests here work around that with throwaway
// scratch tables, but insertItem/dbItemByID/etc. always use these two fixed
// table names, so those need to actually exist. Never dropped afterward
// (unlike the scratch tables elsewhere in this package): this mirrors the
// real migrations and is harmless to leave in place.
func ensureDataSyncTables(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE SCHEMA IF NOT EXISTS kb`,
		`CREATE TABLE IF NOT EXISTS kb.data_sync_items (
			id TEXT PRIMARY KEY,
			kind TEXT NOT NULL DEFAULT 'table' CHECK (kind IN ('table','table_with_files')),
			origin TEXT NOT NULL CHECK (origin IN ('local','learned')),
			table_name TEXT NOT NULL,
			cursor_col TEXT NOT NULL,
			natural_key JSONB NOT NULL,
			columns JSONB NOT NULL,
			json_columns JSONB NOT NULL DEFAULT '[]',
			filter TEXT,
			file_column TEXT,
			file_dir_env TEXT,
			file_dir_default_subdir TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_by TEXT,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS kb.data_sync_state (
			sync_item_id TEXT PRIMARY KEY,
			last_cursor TIMESTAMPTZ,
			last_synced_at TIMESTAMPTZ,
			last_row_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT
		)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("ensure data-sync tables: %v", err)
		}
	}
}

// testItemID returns a unique id for one test's rows and registers cleanup
// to delete them from both (shared, real-named) tables afterward.
func testItemID(t *testing.T, db *sql.DB, prefix string) string {
	t.Helper()
	id := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM kb.data_sync_items WHERE id = $1", id)
		_, _ = db.Exec("DELETE FROM kb.data_sync_state WHERE sync_item_id = $1", id)
	})
	return id
}

func testLocalItem(id string) TableSyncItem {
	return TableSyncItem{
		ID:         id,
		Table:      "kb.some_table",
		CursorCol:  "update_time",
		NaturalKey: []string{"natural_col"},
		Columns:    []string{"natural_col", "update_time"},
	}
}

// withProjectDBHandle points ApiTypes.ProjectDBHandle at db for the duration
// of the test -- HandleListSourceItems/HandleListSyncItems read it directly
// rather than taking it as a parameter, matching every other handler in
// this package.
func withProjectDBHandle(t *testing.T, db *sql.DB) {
	t.Helper()
	prev := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	t.Cleanup(func() { ApiTypes.ProjectDBHandle = prev })
}

func TestInsertItemRejectsCompiledIDCollision(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()

	item := testLocalItem("kb_product_names") // collides with the compiled Registry
	if err := insertItem(ctx, db, item, "tester@example.com"); !errors.Is(err, errIDExists) {
		t.Fatalf("insertItem() error = %v, want errIDExists", err)
	}
}

func TestInsertItemRejectsDuplicateDBID(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "dup")

	item := testLocalItem(id)
	if err := insertItem(ctx, db, item, "tester@example.com"); err != nil {
		t.Fatalf("first insertItem() error = %v", err)
	}
	if err := insertItem(ctx, db, item, "tester@example.com"); !errors.Is(err, errIDExists) {
		t.Fatalf("second insertItem() error = %v, want errIDExists", err)
	}
}

func TestResolveItemChecksCompiledThenDB(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "resolve")

	if _, ok, err := ResolveItem(ctx, db, id); err != nil || ok {
		t.Fatalf("ResolveItem() before insert = (ok=%v, err=%v), want (false, nil)", ok, err)
	}

	item := testLocalItem(id)
	if err := insertItem(ctx, db, item, ""); err != nil {
		t.Fatalf("insertItem() error = %v", err)
	}

	got, ok, err := ResolveItem(ctx, db, id)
	if err != nil || !ok {
		t.Fatalf("ResolveItem() after insert = (ok=%v, err=%v), want (true, nil)", ok, err)
	}
	if got.Table != item.Table || got.Origin != OriginLocal {
		t.Fatalf("ResolveItem() = %+v, want Table=%q Origin=local", got, item.Table)
	}

	compiled, ok, err := ResolveItem(ctx, db, "kb_product_names")
	if err != nil || !ok || compiled.Origin != "" {
		t.Fatalf("ResolveItem(kb_product_names) = (%+v, ok=%v, err=%v), want the compiled item (empty Origin)", compiled, ok, err)
	}
}

func TestUpdateItemResetsSyncStateAndOnlyAllowsLocal(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "edit")

	item := testLocalItem(id)
	if err := insertItem(ctx, db, item, ""); err != nil {
		t.Fatalf("insertItem() error = %v", err)
	}
	if err := saveSyncSuccess(ctx, db, id, "2026-09-17T00:00:00Z", 3); err != nil {
		t.Fatalf("saveSyncSuccess() error = %v", err)
	}

	edited := item
	edited.Table = "kb.some_other_table"
	if err := updateItem(ctx, db, id, edited); err != nil {
		t.Fatalf("updateItem() error = %v", err)
	}

	got, ok, err := dbItemByID(ctx, db, id)
	if err != nil || !ok || got.Table != "kb.some_other_table" {
		t.Fatalf("dbItemByID() after edit = (%+v, ok=%v, err=%v), want Table=kb.some_other_table", got, ok, err)
	}
	state, err := getSyncState(ctx, db, id)
	if err != nil {
		t.Fatalf("getSyncState() error = %v", err)
	}
	if state.LastCursor.Valid || state.LastSyncedAt.Valid || state.LastRowCount != 0 {
		t.Fatalf("getSyncState() after edit = %+v, want reset to zero value", state)
	}

	if err := updateItem(ctx, db, "kb_product_names", edited); !errors.Is(err, errCompiledItem) {
		t.Fatalf("updateItem(kb_product_names) error = %v, want errCompiledItem", err)
	}
}

func TestUpdateItemRejectsLearnedOrigin(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "learned-edit")

	item := testLocalItem(id)
	if err := upsertLearnedItem(ctx, db, item); err != nil {
		t.Fatalf("upsertLearnedItem() error = %v", err)
	}

	if err := updateItem(ctx, db, id, item); !errors.Is(err, errLearnedItem) {
		t.Fatalf("updateItem() on learned item error = %v, want errLearnedItem", err)
	}
}

func TestUpsertLearnedItemNeverOverwritesLocal(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "local-wins")

	local := testLocalItem(id)
	local.Table = "kb.local_table"
	if err := insertItem(ctx, db, local, ""); err != nil {
		t.Fatalf("insertItem() error = %v", err)
	}

	learned := testLocalItem(id)
	learned.Table = "kb.learned_table"
	if err := upsertLearnedItem(ctx, db, learned); err != nil {
		t.Fatalf("upsertLearnedItem() error = %v", err)
	}

	got, ok, err := dbItemByID(ctx, db, id)
	if err != nil || !ok || got.Table != "kb.local_table" || got.Origin != OriginLocal {
		t.Fatalf("dbItemByID() = (%+v, ok=%v, err=%v), want the local row unchanged", got, ok, err)
	}
}

func TestUpsertLearnedItemRefreshesExistingLearnedShape(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "learned-refresh")

	first := testLocalItem(id)
	first.Table = "kb.v1"
	if err := upsertLearnedItem(ctx, db, first); err != nil {
		t.Fatalf("first upsertLearnedItem() error = %v", err)
	}

	second := testLocalItem(id)
	second.Table = "kb.v2"
	if err := upsertLearnedItem(ctx, db, second); err != nil {
		t.Fatalf("second upsertLearnedItem() error = %v", err)
	}

	got, ok, err := dbItemByID(ctx, db, id)
	if err != nil || !ok || got.Table != "kb.v2" {
		t.Fatalf("dbItemByID() = (%+v, ok=%v, err=%v), want Table=kb.v2 (refreshed shape)", got, ok, err)
	}
}

func TestDeleteItemRemovesRowAndSyncState(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "delete")

	item := testLocalItem(id)
	if err := insertItem(ctx, db, item, ""); err != nil {
		t.Fatalf("insertItem() error = %v", err)
	}
	if err := saveSyncSuccess(ctx, db, id, "2026-09-17T00:00:00Z", 1); err != nil {
		t.Fatalf("saveSyncSuccess() error = %v", err)
	}

	if err := deleteItem(ctx, db, id); err != nil {
		t.Fatalf("deleteItem() error = %v", err)
	}
	if _, ok, err := dbItemByID(ctx, db, id); err != nil || ok {
		t.Fatalf("dbItemByID() after delete = (ok=%v, err=%v), want (false, nil)", ok, err)
	}
	state, err := getSyncState(ctx, db, id)
	if err != nil {
		t.Fatalf("getSyncState() error = %v", err)
	}
	if state.LastSyncedAt.Valid {
		t.Fatalf("getSyncState() after delete = %+v, want zero value (row removed)", state)
	}

	if err := deleteItem(ctx, db, "kb_product_names"); !errors.Is(err, errCompiledItem) {
		t.Fatalf("deleteItem(kb_product_names) error = %v, want errCompiledItem", err)
	}
	if err := deleteItem(ctx, db, id); !errors.Is(err, errItemNotFound) {
		t.Fatalf("deleteItem() (already gone) error = %v, want errItemNotFound", err)
	}
}

// TestFetchSourceItemDefinitionsRoundTrip exercises real discovery over
// HTTP: an item created on a "source" is advertised via
// HandleListSourceItems and decoded back by fetchSourceItemDefinitions,
// full shape intact including table_with_files fields.
func TestFetchSourceItemDefinitionsRoundTrip(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	withProjectDBHandle(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "discover")

	item := testLocalItem(id)
	item.Kind = KindTableWithFiles
	item.FileColumn = "natural_col"
	if err := insertItem(ctx, db, item, ""); err != nil {
		t.Fatalf("insertItem() error = %v", err)
	}

	t.Setenv("DATA_SYNC_SHARED_SECRET", "test-secret")

	e := echo.New()
	e.GET("/api/internal/data-sync/items", HandleListSourceItems)
	srv := httptest.NewServer(e)
	defer srv.Close()

	defs, err := fetchSourceItemDefinitions(ctx, srv.URL, "test-secret")
	if err != nil {
		t.Fatalf("fetchSourceItemDefinitions() error = %v", err)
	}
	var found *TableSyncItem
	for i := range defs {
		if defs[i].ID == id {
			found = &defs[i]
		}
	}
	if found == nil {
		t.Fatalf("discovered items did not include %q: %+v", id, defs)
	}
	if found.Kind != KindTableWithFiles || found.FileColumn != "natural_col" {
		t.Fatalf("discovered item = %+v, want Kind=table_with_files FileColumn=natural_col", found)
	}
}

func TestUpsertLearnedItemCreatesNewLearnedItem(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "learned-new")

	if err := upsertLearnedItem(ctx, db, testLocalItem(id)); err != nil {
		t.Fatalf("upsertLearnedItem() error = %v", err)
	}

	got, ok, err := dbItemByID(ctx, db, id)
	if err != nil || !ok || got.Origin != OriginLearned {
		t.Fatalf("dbItemByID() = (%+v, ok=%v, err=%v), want a new origin=learned row", got, ok, err)
	}
}

// TestListSyncItemsFallsBackWhenSourceUnreachable covers design.md
// Decision 4's resilience property (and the matching spec scenario
// "Previously-learned item still works when the source is unreachable"):
// an item already cached from a prior successful discovery refresh must
// still resolve and still appear in the list even when the configured
// source is down at the moment of a later list call.
//
// This only needs one database -- unlike a genuine two-instance setup, the
// point here is specifically that HandleListSyncItems must not treat a
// discovery failure as a reason to drop what it already knows, which is
// fully exercisable without actually standing up a second Postgres
// instance to play the "source" role.
func TestListSyncItemsFallsBackWhenSourceUnreachable(t *testing.T) {
	db := testDB(t)
	ensureDataSyncTables(t, db)
	withProjectDBHandle(t, db)
	ctx := context.Background()
	id := testItemID(t, db, "cached")

	// Simulate "already learned on a prior successful refresh".
	if err := upsertLearnedItem(ctx, db, testLocalItem(id)); err != nil {
		t.Fatalf("upsertLearnedItem() error = %v", err)
	}

	t.Setenv("DATA_SYNC_SOURCE_URL", "http://127.0.0.1:1") // nothing listens here
	t.Setenv("DATA_SYNC_SHARED_SECRET", "test-secret")

	e := echo.New()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/data-sync/items", nil)
	c := e.NewContext(req, rec)

	if err := HandleListSyncItems(c); err != nil {
		t.Fatalf("HandleListSyncItems() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a source outage must not fail the list)", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), id) {
		t.Fatalf("list response = %s, want it to still contain cached item %q", rec.Body.String(), id)
	}

	if _, ok, err := ResolveItem(ctx, db, id); err != nil || !ok {
		t.Fatalf("ResolveItem(%q) with source down = (ok=%v, err=%v), want (true, nil) from the cache", id, ok, err)
	}
}
