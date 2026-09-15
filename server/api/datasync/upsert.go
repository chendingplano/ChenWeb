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

	var setClauses []string
	for _, col := range item.Columns {
		if item.isNaturalKeyColumn(col) {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) ON CONFLICT (%s) DO UPDATE SET %s",
		item.Table,
		strings.Join(item.Columns, ", "),
		strings.Join(placeholders, ", "),
		strings.Join(item.NaturalKey, ", "),
		strings.Join(setClauses, ", "),
	)

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
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("upsert row for %s: %w", item.ID, err)
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
