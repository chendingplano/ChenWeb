package sitehandler

import (
	"log"
	"net/http"

	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

type workspaceContentConfigResponse struct {
	Status       bool              `json:"status"`
	Visibility   map[string]bool   `json:"visibility"`
	Labels       map[string]string `json:"labels"`
	Descriptions map[string]string `json:"descriptions"`
}

// GetWorkspaceContentConfig returns the configured [workspace-content]
// id->enabled mapping so /semos/workspace can filter its masthead/apps
// content, plus (when a lang query param is given) any configured id->label
// and id->description overrides for that language, from
// config/workspace-content/labels-<lang>.toml.
// Endpoint: GET /api/v1/workspace/content-config?lang=<code>. Ids absent from
// `visibility` default to enabled on the frontend; ids absent from `labels`/
// `descriptions` keep their site-config default text. An empty/unrecognized
// lang, or one with no matching file, resolves to empty `labels`/
// `descriptions` maps rather than an error. Mirrors kbhandler.GetKbMenuConfig
// (ADR 2026071601 / 2026071602), applied to the workspace page
// (ADR 2026071701).
func GetWorkspaceContentConfig(c echo.Context) error {
	visibility := appconfig.GetWorkspaceContentConfig()
	if visibility == nil {
		visibility = map[string]bool{}
	}

	labels, descriptions, err := LoadWorkspaceContentOverrides(c.QueryParam("lang"))
	if err != nil {
		log.Printf("***** Alarm: load workspace content overrides failed lang=%q err=%v (CWB_SITE_008)", c.QueryParam("lang"), err)
		labels = map[string]string{}
		descriptions = map[string]string{}
	}

	return c.JSON(http.StatusOK, workspaceContentConfigResponse{
		Status:       true,
		Visibility:   visibility,
		Labels:       labels,
		Descriptions: descriptions,
	})
}
