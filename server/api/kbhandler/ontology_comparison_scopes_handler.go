package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/comparison"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ontologyComparisonScopeResponse struct {
	Status bool                       `json:"status"`
	Record comparison.ComparisonScope `json:"record"`
}

// CreateOntologyComparisonScope freezes the selection and release snapshots
// used by one or more generic DR21/DR22 comparison runs.
func CreateOntologyComparisonScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OCS_100")
	defer rc.Close()
	var scope comparison.ComparisonScope
	if err := json.NewDecoder(c.Request().Body).Decode(&scope); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OCS_101)"})
	}
	created, err := (comparison.ComparisonStore{DB: ApiTypes.ProjectDBHandle}).CreateScope(c.Request().Context(), scope)
	if err != nil {
		rc.GetLogger().Error("create ontology comparison scope failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create immutable comparison scope (CWB_KB_OCS_102)"})
	}
	return c.JSON(http.StatusOK, ontologyComparisonScopeResponse{Status: true, Record: created})
}

// GetOntologyComparisonScope returns one frozen comparison selection.
func GetOntologyComparisonScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OCS_200")
	defer rc.Close()
	got, err := (comparison.ComparisonStore{DB: ApiTypes.ProjectDBHandle}).GetScope(c.Request().Context(), c.Param("scope_id"))
	if err != nil {
		rc.GetLogger().Error("get ontology comparison scope failed", "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology comparison scope not found (CWB_KB_OCS_201)"})
	}
	return c.JSON(http.StatusOK, ontologyComparisonScopeResponse{Status: true, Record: got})
}
