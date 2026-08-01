package kbhandler

import (
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// DrainDeferredSemanticDecisions re-attempts input records that have
// kb.semantic_decision_candidates stuck at status='deferred' with
// dependency_fingerprint='unresolved_referent' because their source
// artifact had no reconciled kb.artifact_objects row at normalize time.
// Mirrors ResolveAmbiguousObjects's DR5 bulk-backfill pattern (ADR
// 2026070701) applied to the Phase D association backlog (ADR 2026072901 P3
// chunk F).
//
// POST /kb/semantic-decisions/drain-deferred
//
//	?limit=  (records per call; default 200, call repeatedly until records_scanned=0)
func DrainDeferredSemanticDecisions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_DDD_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_DDD_010)"})
	}

	limit := parsePositiveInt(c.QueryParam("limit"), 200)

	logger.Info("drain deferred semantic decisions started", "limit", limit)
	report, err := assertions.DrainDeferredCandidates(c.Request().Context(), db, limit)
	if err != nil {
		logger.Error("drain deferred semantic decisions failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "drain deferred semantic decisions failed (CWB_KB_DDD_011)"})
	}
	logger.Info("drain deferred semantic decisions finished",
		"records_scanned", report.RecordsScanned, "records_reprocessed", report.RecordsReprocessed,
		"accepted", report.Accepted, "deferred", report.Deferred, "rejected", report.Rejected)

	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"result": report,
	})
}
