package kbhandler

import (
	"net/http"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ResolveEntityObjects re-attempts entity->object-node resolution for
// kb.entities rows at object_link_status IN ('pending', 'deferred') — see
// docprocessing.ResolveEntityObjects for the classify/exclude/associate/defer
// rules (ADR 2026070101 Phase 4). This is the repeatable backlog-drain
// endpoint for what Phase 3's per-record match-only pass could not resolve
// deterministically; it never runs on a schedule inside the app (see the
// ADR's "Repeatable Endpoint, Not a New Scheduler" rationale).
//
// POST /kb/entities/resolve-objects
//
//	?limit=  (rows per call; default 200, call repeatedly until scanned=0)
func ResolveEntityObjects(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_REO_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_REO_010)"})
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 200)

	classifier, cfg, err := docprocessing.NewEntityObjectClassifierFromEnv()
	if err != nil {
		logger.Error("resolve entity objects: build classifier failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "resolve entity objects failed (CWB_KB_REO_011)"})
	}
	if classifier == nil {
		return c.JSON(http.StatusServiceUnavailable, errorResponse{Status: false, ErrorMsg: "entity object resolution is not configured: set ENTITY_OBJECT_RESOLVE_MODEL_NAME (CWB_KB_REO_012)"})
	}

	store := docprocessing.EntityObjectResolveSQLStore{DB: db}
	objectStore := docprocessing.ArtifactObjectSQLStore{DB: db}
	reconciler := docprocessing.ObjectReconciler{
		Store:   docprocessing.ObjectNodeSQLStore{DB: db},
		Options: docprocessing.ObjectReconcileOptionsFromEnv(),
	}

	logger.Info("resolve entity objects started", "limit", limit)
	result, err := docprocessing.ResolveEntityObjects(c.Request().Context(), store, objectStore, reconciler, classifier, cfg, limit, logger)
	if err != nil {
		logger.Error("resolve entity objects failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "resolve entity objects failed (CWB_KB_REO_013)"})
	}
	logger.Info("resolve entity objects finished",
		"scanned", result.Scanned, "classified", result.Classified, "skipped_unchanged", result.SkippedUnchanged,
		"excluded", result.Excluded, "linked", result.Linked, "deferred", result.Deferred,
		"exhausted", result.Exhausted, "failed", result.Failed)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"result": result,
	})
}
