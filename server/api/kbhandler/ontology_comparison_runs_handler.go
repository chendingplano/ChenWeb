package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// CreateOntologyComparisonRun records a reproducible evaluation attempt for
// an existing frozen scope. Cells are added separately and retain all evidence.
func CreateOntologyComparisonRun(c echo.Context) error {
	var run comparison.ComparisonRun
	if err := json.NewDecoder(c.Request().Body).Decode(&run); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body"})
	}
	run.ComparisonScopeID = c.Param("scope_id")
	created, err := (comparison.ComparisonStore{DB: ApiTypes.ProjectDBHandle}).CreateRun(c.Request().Context(), run)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create comparison run"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": created})
}
