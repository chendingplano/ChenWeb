// server/api/flowhandler/flowhandler.go
// Package flowhandler provides stub HTTP handlers for flow management.
// All handlers return placeholder data. Replace with real DB calls in a future phase.
package flowhandler

import (
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var stubFlow = map[string]any{
	"flow_id":           1,
	"user_id":           1,
	"flow_name":         "My First Flow",
	"flow_desc":         "",
	"is_default":        false,
	"is_shared":         false,
	"is_template":       false,
	"template_category": "",
	"flow_data":         map[string]any{"nodes": []any{}, "edges": []any{}},
	"thumbnail_svg":     nil,
	"created_at":        time.Now().Format(time.RFC3339),
	"updated_at":        time.Now().Format(time.RFC3339),
}

// ListFlows returns the list of flows filtered by scope.
// GET /api/v1/flows?scope=mine|shared|templates
func ListFlows(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_010")
	scope := c.QueryParam("scope")
	logger.Info("ListFlows called scope=" + scope)
	return c.JSON(http.StatusOK, map[string]any{"flows": []any{stubFlow}})
}

// CreateFlow creates a new flow.
// POST /api/v1/flows
func CreateFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_020")
	logger.Info("CreateFlow called")
	return c.JSON(http.StatusCreated, map[string]any{"flow": stubFlow})
}

// GetFlow returns a single flow by ID.
// GET /api/v1/flows/:id
func GetFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_030")
	id := c.Param("id")
	logger.Info("GetFlow called id=" + id)
	return c.JSON(http.StatusOK, map[string]any{"flow": stubFlow})
}

// UpdateFlow updates a flow by ID.
// PUT /api/v1/flows/:id
func UpdateFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_040")
	id := c.Param("id")
	logger.Info("UpdateFlow called id=" + id)
	updated := map[string]any{}
	for k, v := range stubFlow {
		updated[k] = v
	}
	updated["updated_at"] = time.Now().Format(time.RFC3339)
	return c.JSON(http.StatusOK, map[string]any{"flow": updated})
}

// DeleteFlow deletes a flow by ID.
// DELETE /api/v1/flows/:id
func DeleteFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_050")
	id := c.Param("id")
	logger.Info("DeleteFlow called id=" + id)
	return c.JSON(http.StatusOK, map[string]any{"message": "flow deleted", "flow_id": id})
}
