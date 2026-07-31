package kbhandler

import (
	"database/sql"
	"errors"
	"net/http"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type docFacetsResponse struct {
	Status bool                          `json:"status"`
	Result *docprocessing.DocFacetRecord `json:"result,omitempty"`
}

// GetDocFacets handles GET /api/v1/kb/doc-facets?record_id=N, returning the
// deterministic routing facets last computed for that record (kb.doc_facets
// is upserted whenever P1 plan facts are resolved for a doc-processing run).
func GetDocFacets(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DF_001")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil || recordID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "query param 'record_id' must be a positive integer (CWB_KB_DF_002)",
		})
	}

	store := docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle}
	facets, err := store.GetDocFacets(c.Request().Context(), *recordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, docFacetsResponse{Status: true})
		}
		logger.Error("get doc facets failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to get doc facets (CWB_KB_DF_003)",
		})
	}

	return c.JSON(http.StatusOK, docFacetsResponse{Status: true, Result: &facets})
}
