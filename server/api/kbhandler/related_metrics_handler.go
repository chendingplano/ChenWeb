package kbhandler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type relatedMetricsResponse struct {
	Status      bool               `json:"status"`
	MetricID    string             `json:"metric_id"`
	Scope       string             `json:"scope"`
	ClassTermID *string            `json:"class_term_id"`
	Results     []relatedMetricRow `json:"results"`
}

// GetRelatedMetrics handles GET /api/v1/kb/metrics/:metric_id/related-metrics.
//
//	?scope=same_class|similar_class   (required)
//	?limit=1..200                     (default 20 for similar_class, 200 for same_class)
//
// same_class returns every other metric whose resolved governed class term equals
// the selected metric's. similar_class returns the top N metrics that are
// instances of class contracts hybrid-matched as similar to the selected metric's
// class contract. A metric with no resolved class is a valid empty result
// (class_term_id: null), not an error. Read-only — no write path.
func GetRelatedMetrics(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_RELM_001")
	defer rc.Close()
	logger := rc.GetLogger()

	metricID := strings.TrimSpace(c.Param("metric_id"))
	recordID, seqno, err := parseMetricID(metricID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid metric_id (CWB_KB_RELM_010)",
		})
	}
	metricID = canonicalMetricID(recordID, seqno)

	scope, ok := parseRelatedMetricsScope(c.QueryParam("scope"))
	if !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "scope must be same_class or similar_class (CWB_KB_RELM_011)",
		})
	}
	limit := relatedMetricsLimit(c.QueryParam("limit"), scope)

	store := relatedMetricsStore{DB: ApiTypes.ProjectDBHandle}

	exists, err := store.metricExists(c.Request().Context(), metricID)
	if err != nil {
		logger.Error("related metrics: metric lookup failed", "metric_id", metricID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to look up metric (CWB_KB_RELM_020)",
		})
	}
	if !exists {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "metric not found (CWB_KB_RELM_021)",
		})
	}

	classTermID, err := store.resolveClassTermID(c.Request().Context(), metricID)
	if err != nil {
		logger.Error("related metrics: resolve class failed", "metric_id", metricID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve metric class (CWB_KB_RELM_022)",
		})
	}

	resp := relatedMetricsResponse{
		Status:   true,
		MetricID: metricID,
		Scope:    scope,
		Results:  []relatedMetricRow{},
	}
	if classTermID == "" {
		// No resolved governed class: a valid empty result, not an error.
		resp.ClassTermID = nil
		return c.JSON(http.StatusOK, resp)
	}
	resp.ClassTermID = &classTermID

	var results []relatedMetricRow
	switch scope {
	case "same_class":
		results, err = store.sameClass(c.Request().Context(), classTermID, metricID, limit)
	case "similar_class":
		// computeQueryEmbedding is best-effort: ok=false (no model / API error)
		// makes MatchSimilar fall back to the lexical half.
		results, err = store.similarClass(c.Request().Context(), classTermID, metricID, limit, computeQueryEmbedding)
	}
	if errors.Is(err, errRelatedMetricNotFound) {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "metric not found (CWB_KB_RELM_021)",
		})
	}
	if err != nil {
		logger.Error("related metrics: cohort query failed", "metric_id", metricID, "scope", scope, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to load related metrics (CWB_KB_RELM_030)",
		})
	}
	if results != nil {
		resp.Results = results
	}
	return c.JSON(http.StatusOK, resp)
}
