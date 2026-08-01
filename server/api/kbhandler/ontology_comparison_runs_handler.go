package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// CreateOntologyComparisonRun records a reproducible evaluation attempt for
// an existing frozen scope. The assertion watermark is computed server-side
// from the scope's own pinned target_object_ids -- never accepted from the
// request body -- so a run's provenance can't be forged or diverge from
// governed state. Cells are added separately and retain all evidence.
func CreateOntologyComparisonRun(c echo.Context) error {
	var payload struct {
		InputRecordID     int64  `json:"input_record_id"`
		ComparatorVersion string `json:"comparator_version"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	db := ApiTypes.ProjectDBHandle
	scopeID := c.Param("scope_id")
	store := comparison.ComparisonStore{DB: db}
	scope, err := store.GetScope(c.Request().Context(), scopeID)
	if err != nil {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "comparison scope not found"})
	}
	watermark, err := comparison.ComputeAssertionWatermark(c.Request().Context(), scope, assertions.AssertionStore{DB: db})
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to compute assertion watermark"})
	}
	created, err := store.CreateRun(c.Request().Context(), comparison.ComparisonRun{
		ComparisonScopeID:  scopeID,
		InputRecordID:      payload.InputRecordID,
		AssertionWatermark: watermark,
		ComparatorVersion:  payload.ComparatorVersion,
	})
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create comparison run"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": created})
}
