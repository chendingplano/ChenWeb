package wpi

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
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

// tableDatabase keeps unqualified legacy references in the project database.
func (h *Handler) tableDatabase(table string) (*sql.DB, string, error) {
	parts := strings.Split(table, ".")
	if len(parts) != 3 {
		return h.DB, table, nil
	}
	db := h.Databases[parts[0]]
	if db == nil {
		return nil, "", fmt.Errorf("database %q is not available", parts[0])
	}
	return db, parts[1] + "." + parts[2], nil
}

func (h *Handler) columns(ctx context.Context, table string) (map[string]bool, error) {
	db, name, err := h.tableDatabase(table)
	if err != nil {
		return nil, err
	}
	return catalogColumns(db)(ctx, name)
}

// TableCatalog lists configured databases, or the visible tables in one of them.
func (h *Handler) TableCatalog(c echo.Context) error {
	if h.User(c) == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	name := c.QueryParam("database")
	if name == "" {
		return c.JSON(http.StatusOK, map[string]any{"databases": h.DatabaseNames})
	}
	db := h.Databases[name]
	if db == nil {
		return errorJSON(c, http.StatusServiceUnavailable, "Database is not available")
	}
	rows, err := db.QueryContext(c.Request().Context(), `SELECT table_schema, table_name FROM information_schema.tables WHERE table_type = 'BASE TABLE' AND table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast') AND table_schema NOT LIKE 'pg_temp_%' AND table_schema NOT LIKE 'pg_toast_temp_%' ORDER BY table_schema, table_name`)
	if err != nil {
		handlerLog.Error("WPI table catalog failed", "database", name, "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to load database tables")
	}
	defer rows.Close()
	type table struct {
		Schema string `json:"schema"`
		Table  string `json:"table"`
	}
	tables := []table{}
	for rows.Next() {
		var item table
		if err := rows.Scan(&item.Schema, &item.Table); err != nil {
			return errorJSON(c, http.StatusInternalServerError, "Unable to load database tables")
		}
		tables = append(tables, item)
	}
	if err := rows.Err(); err != nil {
		return errorJSON(c, http.StatusInternalServerError, "Unable to load database tables")
	}
	return c.JSON(http.StatusOK, map[string]any{"tables": tables})
}
