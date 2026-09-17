package datasync

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// HandleGetItemFile serves
// GET /api/internal/data-sync/items/:itemId/files?key=<url-encoded JSON array>.
// It streams the file referenced by one row of a table_with_files item,
// identified by the row's natural key (in the item's NaturalKey order) --
// never by a client-supplied filesystem path, so a target can never probe or
// read an arbitrary path on the source (design.md Decision 3).
func HandleGetItemFile(c echo.Context) error {
	if !authenticatePullRequest(c) {
		return nil
	}
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "project database is not initialized"})
	}

	itemID := c.Param("itemId")
	ctx := c.Request().Context()
	item, ok, err := ResolveItem(ctx, ApiTypes.ProjectDBHandle, itemID)
	if err != nil {
		pullLogger.Error("failed to resolve sync item", "item_id", itemID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to resolve sync item"})
	}
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown sync item: " + itemID})
	}
	if item.Kind != KindTableWithFiles {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "sync item is not a table_with_files kind"})
	}

	var keyValues []string
	if err := json.Unmarshal([]byte(c.QueryParam("key")), &keyValues); err != nil || len(keyValues) != len(item.NaturalKey) {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "key must be a JSON array matching the item's natural key"})
	}

	path, err := lookupItemFilePath(ApiTypes.ProjectDBHandle, item, keyValues)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "no matching row for the given key"})
	}
	if err != nil {
		pullLogger.Error("failed to look up sync item file", "item_id", itemID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to look up file"})
	}
	if path == "" {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "row has no file"})
	}

	f, err := os.Open(path)
	if err != nil {
		pullLogger.Error("sync item file missing on disk", "item_id", itemID, "path", path, "error", err)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "file missing on source"})
	}
	defer f.Close()

	c.Response().Header().Set(echo.HeaderContentDisposition, fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	return c.Stream(http.StatusOK, "application/octet-stream", f)
}

// lookupItemFilePath re-derives a row's file path server-side from its
// natural key -- the source never trusts a client-supplied path.
func lookupItemFilePath(db *sql.DB, item TableSyncItem, keyValues []string) (string, error) {
	var whereClauses []string
	if item.Filter != "" {
		whereClauses = append(whereClauses, "("+item.Filter+")")
	}
	args := make([]any, len(keyValues))
	for i, col := range item.NaturalKey {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", col, i+1))
		args[i] = keyValues[i]
	}
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s",
		item.FileColumn, item.Table, strings.Join(whereClauses, " AND "),
	)
	var path sql.NullString
	if err := db.QueryRow(query, args...).Scan(&path); err != nil {
		return "", err
	}
	return path.String, nil
}

// HandleListSourceItems serves GET /api/internal/data-sync/items -- every
// item this instance can act as a source for (compiled Registry ∪ its own
// kb.data_sync_items, local and learned), so a target can discover an item
// it doesn't already know about instead of requiring an admin to hand-
// replicate the same New Data Syncher form on every box (design.md
// Decision 4).
func HandleListSourceItems(c echo.Context) error {
	if !authenticatePullRequest(c) {
		return nil
	}
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "project database is not initialized"})
	}

	items, err := allKnownItems(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		pullLogger.Error("failed to list sync items for discovery", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to list sync items"})
	}
	defs := make([]itemDefinition, len(items))
	for i, item := range items {
		defs[i] = toItemDefinition(item)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": defs})
}
