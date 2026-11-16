package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

// upsertRows applies rows to item's table by NaturalKey: inserting new rows,
// updating existing ones, and never deleting anything (see the "Target
// incremental apply" requirement in production-data-sync).
func upsertRows(ctx context.Context, db *sql.DB, item TableSyncItem, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}

	placeholders := make([]string, len(item.Columns))
	for i := range item.Columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	var (
		setClauses []string
	)
	updateColumnCount := 0
	for _, col := range item.Columns {
		if item.isNaturalKeyColumn(col) {
			continue
		}
		updateColumnCount++
		param := updateColumnCount
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, param))
	}

	keyConditions := make([]string, len(item.NaturalKey))
	for i, col := range item.NaturalKey {
		keyConditions[i] = fmt.Sprintf("%s = $%d", col, updateColumnCount+i+1)
	}
	cursorParam := updateColumnCount + len(item.NaturalKey) + 1
	updateQuery := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s AND %s < $%d",
		item.Table,
		strings.Join(setClauses, ", "),
		strings.Join(keyConditions, " AND "),
		item.CursorCol,
		cursorParam,
	)

	insertQuery := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
		item.Table,
		strings.Join(item.Columns, ", "),
		strings.Join(placeholders, ", "),
	)

	queryArgs := func(item TableSyncItem, row Row) ([]any, error) {
		args, err := rowArgs(item, row)
		if err != nil {
			return nil, err
		}
		updateColumnArgs := make([]any, 0, updateColumnCount)
		naturalKeyArgs := make([]any, 0, len(item.NaturalKey))
		var cursorArg any
		argsByColumn := make(map[string]any, len(item.Columns))
		for i, col := range item.Columns {
			argsByColumn[col] = args[i]
			if item.isNaturalKeyColumn(col) {
				continue
			}
			updateColumnArgs = append(updateColumnArgs, args[i])
			if col == item.CursorCol {
				cursorArg = args[i]
			}
		}
		for _, col := range item.NaturalKey {
			naturalKeyArgs = append(naturalKeyArgs, argsByColumn[col])
		}
		updateArgs := append(append([]any{}, updateColumnArgs...), naturalKeyArgs...)
		updateArgs = append(updateArgs, cursorArg)
		return updateArgs, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert transaction for %s: %w", item.ID, err)
	}
	defer tx.Rollback()

	// A page is applied in cursor order, which need not put a row after the row it
	// points at (a concept merged into another concept, say). Constraints declared
	// DEFERRABLE are therefore checked at commit; others are unaffected.
	if _, err := tx.ExecContext(ctx, "SET CONSTRAINTS ALL DEFERRED"); err != nil {
		return fmt.Errorf("defer constraints for %s: %w", item.ID, err)
	}

	// Keep the single-row path lightweight. Larger pulls use COPY and two
	// set-based writes instead of one or two database round trips per row.
	if len(rows) > 1 {
		ordered, err := requiresOrderedApply(ctx, tx, item)
		if err != nil {
			return fmt.Errorf("check sync ordering for %s: %w", item.ID, err)
		}
		if !ordered {
			if err := bulkUpsertRows(ctx, tx, item, rows); err != nil {
				return err
			}
			return tx.Commit()
		}
	}

	for _, sourceRow := range rows {
		row, err := resolveReferences(ctx, tx, item, sourceRow)
		if err != nil {
			return err
		}
		args, err := rowArgs(item, row)
		if err != nil {
			return fmt.Errorf("build upsert args for %s: %w", item.ID, err)
		}
		result, err := tx.ExecContext(ctx, insertQuery, args...)
		if err != nil {
			return fmt.Errorf("upsert row for %s: %w", item.ID, err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect upsert result for %s: %w", item.ID, err)
		}
		if inserted == 0 {
			updateArgs, err := queryArgs(item, row)
			if err != nil {
				return fmt.Errorf("build update args for %s: %w", item.ID, err)
			}
			if _, err := tx.ExecContext(ctx, updateQuery, updateArgs...); err != nil {
				return fmt.Errorf("update existing row for %s: %w", item.ID, err)
			}
		}
	}

	return tx.Commit()
}

