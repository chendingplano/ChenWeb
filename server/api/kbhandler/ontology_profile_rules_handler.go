package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ontologyProfileRuleResponse struct {
	Status bool                 `json:"status"`
	Record profiles.ProfileRule `json:"record"`
}

// CreateOntologyProfileRule creates an initial draft rule. Its registered
// rule kind and parent profile version are validated by the generic store.
func CreateOntologyProfileRule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OPR_100")
	defer rc.Close()
	var rule profiles.ProfileRule
	if err := json.NewDecoder(c.Request().Body).Decode(&rule); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OPR_101)"})
	}
	rule.ModifyBy = rule.CreateBy
	created, err := (profiles.ProfileRuleStore{DB: ApiTypes.ProjectDBHandle}).CreateProfileRule(c.Request().Context(), rule)
	if err != nil {
		rc.GetLogger().Error("create ontology profile rule failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create draft ontology profile rule (CWB_KB_OPR_102)"})
	}
	return c.JSON(http.StatusOK, ontologyProfileRuleResponse{Status: true, Record: created})
}
