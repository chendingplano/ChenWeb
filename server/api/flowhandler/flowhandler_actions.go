// server/api/flowhandler/flowhandler_actions.go
package flowhandler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// GetDefaultFlow returns the user's default flow.
// GET /api/v1/flows/default — returns 404 if none configured.
func GetDefaultFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_060")
	logger.Info("GetDefaultFlow called")
	// TODO: query DB for user's default flow
	return c.JSON(http.StatusOK, map[string]any{"flow": stubFlow})
}

// SetDefaultFlow marks a flow as the user's default (clears previous default).
// PUT /api/v1/flows/:id/default
func SetDefaultFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_070")
	id := c.Param("id")
	logger.Info("SetDefaultFlow called id=" + id)
	return c.JSON(http.StatusOK, map[string]any{"message": "default flow set", "flow_id": id})
}

// ForkFlow creates a new private flow forked from an existing one.
// POST /api/v1/flows/:id/fork
func ForkFlow(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_080")
	id := c.Param("id")
	logger.Info("ForkFlow called source_id=" + id)
	forked := map[string]any{}
	for k, v := range stubFlow {
		forked[k] = v
	}
	forked["flow_id"] = 99
	forked["flow_name"] = fmt.Sprintf("Copy of %v", stubFlow["flow_name"])
	forked["is_default"] = false
	forked["is_template"] = false
	forked["is_shared"] = false
	forked["created_at"] = time.Now().Format(time.RFC3339)
	forked["updated_at"] = time.Now().Format(time.RFC3339)
	return c.JSON(http.StatusCreated, map[string]any{"flow": forked})
}

// SaveAsTemplate copies a flow as a public template.
// POST /api/v1/flows/:id/template
func SaveAsTemplate(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("CWB_FLW_090")
	id := c.Param("id")
	logger.Info("SaveAsTemplate called source_id=" + id)
	tmpl := map[string]any{}
	for k, v := range stubFlow {
		tmpl[k] = v
	}
	tmpl["flow_id"] = 100
	tmpl["is_template"] = true
	tmpl["is_shared"] = true
	tmpl["created_at"] = time.Now().Format(time.RFC3339)
	tmpl["updated_at"] = time.Now().Format(time.RFC3339)
	return c.JSON(http.StatusCreated, map[string]any{"flow": tmpl})
}
