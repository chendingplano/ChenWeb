package wpi

import (
	"context"
	"database/sql"
	"strings"

	"github.com/lib/pq"
)

// splitTable splits "schema.name" into its parts; a bare name is in public.
func splitTable(table string) (schema, name string) {
	if i := strings.IndexByte(table, '.'); i >= 0 {
		return table[:i], table[i+1:]
	}
	return "public", table
}

// quoteTable returns the table as a quoted, schema-qualified identifier.
func quoteTable(table string) string {
	schema, name := splitTable(table)
	return pq.QuoteIdentifier(schema) + "." + pq.QuoteIdentifier(name)
}

// catalogColumns looks tables up in information_schema. A table the database
// user cannot see is reported as missing, like a table that does not exist.
func catalogColumns(db *sql.DB) Columns {
	return func(ctx context.Context, table string) (map[string]bool, error) {
		schema, name := splitTable(table)
		rows, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2`, schema, name)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		cols := map[string]bool{}
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				return nil, err
			}
			cols[c] = true
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(cols) == 0 {
			return nil, nil
		}
		return cols, nil
	}
}
