package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// See openspec/changes/configurable-data-sync-items/design.md Decisions 1,
// 4, and 6. Compiled Registry items are never stored here; this file only
// manages kb.data_sync_items (origin=local, created via the admin UI, or
// origin=learned, write-through-cached from a configured source).

var (
	errIDExists     = errors.New("a sync item with this id already exists")
	errItemNotFound = errors.New("sync item not found")
	errCompiledItem = errors.New("this is a compiled sync item; edit registry.go and redeploy instead")
	errLearnedItem  = errors.New("this item's shape is owned by the source that advertised it; delete it if you no longer want it, but it can't be edited here")
)

// dbExecer is satisfied by both *sql.DB and *sql.Tx, so the row-writing
// helpers below work either standalone or inside a transaction.
type dbExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

const dbSyncItemColumns = `id, kind, origin, table_name, cursor_col, natural_key, columns,
	json_columns, COALESCE(filter, ''), COALESCE(file_column, ''),
	COALESCE(file_dir_env, ''), COALESCE(file_dir_default_subdir, '')`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanDBItem(row rowScanner) (TableSyncItem, error) {
	var item TableSyncItem
	var kind, origin string
	var naturalKeyJSON, columnsJSON, jsonColumnsJSON []byte
	if err := row.Scan(
		&item.ID, &kind, &origin, &item.Table, &item.CursorCol,
		&naturalKeyJSON, &columnsJSON, &jsonColumnsJSON,
		&item.Filter, &item.FileColumn, &item.FileDirEnv, &item.FileDirDefaultSubdir,
	); err != nil {
		return TableSyncItem{}, err
	}
	item.Kind = SyncKind(kind)
	item.Origin = SyncOrigin(origin)

	var err error
	if item.NaturalKey, err = unmarshalStrings(naturalKeyJSON); err != nil {
		return TableSyncItem{}, fmt.Errorf("decode natural_key for %s: %w", item.ID, err)
	}
	if item.Columns, err = unmarshalStrings(columnsJSON); err != nil {
		return TableSyncItem{}, fmt.Errorf("decode columns for %s: %w", item.ID, err)
	}
	if item.JSONColumns, err = unmarshalStrings(jsonColumnsJSON); err != nil {
		return TableSyncItem{}, fmt.Errorf("decode json_columns for %s: %w", item.ID, err)
	}
	return item, nil
}

func unmarshalStrings(raw []byte) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func marshalStrings(vals []string) ([]byte, error) {
	if vals == nil {
		vals = []string{}
	}
	return json.Marshal(vals)
}

func kindOrDefault(k SyncKind) string {
	if k == "" {
		return string(KindTable)
	}
	return string(k)
}

// dbItemByID looks up one row of kb.data_sync_items. Does not consult the
// compiled Registry -- see ResolveItem for the combined lookup.
func dbItemByID(ctx context.Context, db *sql.DB, id string) (TableSyncItem, bool, error) {
	row := db.QueryRowContext(ctx, `SELECT `+dbSyncItemColumns+` FROM kb.data_sync_items WHERE id = $1`, id)
	item, err := scanDBItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TableSyncItem{}, false, nil
	}
	if err != nil {
		return TableSyncItem{}, false, err
	}
	return item, true, nil
}

