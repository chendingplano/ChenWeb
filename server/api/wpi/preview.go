package wpi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// PreviewList runs a validated, read-only load source from the editor without saving a page.
func (h *Handler) PreviewList(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(user) {
		return errorJSON(c, http.StatusForbidden, "Administrator access required")
	}
	start := time.Now()
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes+1))
	if err != nil || len(raw) > maxBodyBytes {
		return errorJSON(c, http.StatusBadRequest, "Invalid request body")
	}
	var req struct {
		Source map[string]any `json:"source"`
		Args   map[string]any `json:"args"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil || req.Source == nil {
		return errorJSON(c, http.StatusBadRequest, "Invalid source")
	}
	if str(req.Source["action"]) != "load" || str(req.Source["result"]) != "list" {
		return errorJSON(c, http.StatusBadRequest, "Only list load sources can be previewed")
	}
	// An unsaved source has no class copy yet; it gets one of the active class,
	// as it would when its page is written (OpenSpec wpi-pages-own-panes, D4).
	if _, has := req.Source[copyKey]; !has {
		rows, err := store{h.DB}.latest(c.Request().Context(), KindAction, []string{"load"})
		if err != nil {
			handlerLog.Error("WPI list preview class load failed", "user", actorName(user), "error", err)
			return errorJSON(c, http.StatusInternalServerError, "Unable to preview records")
		}
		row, ok := rows["load"]
		if !ok {
			return errorJSON(c, http.StatusBadRequest, `action class "load" has no active row to copy`)
		}
		req.Source[copyKey] = row.Body
	}
	v := &validator{kind: KindPage}
	pos := actionPos{val: req.Source, path: "/source", allowed: []string{"load"}, roots: []string{RootInputDoc}}
	merged := v.actionInstance(pos)
	if len(v.issues) > 0 {
		return errorJSON(c, http.StatusBadRequest, v.issues[0].Message)
	}
	body, err := json.Marshal(merged)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Invalid source")
	}
	var act Action
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&act); err != nil {
		return errorJSON(c, http.StatusBadRequest, "Invalid source")
	}
	act.Class = "load"
	result, rows, err := h.run(c.Request().Context(), act, merged, runRequest{Args: req.Args}, user)
	if err != nil {
		handlerLog.Warn("WPI list preview failed", "user", actorName(user), "table", act.Table, "error", err)
		var re *requestError
		if errors.As(err, &re) {
			return errorJSON(c, re.status, re.msg)
		}
		return errorJSON(c, http.StatusInternalServerError, "Unable to preview records")
	}
	handlerLog.Info("WPI list preview", "user", actorName(user), "table", act.Table, "rows", rows, "duration_ms", time.Since(start).Milliseconds())
	return c.JSON(http.StatusOK, result)
}
