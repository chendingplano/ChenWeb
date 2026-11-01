package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
		if !ok || isNullJSON(raw) {
			args[i] = nil
			continue
		}
		if item.isJSONColumn(col) {
			// Postgres accepts JSON text for a jsonb parameter directly.
			args[i] = string(raw)
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
