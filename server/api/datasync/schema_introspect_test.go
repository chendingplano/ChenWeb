package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// scratchIntrospectTable creates a throwaway table shaped like a realistic
// syncable table: a surrogate BIGSERIAL id, a column with its own UNIQUE
// constraint (the intended natural key), a TIMESTAMPTZ cursor column, and a
// JSONB column -- enough to exercise every branch of resolveItemShape.
func scratchIntrospectTable(t *testing.T, db *sql.DB, name string) {
	t.Helper()
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE %s (
			id          BIGSERIAL PRIMARY KEY,
			slug        TEXT NOT NULL UNIQUE,
			payload     JSONB NOT NULL DEFAULT '{}'::jsonb,
			update_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			label       TEXT NOT NULL DEFAULT ''
		)`, name))
	if err != nil {
		t.Fatalf("create scratch introspect table %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP TABLE IF EXISTS " + name); err != nil {
			t.Logf("cleanup: drop table %s: %v", name, err)
		}
	})
}

func scratchTableName(prefix string) string {
	return fmt.Sprintf("datasync_it_schema_%s_%d", prefix, time.Now().UnixNano())
}

func TestListSyncableTablesIncludesScratchTable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("list")
	scratchIntrospectTable(t, db, "public."+name)

	tables, err := listSyncableTables(ctx, db)
	if err != nil {
		t.Fatalf("listSyncableTables() error = %v", err)
	}
	found := false
	for _, tbl := range tables {
		if tbl.Schema == "public" && tbl.Table == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("listSyncableTables() = %+v, want it to include public.%s", tables, name)
	}
}

func TestTableColumnsAndPrimaryKey(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("cols")
	scratchIntrospectTable(t, db, "public."+name)

	cols, err := tableColumns(ctx, db, "public", name)
	if err != nil {
		t.Fatalf("tableColumns() error = %v", err)
	}
	want := map[string]string{
		"id": "bigint", "slug": "text", "payload": "jsonb",
		"update_time": "timestamp with time zone", "label": "text",
	}
	if len(cols) != len(want) {
		t.Fatalf("tableColumns() = %+v, want %d columns", cols, len(want))
	}
	for _, c := range cols {
		if want[c.Name] != c.DataType {
			t.Fatalf("column %s data_type = %q, want %q", c.Name, c.DataType, want[c.Name])
		}
	}

	pk, err := tablePrimaryKey(ctx, db, "public", name)
	if err != nil {
		t.Fatalf("tablePrimaryKey() error = %v", err)
	}
	if len(pk) != 1 || pk[0] != "id" {
		t.Fatalf("tablePrimaryKey() = %v, want [id]", pk)
	}
}

func TestTableNaturalKeyCandidatesExcludesPrimaryKey(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("nk")
	scratchIntrospectTable(t, db, "public."+name)

	pk, err := tablePrimaryKey(ctx, db, "public", name)
	if err != nil {
		t.Fatalf("tablePrimaryKey() error = %v", err)
	}
	candidates, err := tableNaturalKeyCandidates(ctx, db, "public", name, pk)
	if err != nil {
		t.Fatalf("tableNaturalKeyCandidates() error = %v", err)
	}
	if len(candidates) != 1 || len(candidates[0].Columns) != 1 || candidates[0].Columns[0] != "slug" {
		t.Fatalf("tableNaturalKeyCandidates() = %+v, want exactly [{Columns:[slug]}]", candidates)
	}
}

// TestTableNaturalKeyCandidatesNoCandidatesMarshalsAsEmptyArray reproduces
// the bug reported against kb.images: a table with no UNIQUE constraint
// beyond its primary key must still marshal natural_key_candidates as `[]`,
// not `null` -- the frontend's TableSchemaInfo.natural_key_candidates type
// is never-null, and calls naturalKeyCandidates.length on it unconditionally
// (sync-data-view.svelte's onTableChange and Natural Key picker). A `null`
// response throws a TypeError mid-render, which aborts that Svelte update
// cycle and leaves the Cursor Column / File Column pickers stuck disabled
// too, even though their own data (tableColumns) loaded fine.
func TestTableNaturalKeyCandidatesNoCandidatesMarshalsAsEmptyArray(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("nonulljson")
	// A table with only a primary key -- no other UNIQUE constraint -- is
	// exactly kb.images' shape.
	_, err := db.Exec(fmt.Sprintf(`
		CREATE TABLE public.%s (
			id    BIGSERIAL PRIMARY KEY,
			label TEXT NOT NULL
		)`, name))
	if err != nil {
		t.Fatalf("create scratch table %s: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP TABLE IF EXISTS public." + name); err != nil {
			t.Logf("cleanup: drop table public.%s: %v", name, err)
		}
	})

	pk, err := tablePrimaryKey(ctx, db, "public", name)
	if err != nil {
		t.Fatalf("tablePrimaryKey() error = %v", err)
	}
	candidates, err := tableNaturalKeyCandidates(ctx, db, "public", name, pk)
	if err != nil {
		t.Fatalf("tableNaturalKeyCandidates() error = %v", err)
	}
	if candidates == nil {
		t.Fatalf("tableNaturalKeyCandidates() = nil, want a non-nil empty slice (marshals as `null`, not `[]`)")
	}

	b, err := json.Marshal(tableSchemaInfo{NaturalKeyCandidates: candidates})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(b), `"natural_key_candidates":[]`) {
		t.Fatalf("json.Marshal() = %s, want natural_key_candidates to serialize as [] not null", b)
	}
}

func TestResolveItemShapeDerivesColumnsAndValidates(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("resolve")
	scratchIntrospectTable(t, db, "public."+name)

	item, err := resolveItemShape(ctx, db, "test_resolve", syncItemRequest{
		Kind:       "table",
		Table:      "public." + name,
		CursorCol:  "update_time",
		NaturalKey: []string{"slug"},
	})
	if err != nil {
		t.Fatalf("resolveItemShape() error = %v", err)
	}
	if len(item.Columns) != 4 { // every column except the surrogate id
		t.Fatalf("Columns = %v, want 4 columns (everything but id)", item.Columns)
	}
	for _, col := range item.Columns {
		if col == "id" {
			t.Fatalf("Columns = %v, must not include the surrogate id", item.Columns)
		}
	}
	if len(item.JSONColumns) != 1 || item.JSONColumns[0] != "payload" {
		t.Fatalf("JSONColumns = %v, want [payload] (auto-derived from data_type=jsonb)", item.JSONColumns)
	}
}

func TestResolveItemShapeRejectsNonTimestampCursor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("badcursor")
	scratchIntrospectTable(t, db, "public."+name)

	_, err := resolveItemShape(ctx, db, "x", syncItemRequest{
		Kind:       "table",
		Table:      "public." + name,
		CursorCol:  "label", // a TEXT column, not a timestamp
		NaturalKey: []string{"slug"},
	})
	if err == nil {
		t.Fatalf("resolveItemShape() error = nil, want an error for a non-timestamp cursor column")
	}
}

func TestResolveItemShapeRejectsPrimaryKeyAsNaturalKey(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("pknk")
	scratchIntrospectTable(t, db, "public."+name)

	_, err := resolveItemShape(ctx, db, "x", syncItemRequest{
		Kind:       "table",
		Table:      "public." + name,
		CursorCol:  "update_time",
		NaturalKey: []string{"id"}, // the surrogate primary key -- must never be accepted
	})
	if err == nil {
		t.Fatalf("resolveItemShape() error = nil, want an error when natural_key is the primary key")
	}
}

func TestResolveItemShapeRejectsNonUniqueNaturalKey(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("nonuniquenk")
	scratchIntrospectTable(t, db, "public."+name)

	_, err := resolveItemShape(ctx, db, "x", syncItemRequest{
		Kind:       "table",
		Table:      "public." + name,
		CursorCol:  "update_time",
		NaturalKey: []string{"label"}, // a real column, but not backed by any UNIQUE constraint
	})
	if err == nil {
		t.Fatalf("resolveItemShape() error = nil, want an error when natural_key has no matching unique constraint")
	}
}

func TestResolveItemShapeRejectsUnknownTable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := resolveItemShape(ctx, db, "x", syncItemRequest{
		Kind:       "table",
		Table:      "public.does_not_exist_table_xyz",
		CursorCol:  "update_time",
		NaturalKey: []string{"slug"},
	})
	if err == nil {
		t.Fatalf("resolveItemShape() error = nil, want an error for an unknown table")
	}
}

func TestResolveItemShapeTableWithFilesValidatesFileColumn(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := scratchTableName("filecol")
	scratchIntrospectTable(t, db, "public."+name)

	// A file column that doesn't exist is rejected.
	_, err := resolveItemShape(ctx, db, "x", syncItemRequest{
		Kind:                 "table_with_files",
		Table:                "public." + name,
		CursorCol:            "update_time",
		NaturalKey:           []string{"slug"},
		FileColumn:           "not_a_real_column",
		FileDirDefaultSubdir: "Stuff",
	})
	if err == nil {
		t.Fatalf("resolveItemShape() error = nil, want an error for an unknown file column")
	}

	// A real file column is accepted and shows up in the derived Columns.
	item, err := resolveItemShape(ctx, db, "x", syncItemRequest{
		Kind:                 "table_with_files",
		Table:                "public." + name,
		CursorCol:            "update_time",
		NaturalKey:           []string{"slug"},
		FileColumn:           "label",
		FileDirDefaultSubdir: "Stuff",
	})
	if err != nil {
		t.Fatalf("resolveItemShape() error = %v", err)
	}
	if item.FileColumn != "label" {
		t.Fatalf("FileColumn = %q, want %q", item.FileColumn, "label")
	}
}
