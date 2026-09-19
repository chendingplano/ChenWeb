package datasync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

	for _, row := range rows {
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
