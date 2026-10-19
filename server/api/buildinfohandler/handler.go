package buildinfohandler

import (
	"net/http"
	"strings"

	"github.com/chendingplano/deepdoc/server/buildinfo"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

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

	processes := buildinfo.ListProcesses([]string{"server", "doc-processor", "parser-result-converter", "doc-service"})
	running := 0
	for _, process := range processes {
		if process.Status == "running" {
			running++
		}
	}
	logger.Info("listed live build info", "running_processes", running, "processes", len(processes))
	return c.JSON(http.StatusOK, map[string]any{"status": true, "processes": processes})
}
