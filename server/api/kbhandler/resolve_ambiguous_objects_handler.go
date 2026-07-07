package kbhandler

import (
	"net/http"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ResolveAmbiguousObjects re-attempts reconciliation for kb.artifact_objects rows
// stuck at reconcile_status='ambiguous' (object_id NULL) because ReconcileOne's
// tied-candidate branch never assigns an object_id or creates a node. See
// docprocessing.ResolveAmbiguousArtifactObjects for the resolution rules.
//
// POST /kb/objects/resolve-ambiguous
//
//	?limit=  (rows per call; default 200, call repeatedly until scanned=0)
func ResolveAmbiguousObjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RAO_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_RAO_010)"})
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 200)

	store := docprocessing.ArtifactObjectSQLStore{DB: db}
	reconciler := docprocessing.ObjectReconciler{
		Store:   docprocessing.ObjectNodeSQLStore{DB: db},
		Options: docprocessing.ObjectReconcileOptionsFromEnv(),
	}

	logger.Info("resolve ambiguous objects started", "limit", limit)
	result, err := docprocessing.ResolveAmbiguousArtifactObjects(c.Request().Context(), store, reconciler, logger, limit)
	if err != nil {
		logger.Error("resolve ambiguous objects failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "resolve ambiguous objects failed (CWB_KB_RAO_011)"})
	}
	logger.Info("resolve ambiguous objects finished",
		"scanned", result.Scanned, "matched", result.Matched,
		"tie_broken", result.TieBroken, "created", result.Created, "failed", result.Failed)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"result": result,
	})
}
