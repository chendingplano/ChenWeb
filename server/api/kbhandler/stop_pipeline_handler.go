package kbhandler

import (
	"net/http"
	"strconv"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// StopPipeline handles POST /api/v1/kb/inputs/:id/stop.
//
// Two things happen atomically from the caller's perspective:
//  1. The stop-requested flag is written to kb.inputs.status so the running
//     doc-processor service cancels its pipeline context within ~1 s.
//  2. doc_processing is immediately updated to proc_status:"stopped" if it is
//     currently "running" — this handles the case where the doc-processor is
//     not running and the record is stuck in a running state.
func StopPipeline(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KBST_001")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid record id (CWB_KBST_011)",
		})
	}

	logger.Info("stop pipeline request", "record_id", id)

	ctx := c.Request().Context()
	store := docprocessing.StopRequestSQLStore{DB: ApiTypes.ProjectDBHandle}

	// Signal the running doc-processor (if any) to cancel its pipeline context.
	if err := store.SetStopRequested(ctx, id); err != nil {
		logger.Error("stop pipeline: set stop flag failed", "record_id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to request pipeline stop (CWB_KBST_021)",
		})
	}

	// Immediately mark doc_processing as stopped for the "not running" case.
	// Safe to call when the doc-processor IS running — idempotent.
	if err := store.MarkDocProcessingStopped(ctx, id); err != nil {
		logger.Warn("stop pipeline: update doc_processing status failed", "record_id", id, "error", err)
		// Non-fatal: the running doc-processor will still pick up the stop flag.
	}

	logger.Info("stop pipeline requested", "record_id", id)
	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"msg":    "stop requested",
	})
}