// resolveReferences replaces each reference column's natural-key array (as
// sent by referenceSelectExpr) with this instance's surrogate id for that
// parent row, looked up inside tx. It returns a copy and leaves row untouched.
func resolveReferences(ctx context.Context, tx *sql.Tx, item TableSyncItem, row Row) (Row, error) {
	if len(item.References) == 0 {
		return row, nil
	}
	resolved := make(Row, len(row))
	for col, raw := range row {
		resolved[col] = raw
	}
	for _, ref := range item.References {
		raw, ok := row[ref.Column]
		if !ok || isNullJSON(raw) {
			continue
		}
		parent, ok := ItemByID(ref.ItemID)
		if !ok {
			return nil, fmt.Errorf("reference %s.%s: parent item %q is not a compiled item", item.ID, ref.Column, ref.ItemID)
		}
		var key []string
		if err := json.Unmarshal(raw, &key); err != nil {
			return nil, fmt.Errorf("decode %s.%s natural key: %w", item.ID, ref.Column, err)
		}
		if len(key) != len(parent.NaturalKey) {
			return nil, fmt.Errorf("%s.%s: natural key has %d values, want %d", item.ID, ref.Column, len(key), len(parent.NaturalKey))
		}
		conditions := make([]string, len(parent.NaturalKey))
		args := make([]any, len(key))
		for i, col := range parent.NaturalKey {
			conditions[i] = fmt.Sprintf("%s = $%d", col, i+1)
			args[i] = key[i]
		}
		var id int64
		query := fmt.Sprintf("SELECT id FROM %s WHERE %s", parent.Table, strings.Join(conditions, " AND "))
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, fmt.Errorf("%s.%s: no %s row for %v; sync %s first", item.ID, ref.Column, parent.ID, key, parent.ID)
			}
			return nil, fmt.Errorf("resolve %s.%s: %w", item.ID, ref.Column, err)
		}
		encoded, err := json.Marshal(strconv.FormatInt(id, 10))
		if err != nil {
			return nil, err
		}
		resolved[ref.Column] = encoded
	}
	return resolved, nil
}

// rowArgs decodes one Row into the []any bind parameters expected by the
// INSERT built in upsertRows, in item.Columns order.
func rowArgs(item TableSyncItem, row Row) ([]any, error) {
	args := make([]any, len(item.Columns))
	for i, col := range item.Columns {
		raw, ok := row[col]
		if item.isJSONColumn(col) {
			// An absent key is SQL NULL; the literal null is the jsonb value
			// `null`, a different value that must survive the transfer.
			// Postgres accepts JSON text for a jsonb parameter directly.
			if !ok || len(raw) == 0 {
				args[i] = nil
			} else {
				args[i] = string(raw)
			}
			continue
		}
		if !ok || isNullJSON(raw) {
			args[i] = nil
			continue
		}
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil, fmt.Errorf("column %s: %w", col, err)
		}
		args[i] = decoded
	}
	return args, nil
}

