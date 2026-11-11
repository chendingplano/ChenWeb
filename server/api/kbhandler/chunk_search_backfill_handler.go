package kbhandler

import (
	"net/http"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// BackfillChunkSearch creates missing registry rows from saved source chunks.
func BackfillChunkSearch(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261111-732")
	defer rc.Close()
	parseID := func(key string) (int64, error) {
		raw := strings.TrimSpace(c.QueryParam(key))
		if raw == "" {
			return 0, nil
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 || (key == "input_record_id" && n == 0) {
			return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid "+key)
		}
		return n, nil
	}
	after, err := parseID("after_record_id")
	if err != nil {
		return err
	}
	recordID, err := parseID("input_record_id")
	if err != nil {
		return err
	}
	limit := 20
	if raw := c.QueryParam("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, "limit must be 1..100")
		}
	}
	ctx := c.Request().Context()
	if user := rc.IsAuthenticated(); user != nil {
		ctx = docprocessing.WithLLMUserID(ctx, user.UserId)
	}
	call := newSearchCall(ctx, ApiTypes.ProjectDBHandle, "")
	ctx = docprocessing.WithLLMRunID(ctx, call.runID)
	result, err := docprocessing.BackfillChunkSearch(ctx, ApiTypes.ProjectDBHandle, after, recordID, limit, rc.GetLogger())
	if err != nil {
		rc.GetLogger().Error("chunk search backfill failed", "error", err)
		return echo.NewHTTPError(http.StatusInternalServerError, "chunk search backfill failed")
	}
	rc.GetLogger().Info("chunk search backfill finished", "scanned", result.Scanned, "indexed", result.Indexed, "failed", result.Failed, "next_record_id", result.NextRecordID)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "result": result})
}
