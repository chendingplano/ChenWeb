package kbhandler

import (
	"encoding/json"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type ontologyProfileResponse struct {
	Status bool             `json:"status"`
	Record profiles.Profile `json:"record"`
}

// CreateOntologyProfile creates only an initial draft. Visibility and
// normative execution remain controlled by approval and module activation.
func CreateOntologyProfile(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OP_100")
	defer rc.Close()
	var profile profiles.Profile
	if err := json.NewDecoder(c.Request().Body).Decode(&profile); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_OP_101)"})
	}
	profile.ModifyBy = profile.CreateBy
	created, err := (profiles.ProfileStore{DB: ApiTypes.ProjectDBHandle}).CreateProfile(c.Request().Context(), profile)
	if err != nil {
		rc.GetLogger().Error("create ontology profile failed", "err", err)
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "failed to create draft ontology profile (CWB_KB_OP_102)"})
	}
	return c.JSON(http.StatusOK, ontologyProfileResponse{Status: true, Record: created})
}
