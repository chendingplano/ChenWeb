package datasync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// syncItemRequest is the New Data Syncher form's wire shape, for both create
// (POST) and edit (PUT). Deliberately has no Columns/JSONColumns fields --
// the admin picks a real table (Table) and picks CursorCol/NaturalKey/
// FileColumn from that table's real columns/constraints (schema_introspect.go
// backs those pickers); Columns and JSONColumns are always derived
// server-side from live introspection at create/edit time, per the admin's
// own request that "the syncher should always copy all columns."
type syncItemRequest struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Table                string   `json:"table"`
	CursorCol            string   `json:"cursor_col"`
	NaturalKey           []string `json:"natural_key"`
	Filter               string   `json:"filter"`
	FileColumn           string   `json:"file_column"`
	FileDirEnv           string   `json:"file_dir_env"`
	FileDirDefaultSubdir string   `json:"file_dir_default_subdir"`
}

// timestampDataTypes are the information_schema.columns.data_type values
// Postgres reports for a TIMESTAMPTZ or TIMESTAMP column -- CursorCol must
// be one of these, since the pull/apply pipeline treats its value as a
// timestamp (the "-infinity" sentinel for an empty cursor, etc.).
var timestampDataTypes = map[string]bool{
	"timestamp with time zone":    true,
	"timestamp without time zone": true,
}

// resolveItemShape validates req and derives the full TableSyncItem shape
// (Columns/JSONColumns auto-populated) by introspecting req.Table live,
// rather than trusting anything the client says about the table's columns.
// Also validates that CursorCol is a real timestamp column and NaturalKey
// exactly matches one of the table's own non-primary-key unique
// constraints -- never the primary key itself (see schema_introspect.go's
// top comment for why).
func resolveItemShape(ctx context.Context, db *sql.DB, id string, req syncItemRequest) (TableSyncItem, error) {
	item := TableSyncItem{
		ID:                   id,
		Kind:                 SyncKind(req.Kind),
		Table:                req.Table,
		CursorCol:            req.CursorCol,
		NaturalKey:           req.NaturalKey,
		Filter:               req.Filter,
		FileColumn:           req.FileColumn,
		FileDirEnv:           req.FileDirEnv,
		FileDirDefaultSubdir: req.FileDirDefaultSubdir,
	}
	if err := validateTable(item.Table); err != nil {
		return TableSyncItem{}, err
	}
	schema, table, _ := strings.Cut(item.Table, ".")

	columns, err := tableColumns(ctx, db, schema, table)
	if err != nil {
		return TableSyncItem{}, fmt.Errorf("introspect table: %w", err)
	}
	if len(columns) == 0 {
		return TableSyncItem{}, fmt.Errorf("table %q not found", item.Table)
	}
	pk, err := tablePrimaryKey(ctx, db, schema, table)
	if err != nil {
		return TableSyncItem{}, fmt.Errorf("introspect primary key: %w", err)
	}
	candidates, err := tableNaturalKeyCandidates(ctx, db, schema, table, pk)
	if err != nil {
		return TableSyncItem{}, fmt.Errorf("introspect unique constraints: %w", err)
	}

	byName := make(map[string]columnInfo, len(columns))
	pkSet := make(map[string]bool, len(pk))
	for _, col := range pk {
		pkSet[col] = true
	}
	for _, col := range columns {
		byName[col.Name] = col
		if !pkSet[col.Name] {
			item.Columns = append(item.Columns, col.Name)
			if col.DataType == "jsonb" {
				item.JSONColumns = append(item.JSONColumns, col.Name)
			}
		}
	}

	cursorCol, ok := byName[item.CursorCol]
	if !ok {
		return TableSyncItem{}, fmt.Errorf("cursor column %q is not a real column of %s", item.CursorCol, item.Table)
	}
	if !timestampDataTypes[cursorCol.DataType] {
		return TableSyncItem{}, fmt.Errorf("cursor column %q must be a timestamp column (found type %q)", item.CursorCol, cursorCol.DataType)
	}

	matched := false
	for _, cand := range candidates {
		if columnSetEqual(cand.Columns, item.NaturalKey) {
			matched = true
			break
		}
	}
	if !matched {
		return TableSyncItem{}, fmt.Errorf(
			"natural key %v does not match any unique constraint on %s other than its primary key -- add one via a migration first",
			item.NaturalKey, item.Table)
	}

	if item.Kind == KindTableWithFiles {
		if _, ok := byName[item.FileColumn]; !ok {
			return TableSyncItem{}, fmt.Errorf("file column %q is not a real column of %s", item.FileColumn, item.Table)
		}
	}

	return item, nil
}

func columnSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA, sortedB := append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(sortedA)
	sort.Strings(sortedB)
	for i := range sortedA {
		if sortedA[i] != sortedB[i] {
			return false
		}
	}
	return true
}

func currentUserEmail(c echo.Context) string {
	rc := EchoFactory.NewFromEcho(c, "CWB_DSYNC_503")
	defer rc.Close()
	if u := rc.IsAuthenticated(); u != nil {
		return strings.TrimSpace(u.Email)
	}
	return ""
}

// HandleCreateSyncItem serves POST /api/v1/data-sync/items -- the "New Data
// Syncher" button's backend. Always creates an origin=local item.
func HandleCreateSyncItem(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	var req syncItemRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid request body"})
	}
	req.ID = strings.TrimSpace(req.ID)
	ctx := c.Request().Context()
	item, err := resolveItemShape(ctx, ApiTypes.ProjectDBHandle, req.ID, req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	if err := validateItem(item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}

	if err := insertItem(ctx, ApiTypes.ProjectDBHandle, item, currentUserEmail(c)); err != nil {
		if errors.Is(err, errIDExists) {
			return c.JSON(http.StatusConflict, map[string]any{"ok": false, "message": err.Error()})
		}
		adminLogger.Error("failed to create sync item", "item_id", item.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to create sync item"})
	}

	item.Origin = OriginLocal
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "item": toSyncItemView(item, string(OriginLocal), SyncState{})})
}

// HandleUpdateSyncItem serves PUT /api/v1/data-sync/items/:itemId. Only
// succeeds for an existing origin=local item (design.md Decision 6);
// replacing its shape resets its sync state so a stale cursor is never
// evaluated against a changed shape.
func HandleUpdateSyncItem(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	id := c.Param("itemId")
	var req syncItemRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "invalid request body"})
	}
	ctx := c.Request().Context()
	item, err := resolveItemShape(ctx, ApiTypes.ProjectDBHandle, id, req)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}
	if err := validateItem(item); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
	}

	if err := updateItem(ctx, ApiTypes.ProjectDBHandle, id, item); err != nil {
		switch {
		case errors.Is(err, errItemNotFound):
			return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": err.Error()})
		case errors.Is(err, errCompiledItem), errors.Is(err, errLearnedItem):
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		default:
			adminLogger.Error("failed to update sync item", "item_id", id, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to update sync item"})
		}
	}

	item.Origin = OriginLocal
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "item": toSyncItemView(item, string(OriginLocal), SyncState{})})
}

// HandleDeleteSyncItem serves DELETE /api/v1/data-sync/items/:itemId. Works
// for both origin=local and origin=learned items (deleting a learned item
// only clears this instance's local cache row) but not a compiled item.
func HandleDeleteSyncItem(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	id := c.Param("itemId")
	ctx := c.Request().Context()
	if err := deleteItem(ctx, ApiTypes.ProjectDBHandle, id); err != nil {
		switch {
		case errors.Is(err, errItemNotFound):
			return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": err.Error()})
		case errors.Is(err, errCompiledItem):
			return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": err.Error()})
		default:
			adminLogger.Error("failed to delete sync item", "item_id", id, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to delete sync item"})
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}
