package kbhandler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// artifactObjectStatsDTO is the Artifact Object Statistics payload. Provisions,
// Metrics, Inventory Items, and Other partition by artifact_type (mutually
// exclusive, sum to Total). Unresolved is orthogonal (object_id IS NULL) and is
// reported separately, not as a partition slice.
type artifactObjectStatsDTO struct {
	Total          int64 `json:"total"`
	Provisions     int64 `json:"provisions"`
	Metrics        int64 `json:"metrics"`
	InventoryItems int64 `json:"inventory_items"`
	Other          int64 `json:"other"`
	Unresolved     int64 `json:"unresolved"`
}

// objectNodeStatsDTO is the Object Nodes Statistics payload. The per-type counts
// are the number of object nodes connected to at least one artifact object of
// that type; a node can connect to several types, so they overlap and do not
// sum to Total.
type objectNodeStatsDTO struct {
	Total          int64 `json:"total"`
	Provisions     int64 `json:"provisions"`
	Metrics        int64 `json:"metrics"`
	InventoryItems int64 `json:"inventory_items"`
}

type connectivityRowDTO struct {
	ObjectID      string `json:"object_id"`
	CanonicalName string `json:"canonical_name"`
	Connections   int64  `json:"connections"`
}

var connectivityTopNChoices = map[int]bool{20: true, 50: true, 100: true, 200: true, 300: true}

// clampConnectivityTopN restricts the histogram size to the offered pulldown
// choices, defaulting to 50 for anything outside the set.
func clampConnectivityTopN(n int) int {
	if connectivityTopNChoices[n] {
		return n
	}
	return 50
}

// GetArtifactObjectStats handles GET /api/v1/kb/objects/stats/artifact-objects.
func GetArtifactObjectStats(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OST_001")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OST_010)"})
	}

	var s artifactObjectStatsDTO
	err := db.QueryRowContext(c.Request().Context(), `
SELECT
  COUNT(*),
  COUNT(*) FILTER (WHERE artifact_type = 'provision'),
  COUNT(*) FILTER (WHERE artifact_type = 'metric'),
  COUNT(*) FILTER (WHERE artifact_type = 'inventory_item'),
  COUNT(*) FILTER (WHERE object_id IS NULL)
FROM kb.artifact_objects`).Scan(&s.Total, &s.Provisions, &s.Metrics, &s.InventoryItems, &s.Unresolved)
	if err != nil {
		logger.Error("artifact object stats query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load artifact object stats (CWB_KB_OST_011)"})
	}
	s.Other = s.Total - s.Provisions - s.Metrics - s.InventoryItems

	return c.JSON(http.StatusOK, map[string]any{"status": true, "stats": s})
}

// GetObjectNodeStats handles GET /api/v1/kb/objects/stats/object-nodes.
func GetObjectNodeStats(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OST_100")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OST_110)"})
	}

	var s objectNodeStatsDTO
	err := db.QueryRowContext(c.Request().Context(), `
SELECT
  (SELECT COUNT(*) FROM kb.object_nodes),
  COUNT(DISTINCT object_id) FILTER (WHERE artifact_type = 'provision'),
  COUNT(DISTINCT object_id) FILTER (WHERE artifact_type = 'metric'),
  COUNT(DISTINCT object_id) FILTER (WHERE artifact_type = 'inventory_item')
FROM kb.artifact_objects
WHERE object_id IS NOT NULL`).Scan(&s.Total, &s.Provisions, &s.Metrics, &s.InventoryItems)
	if err != nil {
		logger.Error("object node stats query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load object node stats (CWB_KB_OST_111)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true, "stats": s})
}

// GetObjectConnectivity handles GET /api/v1/kb/objects/connectivity?top_n=50 —
// the Top N most connected object nodes in descending connection count.
func GetObjectConnectivity(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_OST_200")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_OST_210)"})
	}

	topN := 50
	if raw := strings.TrimSpace(c.QueryParam("top_n")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			topN = n
		}
	}
	topN = clampConnectivityTopN(topN)

	rows, err := db.QueryContext(c.Request().Context(), `
SELECT n.object_id, COALESCE(n.canonical_name, ''), COUNT(ao.id)
FROM kb.object_nodes n
LEFT JOIN kb.artifact_objects ao ON ao.object_id = n.object_id
GROUP BY n.object_id, n.canonical_name
ORDER BY COUNT(ao.id) DESC, n.object_id
LIMIT $1`, topN)
	if err != nil {
		logger.Error("object connectivity query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load object connectivity (CWB_KB_OST_211)"})
	}
	defer func() { _ = rows.Close() }()

	out := make([]connectivityRowDTO, 0, topN)
	for rows.Next() {
		var r connectivityRowDTO
		if err := rows.Scan(&r.ObjectID, &r.CanonicalName, &r.Connections); err != nil {
			logger.Error("scan connectivity row failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read object connectivity (CWB_KB_OST_212)"})
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("connectivity rows err", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read object connectivity (CWB_KB_OST_213)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true, "top_n": topN, "rows": out})
}
