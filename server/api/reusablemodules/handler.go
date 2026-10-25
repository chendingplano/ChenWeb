// Package reusablemodules serves the read-only development module catalogue.
package reusablemodules

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

type Module struct {
	ID           int64           `json:"id"`
	ModuleID     string          `json:"module_id"`
	ModuleName   string          `json:"module_name"`
	ModuleType   string          `json:"module_type"`
	ProgLanguage string          `json:"prog_language"`
	Description  string          `json:"description"`
	Files        json.RawMessage `json:"files"`
	Version      string          `json:"version"`
	ConfigJSON   json.RawMessage `json:"config_json"`
	Status       string          `json:"status"`
	DocPath      string          `json:"doc_path"`
	CreatedBy    string          `json:"created_by"`
	UpdatedBy    string          `json:"updated_by"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// Handler is registered inside the authenticated /api/v1 group and uses its
// shared project database pool. Records are ordered for stable card placement.
func Handler(db *sql.DB) echo.HandlerFunc {
	logger := loggerutil.CreateDefaultLogger("20261025-673")
	return func(c echo.Context) error {
		fail := func(err error) error {
			logger.Error("List reusable modules failed", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Unable to load modules"})
		}
		rows, err := db.QueryContext(c.Request().Context(), `SELECT id, module_id, module_name, module_type, prog_language, description, files, version, config_json, status, doc_path, created_by, updated_by, created_at, updated_at FROM kb.reusable_modules ORDER BY module_name, module_id`)
		if err != nil {
			return fail(err)
		}
		defer rows.Close()
		modules := make([]Module, 0)
		for rows.Next() {
			var module Module
			var files, config []byte
			if err := rows.Scan(&module.ID, &module.ModuleID, &module.ModuleName, &module.ModuleType, &module.ProgLanguage, &module.Description, &files, &module.Version, &config, &module.Status, &module.DocPath, &module.CreatedBy, &module.UpdatedBy, &module.CreatedAt, &module.UpdatedAt); err != nil {
				return fail(err)
			}
			module.Files, module.ConfigJSON = files, config
			modules = append(modules, module)
		}
		if err := rows.Err(); err != nil {
			return fail(err)
		}
		logger.Info("Listed reusable modules", "count", len(modules))
		return c.JSON(http.StatusOK, map[string]any{"modules": modules})
	}
}
