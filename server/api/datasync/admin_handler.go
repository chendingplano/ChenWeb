package datasync

import (
	"net/http"
	"os"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var adminLogger = loggerutil.CreateDefaultLogger("CWB_DSYNC_502")

type syncItemView struct {
	ItemID       string `json:"item_id"`
	Table        string `json:"table"`
	LastSyncedAt string `json:"last_synced_at,omitempty"`
	LastRowCount int    `json:"last_row_count"`
	LastError    string `json:"last_error,omitempty"`
}

// sourceConfig reads the target-side env vars needed to reach the sync
// source. It fails closed (an error, not a silently-disabled feature) if
// either is unset, matching how the source's own pull handler fails closed
// when its shared secret is unset.
func sourceConfig() (sourceURL, sharedSecret string, err error) {
	sourceURL = strings.TrimSpace(os.Getenv("DATA_SYNC_SOURCE_URL"))
	sharedSecret = strings.TrimSpace(os.Getenv("DATA_SYNC_SHARED_SECRET"))
	if sourceURL == "" || sharedSecret == "" {
		return "", "", errMissingConfig
	}
	return sourceURL, sharedSecret, nil
}

var errMissingConfig = &configError{"DATA_SYNC_SOURCE_URL and DATA_SYNC_SHARED_SECRET must both be set"}

type configError struct{ msg string }

func (e *configError) Error() string { return e.msg }

// HandleListSyncItems serves GET /api/v1/data-sync/items.
func HandleListSyncItems(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	ctx := c.Request().Context()
	views := make([]syncItemView, 0, len(Registry))
	for _, item := range Registry {
		state, err := getSyncState(ctx, ApiTypes.ProjectDBHandle, item.ID)
		if err != nil {
			adminLogger.Error("failed to load sync state", "item_id", item.ID, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load sync state"})
		}
		views = append(views, syncItemView{
			ItemID:       item.ID,
			Table:        item.Table,
			LastSyncedAt: state.LastSyncedAt.String,
			LastRowCount: state.LastRowCount,
			LastError:    state.LastError.String,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "items": views})
}

// HandlePreviewSync serves POST /api/v1/data-sync/items/:itemId/preview. It
// fetches changes from the source but never writes to the target database
// and never advances the stored cursor.
func HandlePreviewSync(c echo.Context) error {
	item, ok := ItemByID(c.Param("itemId"))
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "unknown sync item"})
	}
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	sourceURL, secret, err := sourceConfig()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": err.Error()})
	}

	ctx := c.Request().Context()
	state, err := getSyncState(ctx, ApiTypes.ProjectDBHandle, item.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load sync state"})
	}

	rows, _, err := fetchAllChanges(ctx, sourceURL, secret, item, state.LastCursor.String)
	if err != nil {
		adminLogger.Error("preview fetch failed", "item_id", item.ID, "error", err)
		return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":                true,
		"item_id":           item.ID,
		"changed_row_count": len(rows),
	})
}

// HandleApplySync serves POST /api/v1/data-sync/items/:itemId/apply. It
// fetches changes from the source (re-reading the stored cursor itself,
// rather than trusting whatever the browser saw during preview -- mirrors
// llmadminhandler.ImportModelsTOMLApply), upserts them, and on full success
// advances the stored cursor. On any failure the stored cursor is left
// untouched so a retry re-fetches from the same starting point.
func HandleApplySync(c echo.Context) error {
	item, ok := ItemByID(c.Param("itemId"))
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "unknown sync item"})
	}
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	sourceURL, secret, err := sourceConfig()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": err.Error()})
	}

	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	state, err := getSyncState(ctx, db, item.ID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load sync state"})
	}

	rows, nextCursor, err := fetchAllChanges(ctx, sourceURL, secret, item, state.LastCursor.String)
	if err != nil {
		adminLogger.Error("apply fetch failed", "item_id", item.ID, "error", err)
		_ = saveSyncError(ctx, db, item.ID, err.Error())
		return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": err.Error()})
	}

	if err := upsertRows(ctx, db, item, rows); err != nil {
		adminLogger.Error("apply upsert failed", "item_id", item.ID, "error", err)
		_ = saveSyncError(ctx, db, item.ID, err.Error())
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": err.Error()})
	}

	finalCursor := nextCursor
	if finalCursor == "" {
		finalCursor = state.LastCursor.String
	}
	if err := saveSyncSuccess(ctx, db, item.ID, finalCursor, len(rows)); err != nil {
		adminLogger.Error("failed to persist sync state", "item_id", item.ID, "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "sync applied but failed to persist state"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":               true,
		"item_id":          item.ID,
		"synced_row_count": len(rows),
	})
}
