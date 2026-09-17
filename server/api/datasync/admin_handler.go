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

// originCompiled marks a syncItemView backed by the compiled Registry, which
// has no SyncOrigin of its own (it isn't a kb.data_sync_items row at all).
const originCompiled = "compiled"

// syncItemView carries an item's full shape (not just its sync status) so
// the admin UI can pre-fill the New Data Syncher form when editing --
// there's no separate single-item endpoint, so the list response is the
// only place this data can come from.
type syncItemView struct {
	ItemID               string   `json:"item_id"`
	Table                string   `json:"table"`
	Kind                 string   `json:"kind"`
	Origin               string   `json:"origin"`
	CursorCol            string   `json:"cursor_col"`
	NaturalKey           []string `json:"natural_key"`
	Columns              []string `json:"columns"`
	JSONColumns          []string `json:"json_columns"`
	Filter               string   `json:"filter,omitempty"`
	FileColumn           string   `json:"file_column,omitempty"`
	FileDirEnv           string   `json:"file_dir_env,omitempty"`
	FileDirDefaultSubdir string   `json:"file_dir_default_subdir,omitempty"`
	LastSyncedAt         string   `json:"last_synced_at,omitempty"`
	LastRowCount         int      `json:"last_row_count"`
	LastError            string   `json:"last_error,omitempty"`
}

func toSyncItemView(item TableSyncItem, origin string, state SyncState) syncItemView {
	return syncItemView{
		ItemID:               item.ID,
		Table:                item.Table,
		Kind:                 kindOrDefault(item.Kind),
		Origin:               origin,
		CursorCol:            item.CursorCol,
		NaturalKey:           item.NaturalKey,
		Columns:              item.Columns,
		JSONColumns:          item.JSONColumns,
		Filter:               item.Filter,
		FileColumn:           item.FileColumn,
		FileDirEnv:           item.FileDirEnv,
		FileDirDefaultSubdir: item.FileDirDefaultSubdir,
		LastSyncedAt:         state.LastSyncedAt.String,
		LastRowCount:         state.LastRowCount,
		LastError:            state.LastError.String,
	}
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

// HandleListSyncItems serves GET /api/v1/data-sync/items. Before listing, it
// best-effort refreshes this instance's cache of items its configured
// source advertises (design.md Decision 4) -- a source-unreachable failure
// here just means the list falls back to what's already known, it doesn't
// fail the request, since previewing/syncing an already-known item doesn't
// depend on discovery succeeding right now.
func HandleListSyncItems(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	if sourceURL, secret, err := sourceConfig(); err == nil {
		learned, fetchErr := fetchSourceItemDefinitions(ctx, sourceURL, secret)
		if fetchErr != nil {
			adminLogger.Warn("could not refresh learned sync items from source", "error", fetchErr)
		}
		for _, item := range learned {
			if cacheErr := upsertLearnedItem(ctx, db, item); cacheErr != nil {
				adminLogger.Error("failed to cache learned sync item", "item_id", item.ID, "error", cacheErr)
			}
		}
	}

	dbItems, err := dbListItems(ctx, db)
	if err != nil {
		adminLogger.Error("failed to list local sync items", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list sync items"})
	}

	views := make([]syncItemView, 0, len(Registry)+len(dbItems))
	for _, item := range Registry {
		state, err := getSyncState(ctx, db, item.ID)
		if err != nil {
			adminLogger.Error("failed to load sync state", "item_id", item.ID, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load sync state"})
		}
		views = append(views, toSyncItemView(item, originCompiled, state))
	}
	for _, item := range dbItems {
		state, err := getSyncState(ctx, db, item.ID)
		if err != nil {
			adminLogger.Error("failed to load sync state", "item_id", item.ID, "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load sync state"})
		}
		views = append(views, toSyncItemView(item, string(item.Origin), state))
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true, "items": views})
}

// HandlePreviewSync serves POST /api/v1/data-sync/items/:itemId/preview. It
// fetches changes from the source but never writes to the target database
// and never advances the stored cursor.
func HandlePreviewSync(c echo.Context) error {
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	item, ok, err := ResolveItem(c.Request().Context(), ApiTypes.ProjectDBHandle, c.Param("itemId"))
	if err != nil {
		adminLogger.Error("failed to resolve sync item", "item_id", c.Param("itemId"), "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to resolve sync item"})
	}
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "unknown sync item"})
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
	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	db := ApiTypes.ProjectDBHandle
	ctx := c.Request().Context()

	item, ok, err := ResolveItem(ctx, db, c.Param("itemId"))
	if err != nil {
		adminLogger.Error("failed to resolve sync item", "item_id", c.Param("itemId"), "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to resolve sync item"})
	}
	if !ok {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "unknown sync item"})
	}
	sourceURL, secret, err := sourceConfig()
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": err.Error()})
	}

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

	if item.Kind == KindTableWithFiles {
		if err := materializeFiles(ctx, item, sourceURL, secret, rows); err != nil {
			adminLogger.Error("apply file materialization failed", "item_id", item.ID, "error", err)
			_ = saveSyncError(ctx, db, item.ID, err.Error())
			return c.JSON(http.StatusBadGateway, map[string]any{"ok": false, "message": err.Error()})
		}
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
