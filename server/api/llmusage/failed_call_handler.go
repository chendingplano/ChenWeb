package llmusage

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ListFailedCalls returns a bounded, newest-first page of failed call records.
func ListFailedCalls(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_FLC_001")
	defer rc.Close()
	if rc.IsAuthenticated() == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"status": false})
	}
	page := 1
	if raw := c.QueryParam("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000000 {
			return c.JSON(http.StatusBadRequest, map[string]any{"status": false})
		}
		page = n
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"status": false})
	}
	rows, err := db.QueryContext(c.Request().Context(), `SELECT row_to_json(t) FROM (SELECT * FROM kb.failed_llm_calls ORDER BY id DESC LIMIT 51 OFFSET $1) t`, (page-1)*50)
	if err != nil {
		rc.GetLogger().Error("(CWB_FLC_002) failed to list failed LLM calls", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false})
	}
	defer rows.Close()
	results := make([]json.RawMessage, 0, 51)
	for rows.Next() {
		var row json.RawMessage
		if err = rows.Scan(&row); err != nil {
			break
		}
		results = append(results, row)
	}
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		rc.GetLogger().Error("(CWB_FLC_003) failed to read failed LLM calls", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false})
	}
	more := len(results) > 50
	if more {
		results = results[:50]
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": results, "more": more})
}
