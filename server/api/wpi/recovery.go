package wpi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v4"
)

// PageRecovery stores an author's unfinished editor descriptor independently of published pages.
func (h *Handler) PageRecovery(c echo.Context) error {
	u := h.User(c)
	if u == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(u) {
		return errorJSON(c, http.StatusForbidden, "Admin access required")
	}
	id := c.Param("pageId")
	if !idPattern.MatchString(id) {
		return errorJSON(c, http.StatusBadRequest, "Invalid page ID")
	}
	ctx := c.Request().Context()
	owner := actorName(u)
	if c.Request().Method == http.MethodGet {
		var raw []byte
		err := h.DB.QueryRowContext(ctx, `SELECT body FROM archive.frontend_pages WHERE descriptor_id = $1 AND owner = $2 AND recovery`, id, owner).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, map[string]any{"page": nil})
		}
		if err != nil {
			handlerLog.Error("WPI recovery read failed", "page", id, "user", owner, "error", err)
			return errorJSON(c, 500, "Unable to load recovery")
		}
		return c.JSON(http.StatusOK, map[string]any{"page": json.RawMessage(raw)})
	}
	if c.Request().Method == http.MethodDelete {
		_, err := h.DB.ExecContext(ctx, `DELETE FROM archive.frontend_pages WHERE descriptor_id = $1 AND owner = $2 AND recovery`, id, owner)
		if err != nil {
			return errorJSON(c, 500, "Unable to delete recovery")
		}
		return c.JSON(http.StatusOK, map[string]bool{"deleted": true})
	}
	var req struct {
		Version int             `json:"version"`
		Page    json.RawMessage `json:"page"`
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes+1))
	if err != nil || len(raw) > maxBodyBytes || json.Unmarshal(raw, &req) != nil || req.Version < 0 {
		return errorJSON(c, 400, "Invalid recovery")
	}
	body, err := decodeBody(req.Page)
	if err != nil || body["descriptor_id"] != id {
		return errorJSON(c, 400, "Invalid recovery page")
	}
	tx, err := h.DB.BeginTx(ctx, nil)
	if err != nil {
		return errorJSON(c, 500, "Unable to save recovery")
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, id); err != nil {
		return errorJSON(c, 500, "Unable to save recovery")
	}
	var version int
	err = tx.QueryRowContext(ctx, `SELECT version FROM kb.frontend_pages WHERE descriptor_id = $1 ORDER BY version DESC LIMIT 1`, id).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) && req.Version == 0 {
		version, err = 0, nil
	} else if errors.Is(err, sql.ErrNoRows) || (err == nil && version != req.Version) {
		return errorJSON(c, 409, "Page version changed")
	}
	if err != nil {
		return errorJSON(c, 500, "Unable to save recovery")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO archive.frontend_pages (descriptor_id, version, schema_version, body, status, recovery, owner, created_by, updated_by)
 VALUES ($1, $2, $3, $4::jsonb, 'draft', true, $5, $5, $5)
 ON CONFLICT (descriptor_id, owner) WHERE recovery DO UPDATE SET version = EXCLUDED.version, body = EXCLUDED.body, updated_at = NOW()`, id, version, schemaVersionOf(KindPage), string(req.Page), owner)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		handlerLog.Error("WPI recovery save failed", "page", id, "user", owner, "error", err)
		return errorJSON(c, 500, "Unable to save recovery")
	}
	handlerLog.Info("WPI recovery saved", "page", id, "user", owner, "version", version)
	return c.JSON(http.StatusOK, map[string]bool{"saved": true})
}

func (h *Handler) ListUnsavedPageRecoveries(c echo.Context) error {
	u := h.User(c)
	if u == nil {
		return errorJSON(c, 401, "Authentication required")
	}
	if !isAdmin(u) {
		return errorJSON(c, 403, "Admin access required")
	}
	rows, err := h.DB.QueryContext(c.Request().Context(), `SELECT descriptor_id, body FROM archive.frontend_pages WHERE recovery AND owner = $1 AND version = 0 ORDER BY updated_at`, actorName(u))
	if err != nil {
		return errorJSON(c, 500, "Unable to load recoveries")
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return errorJSON(c, 500, "Unable to load recoveries")
		}
		items = append(items, map[string]any{"id": id, "page": json.RawMessage(raw)})
	}
	if rows.Err() != nil {
		return errorJSON(c, 500, "Unable to load recoveries")
	}
	return c.JSON(200, map[string]any{"recoveries": items})
}
