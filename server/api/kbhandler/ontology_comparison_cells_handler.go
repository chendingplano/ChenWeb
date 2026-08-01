package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// CreateOntologyComparisonCell persists one already-evaluated directional
// cell, including complete evidence lists and comparator rationale.
func CreateOntologyComparisonCell(c echo.Context) error {
	runID, err := strconv.ParseInt(c.Param("run_id"), 10, 64)
	if err != nil || runID < 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid comparison run id"})
	}
	var cell comparison.ComparisonCell
	if err := json.NewDecoder(c.Request().Body).Decode(&cell); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	cell.RunID = runID
	if err := (comparison.ComparisonStore{DB: ApiTypes.ProjectDBHandle}).PersistCell(c.Request().Context(), cell); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to persist comparison cell"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
