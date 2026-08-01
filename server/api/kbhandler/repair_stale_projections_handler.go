package kbhandler

import (
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// RepairStaleProjectionsResult is one projection kind's outcome from a
// repair sweep.
type RepairStaleProjectionsResult struct {
	ProjectionKind string `json:"projection_kind"`
	Examined       int    `json:"examined"`
	Repaired       int    `json:"repaired"`
}

// RepairStaleProjections sweeps every registered projection kind (DR11 seam
// 7) -- or one kind, if requested -- comparing each target's materialized
// value against its authoritative source and repairing any mismatch,
// including direct corruption that never went through kb.projection_state
// (spec §16.2 item 6, §16.3 item 14). This is the previously-missing caller
// for assertions.RepairStaleProjections: safe to call repeatedly, an
// already-correct projection reports zero repairs.
//
// POST /kb/semantic-decisions/repair-stale-projections
//
//	?kind=  (one projection_kind; omitted = every registered kind)
func RepairStaleProjections(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RSP_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_RSP_010)"})
	}

	kinds := assertions.RegisteredProjectionKinds()
	if requested := c.QueryParam("kind"); requested != "" {
		kinds = []string{requested}
	}

	logger.Info("repair stale projections started", "kinds", kinds)
	results := make([]RepairStaleProjectionsResult, 0, len(kinds))
	for _, kind := range kinds {
		examined, repaired, err := assertions.RepairStaleProjections(c.Request().Context(), db, kind)
		if err != nil {
			logger.Error("repair stale projections failed", "kind", kind, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "repair stale projections failed (CWB_KB_RSP_011)"})
		}
		results = append(results, RepairStaleProjectionsResult{ProjectionKind: kind, Examined: examined, Repaired: repaired})
	}
	logger.Info("repair stale projections finished", "results", results)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"result": results,
	})
}
