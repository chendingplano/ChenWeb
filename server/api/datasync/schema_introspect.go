package datasync

import (
	"context"
	"database/sql"
	"net/http"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// This file backs the New Data Syncher form's table/column/natural-key
// pickers (System Admin → Resources → Sync Data) with real Postgres schema
// introspection, so an admin selects a real table and real columns instead
// of typing names that might not exist. See the admin's own request: "do
// not let users enter database name and table" / "[cursor column should]
// list all the fields of the selected table" / natural key should be
// populated from the table's own unique constraints, never its primary key
// (syncing a surrogate id would corrupt data -- source and target generate
// their own ids independently, see production-data-sync/design.md
// Decision 2).

// tableRef identifies one syncable base table.
type tableRef struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
}

// columnInfo is one column of a table, as shown in the Cursor Column / File
// Column pickers.
type columnInfo struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// naturalKeyCandidate is one UNIQUE constraint's column set, excluding the
// primary key -- what the Natural Key picker offers.
type naturalKeyCandidate struct {
	Columns []string `json:"columns"`
}

type tableSchemaInfo struct {
	Columns              []columnInfo          `json:"columns"`
	PrimaryKey           []string              `json:"primary_key"`
	NaturalKeyCandidates []naturalKeyCandidate `json:"natural_key_candidates"`
	JSONColumns          []string              `json:"json_columns"`
}

// HandleListSyncableTables serves GET /api/v1/data-sync/schema/tables --
// every base table in every non-system schema, for the Table picker's first
// (schema) and second (table) dropdowns.
func HandleListSyncableTables(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	tables, err := listSyncableTables(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		adminLogger.Error("failed to list tables for sync-item schema picker", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list tables"})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "tables": tables})
}

// HandleGetTableSchema serves GET /api/v1/data-sync/schema/tables/:schema/:table
// -- columns, primary key, natural-key candidates, and which columns are
// jsonb, for whichever table the admin just selected.
func HandleGetTableSchema(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	schema, table := c.Param("schema"), c.Param("table")
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	columns, err := tableColumns(ctx, db, schema, table)
	if err != nil {
		adminLogger.Error("failed to introspect table columns", "schema", schema, "table", table, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to introspect table"})
	}
	if len(columns) == 0 {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "table not found"})
	}

	pk, err := tablePrimaryKey(ctx, db, schema, table)
	if err != nil {
		adminLogger.Error("failed to introspect primary key", "schema", schema, "table", table, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to introspect table"})
	}
	candidates, err := tableNaturalKeyCandidates(ctx, db, schema, table, pk)
	if err != nil {
		adminLogger.Error("failed to introspect unique constraints", "schema", schema, "table", table, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to introspect table"})
	}

	var jsonCols []string
	for _, col := range columns {
		if col.DataType == "jsonb" {
			jsonCols = append(jsonCols, col.Name)
		}
	}

	return c.JSON(http.StatusOK, map[string]any{"ok": true, "info": tableSchemaInfo{
		Columns:              columns,
		PrimaryKey:           pk,
		NaturalKeyCandidates: candidates,
		JSONColumns:          jsonCols,
	}})
}

func listSyncableTables(ctx context.Context, db *sql.DB) ([]tableRef, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		  AND table_schema NOT IN ('pg_catalog', 'information_schema', 'pg_toast')
		ORDER BY table_schema, table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []tableRef
	for rows.Next() {
		var t tableRef
		if err := rows.Scan(&t.Schema, &t.Table); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func tableColumns(ctx context.Context, db *sql.DB, schema, table string) ([]columnInfo, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []columnInfo
	for rows.Next() {
		var col columnInfo
		if err := rows.Scan(&col.Name, &col.DataType); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

func tablePrimaryKey(ctx context.Context, db *sql.DB, schema, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.table_schema = $1 AND tc.table_name = $2 AND tc.constraint_type = 'PRIMARY KEY'
		ORDER BY kcu.ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var col string
		if err := rows.Scan(&col); err != nil {
			return nil, err
		}
		cols = append(cols, col)
	}
	return cols, rows.Err()
}

// tableNaturalKeyCandidates returns every UNIQUE constraint's column set,
// grouped by constraint, excluding any constraint whose columns exactly
// match the primary key (a table's PK already shows up as a UNIQUE
// constraint in information_schema, and it must never be offered as a
// natural key -- see this file's top comment).
func tableNaturalKeyCandidates(ctx context.Context, db *sql.DB, schema, table string, primaryKey []string) ([]naturalKeyCandidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT tc.constraint_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		WHERE tc.table_schema = $1 AND tc.table_name = $2 AND tc.constraint_type = 'UNIQUE'
		ORDER BY tc.constraint_name, kcu.ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	order := []string{}
	byConstraint := map[string][]string{}
	for rows.Next() {
		var constraintName, col string
		if err := rows.Scan(&constraintName, &col); err != nil {
			return nil, err
		}
		if _, seen := byConstraint[constraintName]; !seen {
			order = append(order, constraintName)
		}
		byConstraint[constraintName] = append(byConstraint[constraintName], col)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	candidates := []naturalKeyCandidate{}
	for _, name := range order {
		cols := byConstraint[name]
		if stringSliceEqual(cols, primaryKey) {
			continue
		}
		candidates = append(candidates, naturalKeyCandidate{Columns: cols})
	}
	return candidates, nil
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
