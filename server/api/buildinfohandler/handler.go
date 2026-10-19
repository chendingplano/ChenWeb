package buildinfohandler

import (
	"net/http"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type activity struct {
	ID            int64     `json:"id"`
	BuildNumber   string    `json:"build_number"`
	BuildName     string    `json:"build_name"`
	Executable    string    `json:"executable"`
	TargetMachine string    `json:"target_machine"`
	OSInfo        string    `json:"os_info"`
	Description   string    `json:"description"`
	Activity      string    `json:"activity"`
	CreatedAt     time.Time `json:"created_at"`
}

func List(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_BUILD_INFO_001")
	defer rc.Close()
	logger := rc.GetLogger()
	user := rc.IsAuthenticated()
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"status": false, "error_msg": "authentication required"})
	}
	admin := user.Admin || user.IsOwner
	for _, role := range user.Roles {
		if role = strings.ToLower(strings.TrimSpace(role)); role == "admin" || role == "root" {
			admin = true
		}
	}
	if !admin {
		return c.JSON(http.StatusForbidden, map[string]any{"status": false, "error_msg": "admin access required"})
	}

	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT id, build_number, build_name, executable, target_machine, os_info,
		       description, activity, created_at
		  FROM kb.build_activities
		 ORDER BY created_at DESC, id DESC
		 LIMIT 200`)
	if err != nil {
		logger.Error("failed to list build activities", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": "failed to load build info"})
	}
	defer rows.Close()

	results := make([]activity, 0)
	for rows.Next() {
		var item activity
		if err := rows.Scan(&item.ID, &item.BuildNumber, &item.BuildName, &item.Executable,
			&item.TargetMachine, &item.OSInfo, &item.Description, &item.Activity, &item.CreatedAt); err != nil {
			logger.Error("failed to scan build activity", "error", err)
			return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": "failed to load build info"})
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("failed to iterate build activities", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": "failed to load build info"})
	}

	logger.Info("listed build activities", "count", len(results))
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": results})
}
