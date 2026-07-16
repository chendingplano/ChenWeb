package kbhandler

import (
	"net/http"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

type kbMenuConfigResponse struct {
	Status bool            `json:"status"`
	Menus  map[string]bool `json:"menus"`
}

// GetKbMenuConfig returns the configured [knowledge-menus] id->enabled
// mapping so the Wiki sidebar on /home3/knowledge can filter its menu.
// Endpoint: GET /api/v1/kb/menu-config. Ids absent from the response default
// to enabled on the frontend; an empty map means no overrides are configured.
func GetKbMenuConfig(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MENU_001")
	defer rc.Close()

	menus := appconfig.GetKnowledgeMenusConfig()
	if menus == nil {
		menus = map[string]bool{}
	}
	return c.JSON(http.StatusOK, kbMenuConfigResponse{Status: true, Menus: menus})
}