// bulkUpsertRows stages values with their real PostgreSQL types. COPY preserves
// the distinction between SQL NULL and a JSON column containing literal null.
// The staging table has no constraints/defaults: only the final target writes
// run its triggers, unique constraints and foreign-key checks.
func bulkUpsertRows(ctx context.Context, tx *sql.Tx, item TableSyncItem, rows []Row) error {
	const stage = "_datasync_rows"
	columns := strings.Join(item.Columns, ", ")
	if _, err := tx.ExecContext(ctx, "CREATE TEMP TABLE "+stage+" ON COMMIT DROP AS SELECT "+columns+" FROM "+item.Table+" WITH NO DATA"); err != nil {
		return fmt.Errorf("create sync staging table for %s: %w", item.ID, err)
	}
	// Resolve before entering COPY mode: lib/pq cannot execute a lookup while
	// its connection is receiving a COPY stream.
	resolvedRows := make([]Row, len(rows))
	referenceCache := make(map[string]json.RawMessage)
	for i, row := range rows {
		resolved, err := resolveReferencesCached(ctx, tx, item, row, referenceCache)
		if err != nil {
			return err
		}
		resolvedRows[i] = resolved
	}
	stmt, err := tx.PrepareContext(ctx, pq.CopyIn(stage, item.Columns...))
	if err != nil {
		return fmt.Errorf("prepare sync copy for %s: %w", item.ID, err)
	}
	defer stmt.Close()
	for _, row := range resolvedRows {
		args, err := rowArgs(item, row)
		if err != nil {
			return fmt.Errorf("build sync copy args for %s: %w", item.ID, err)
		}
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			return fmt.Errorf("copy rows for %s: %w", item.ID, err)
		}
	}
	if _, err := stmt.ExecContext(ctx); err != nil {
		return fmt.Errorf("flush sync copy for %s: %w", item.ID, err)
	}
	if err := stmt.Close(); err != nil {
		return err
	}
	// DO NOTHING intentionally covers *all* unique constraints, including a
	// pre-existing logical row whose portable UUID differs between deployments.
	insert := "INSERT INTO " + item.Table + " (" + columns + ") SELECT " + columns + " FROM " + stage + " WHERE true ON CONFLICT DO NOTHING"
	if _, err := tx.ExecContext(ctx, insert); err != nil {
		return fmt.Errorf("bulk insert for %s: %w", item.ID, err)
	}
	var sets, keys []string
	for _, col := range item.Columns {
		if !item.isNaturalKeyColumn(col) {
			sets = append(sets, col+" = source."+col)
		}
	}
	for _, col := range item.NaturalKey {
		keys = append(keys, "target."+col+" = source."+col)
	}
	update := "UPDATE " + item.Table + " AS target SET " + strings.Join(sets, ", ") + " FROM " + stage + " AS source WHERE " + strings.Join(keys, " AND ") + " AND target." + item.CursorCol + " < source." + item.CursorCol
	if _, err := tx.ExecContext(ctx, update); err != nil {
		return fmt.Errorf("bulk update for %s: %w", item.ID, err)
	}
	return nil
}

// Cache within one apply only. A frequently referenced parent needs one lookup,
// while missing-parent errors still abort the transaction and keep the cursor.
func resolveReferencesCached(ctx context.Context, tx *sql.Tx, item TableSyncItem, row Row, cache map[string]json.RawMessage) (Row, error) {
	if len(item.References) == 0 {
		return row, nil
	}
	resolved := make(Row, len(row))
	for col, raw := range row {
		resolved[col] = raw
	}
	for _, ref := range item.References {
		raw, ok := row[ref.Column]
		if !ok || isNullJSON(raw) {
			continue
		}
		key := ref.ItemID + "\x00" + string(raw)
		if value, ok := cache[key]; ok {
			resolved[ref.Column] = value
			continue
		}
		single := item
		single.References = []SyncReference{ref}
		result, err := resolveReferences(ctx, tx, single, resolved)
		if err != nil {
			return nil, err
		}
		resolved[ref.Column] = result[ref.Column]
		cache[key] = result[ref.Column]
	}
	return resolved, nil
}

// A prior UPDATE may free a unique value needed by the next INSERT (notably
// deactivating an ontology release before activating its replacement). Keep
// cursor order for partial/expression unique indexes and unique indexes on
// transferred, mutable columns. Surrogate primary keys are not transferred and
// do not require the ordered path. This also covers runtime-defined sync items.
func requiresOrderedApply(ctx context.Context, tx *sql.Tx, item TableSyncItem) (bool, error) {
	for _, ref := range item.References {
		if parent, ok := ItemByID(ref.ItemID); ok && parent.Table == item.Table {
			return true, nil
		}
	}
	var ordered bool
	err := tx.QueryRowContext(ctx, `
  SELECT EXISTS (
   SELECT 1 FROM pg_index i
   WHERE i.indrelid = $1::regclass AND i.indisunique
    AND (i.indpred IS NOT NULL OR i.indexprs IS NOT NULL OR EXISTS (
     SELECT 1 FROM unnest(i.indkey::smallint[]) WITH ORDINALITY AS k(attnum, pos)
     JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
     WHERE k.pos <= i.indnkeyatts AND a.attname = ANY($2)
      AND NOT (a.attname = ANY($3))
    ))
  )`, item.Table, pq.Array(item.Columns), pq.Array(item.NaturalKey)).Scan(&ordered)
	return ordered, err
}
