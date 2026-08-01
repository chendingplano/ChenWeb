package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ontologyReviewScopeResponse struct {
	Status bool                 `json:"status"`
	Record profiles.ReviewScope `json:"record"`
}

// CreateOntologyReviewScope freezes an explicit or deterministic selection
// before any review execution. The endpoint deliberately exposes no update
// operation: a changed selection requires a new scope id.
func CreateOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_100")
	defer rc.Close()
	var scope profiles.ReviewScope
	if err := json.NewDecoder(c.Request().Body).Decode(&scope); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_ORS_101)"})
	}
	created, err := (profiles.ReviewScopeStore{DB: ApiTypes.ProjectDBHandle}).Create(c.Request().Context(), scope)
	if err != nil {
		rc.GetLogger().Error("create ontology review scope failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create immutable review scope (CWB_KB_ORS_102)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewScopeResponse{Status: true, Record: created})
}

// GetOntologyReviewScope returns the stored selection facts for a historical
// or future deterministic review.
func GetOntologyReviewScope(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_ORS_200")
	defer rc.Close()
	got, err := (profiles.ReviewScopeStore{DB: ApiTypes.ProjectDBHandle}).Get(c.Request().Context(), c.Param("scope_id"))
	if err != nil {
		rc.GetLogger().Error("get ontology review scope failed", "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "ontology review scope not found (CWB_KB_ORS_201)"})
	}
	return c.JSON(http.StatusOK, ontologyReviewScopeResponse{Status: true, Record: got})
}
