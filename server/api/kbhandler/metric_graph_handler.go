package kbhandler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type metricGraphResponse struct {
	Status bool                       `json:"status"`
	Metric MetricGraphMetric          `json:"metric"`
	Nodes  map[string]MetricGraphNode `json:"nodes"`
}

// GetMetricGraph handles GET /api/v1/kb/metrics/:metric_id/graph.
//
// It returns, for a single metric, the related rows for every Metric Ontology
// Explorer chain node, keyed by the frontend model's chain-node ids. metric_id
// is the canonical "<record_id>_mtc_<seqno>"; the legacy "<record_id>_<seqno>"
// form is accepted and normalized. An empty or malformed value is a 400 with no
// query run; an unknown metric is a 404. Read-only — no write path.
func GetMetricGraph(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MGRAPH_001")
	defer rc.Close()
	logger := rc.GetLogger()

	metricID := strings.TrimSpace(c.Param("metric_id"))
	recordID, seqno, err := parseMetricID(metricID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid metric_id (CWB_KB_MGRAPH_010)",
		})
	}
	metricID = canonicalMetricID(recordID, seqno)

	graph, err := (metricGraphStore{DB: ApiTypes.ProjectDBHandle}).Load(c.Request().Context(), recordID, metricID)
	if errors.Is(err, errMetricNotFound) {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: "metric not found (CWB_KB_MGRAPH_020)",
		})
	}
	if err != nil {
		logger.Error("load metric graph failed", "metric_id", metricID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to load metric graph (CWB_KB_MGRAPH_030)",
		})
	}

	return c.JSON(http.StatusOK, metricGraphResponse{
		Status: true,
		Metric: graph.Metric,
		Nodes:  graph.Nodes,
	})
}
