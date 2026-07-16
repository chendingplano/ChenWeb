package kbhandler

import (
	"net/http"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

type kbMenuConfigResponse struct {
	Status bool              `json:"status"`
	Menus  map[string]bool   `json:"menus"`
	Labels map[string]string `json:"labels"`
}

// GetKbMenuConfig returns the configured [knowledge-menus] id->enabled
// mapping so the Wiki sidebar on /home3/knowledge can filter its menu, plus
// (when a lang query param is given) any configured id->label overrides for
// that language, from config/knowledge-menus/labels-<lang>.toml.
// Endpoint: GET /api/v1/kb/menu-config?lang=<code>. Ids absent from `menus`
// default to enabled on the frontend; ids absent from `labels` keep their
// hardcoded default label. An empty/unrecognized lang, or one with no
// matching file, resolves to an empty `labels` map rather than an error.
func GetKbMenuConfig(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MENU_001")
	defer rc.Close()

	menus := appconfig.GetKnowledgeMenusConfig()
	if menus == nil {
		menus = map[string]bool{}
	}

	labels, err := LoadKnowledgeMenuLabels(c.QueryParam("lang"))
	if err != nil {
		rc.GetLogger().Warn("load knowledge menu labels failed", "lang", c.QueryParam("lang"), "err", err)
		labels = map[string]string{}
	}

	return c.JSON(http.StatusOK, kbMenuConfigResponse{Status: true, Menus: menus, Labels: labels})
}