// dbListItems returns every kb.data_sync_items row (local and learned).
func dbListItems(ctx context.Context, db *sql.DB) ([]TableSyncItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+dbSyncItemColumns+` FROM kb.data_sync_items ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TableSyncItem
	for rows.Next() {
		item, err := scanDBItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// allKnownItems is the compiled Registry unioned with everything in
// kb.data_sync_items (local and learned) -- what this instance can resolve
// an itemId against, and what it advertises to a target asking what it can
// act as a source for (pull_handler.go's discovery endpoint).
func allKnownItems(ctx context.Context, db *sql.DB) ([]TableSyncItem, error) {
	dbItems, err := dbListItems(ctx, db)
	if err != nil {
		return nil, err
	}
	items := make([]TableSyncItem, 0, len(Registry)+len(dbItems))
	items = append(items, Registry...)
	items = append(items, dbItems...)
	return items, nil
}

// ResolveItem checks the compiled Registry first (zero DB cost, can't be
// shadowed by a bad DB row), then falls back to kb.data_sync_items.
func ResolveItem(ctx context.Context, db *sql.DB, id string) (TableSyncItem, bool, error) {
	if item, ok := ItemByID(id); ok {
		return item, true, nil
	}
	return dbItemByID(ctx, db, id)
}

func writeItemRow(ctx context.Context, ex dbExecer, item TableSyncItem, origin SyncOrigin, createdBy string) error {
	naturalKeyJSON, err := marshalStrings(item.NaturalKey)
	if err != nil {
		return fmt.Errorf("encode natural_key: %w", err)
	}
	columnsJSON, err := marshalStrings(item.Columns)
	if err != nil {
		return fmt.Errorf("encode columns: %w", err)
	}
	jsonColumnsJSON, err := marshalStrings(item.JSONColumns)
	if err != nil {
		return fmt.Errorf("encode json_columns: %w", err)
	}

	_, err = ex.ExecContext(ctx, `
		INSERT INTO kb.data_sync_items
			(id, kind, origin, table_name, cursor_col, natural_key, columns, json_columns,
			 filter, file_column, file_dir_env, file_dir_default_subdir, created_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())`,
		item.ID, kindOrDefault(item.Kind), string(origin), item.Table, item.CursorCol,
		naturalKeyJSON, columnsJSON, jsonColumnsJSON,
		nullIfEmpty(item.Filter), nullIfEmpty(item.FileColumn),
		nullIfEmpty(item.FileDirEnv), nullIfEmpty(item.FileDirDefaultSubdir),
		nullIfEmpty(createdBy),
	)
	return err
}

func updateItemRow(ctx context.Context, ex dbExecer, id string, item TableSyncItem, origin SyncOrigin) error {
	naturalKeyJSON, err := marshalStrings(item.NaturalKey)
	if err != nil {
		return fmt.Errorf("encode natural_key: %w", err)
	}
	columnsJSON, err := marshalStrings(item.Columns)
	if err != nil {
		return fmt.Errorf("encode columns: %w", err)
	}
	jsonColumnsJSON, err := marshalStrings(item.JSONColumns)
	if err != nil {
		return fmt.Errorf("encode json_columns: %w", err)
	}

	_, err = ex.ExecContext(ctx, `
		UPDATE kb.data_sync_items SET
			kind = $2, origin = $3, table_name = $4, cursor_col = $5,
			natural_key = $6, columns = $7, json_columns = $8,
			filter = $9, file_column = $10, file_dir_env = $11, file_dir_default_subdir = $12,
			updated_at = NOW()
		WHERE id = $1`,
		id, kindOrDefault(item.Kind), string(origin), item.Table, item.CursorCol,
		naturalKeyJSON, columnsJSON, jsonColumnsJSON,
		nullIfEmpty(item.Filter), nullIfEmpty(item.FileColumn),
		nullIfEmpty(item.FileDirEnv), nullIfEmpty(item.FileDirDefaultSubdir),
	)
	return err
}

// insertItem persists a new origin=local item. Rejects an id collision
// against either the compiled Registry or an existing kb.data_sync_items row
// before touching the database, so the admin handler can surface a specific
// "id already exists" error (the table's PRIMARY KEY still catches a
// concurrent-request race).
func insertItem(ctx context.Context, db *sql.DB, item TableSyncItem, createdBy string) error {
	if _, ok := ItemByID(item.ID); ok {
		return errIDExists
	}
	if _, ok, err := dbItemByID(ctx, db, item.ID); err != nil {
		return err
	} else if ok {
		return errIDExists
	}
	return writeItemRow(ctx, db, item, OriginLocal, createdBy)
}

// upsertLearnedItem write-through-caches an item definition learned from
// this instance's configured source (design.md Decision 4). An existing
// origin=local row always wins over a same-id item the source happens to
// advertise (Decision 6) -- silently skipped, not an error, since this runs
// on every list refresh and a same-id collision isn't the caller's fault.
func upsertLearnedItem(ctx context.Context, db *sql.DB, item TableSyncItem) error {
	if _, ok := ItemByID(item.ID); ok {
		return nil // shadowed by a compiled item; nothing to cache
	}
	existing, ok, err := dbItemByID(ctx, db, item.ID)
	if err != nil {
		return err
	}
	if ok && existing.Origin == OriginLocal {
		return nil
	}
	if !ok {
		return writeItemRow(ctx, db, item, OriginLearned, "")
	}
	return updateItemRow(ctx, db, item.ID, item, OriginLearned)
}

// updateItem replaces a local item's shape and resets its sync state in the
// same transaction (design.md Decision 6), so a stale cursor is never
// evaluated against a changed shape. Fails for a compiled id (errCompiledItem)
// or a learned item (errLearnedItem).
func updateItem(ctx context.Context, db *sql.DB, id string, item TableSyncItem) error {
	if _, ok := ItemByID(id); ok {
		return errCompiledItem
	}
	existing, ok, err := dbItemByID(ctx, db, id)
	if err != nil {
		return err
	}
	if !ok {
		return errItemNotFound
	}
	if existing.Origin != OriginLocal {
		return errLearnedItem
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := updateItemRow(ctx, tx, id, item, OriginLocal); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.data_sync_state WHERE sync_item_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteItem removes a local or learned item and its kb.data_sync_state row
// together. Fails for a compiled id (errCompiledItem).
func deleteItem(ctx context.Context, db *sql.DB, id string) error {
	if _, ok := ItemByID(id); ok {
		return errCompiledItem
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `DELETE FROM kb.data_sync_items WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errItemNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.data_sync_state WHERE sync_item_id = $1`, id); err != nil {
		return err
	}
	return tx.Commit()
}
