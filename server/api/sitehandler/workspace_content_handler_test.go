package sitehandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

func newWorkspaceContentContext(t *testing.T, target string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

// withWorkspaceContentConfig points AppConfig.WorkspaceContent at cfg for the
// duration of the test.
func withWorkspaceContentConfig(t *testing.T, cfg map[string]bool) {
	t.Helper()
	old := appconfig.AppConfig.WorkspaceContent
	appconfig.AppConfig.WorkspaceContent = cfg
	t.Cleanup(func() { appconfig.AppConfig.WorkspaceContent = old })
}

func TestGetWorkspaceContentConfigReturnsEmptyMapsWhenUnconfigured(t *testing.T) {
	withWorkspaceContentConfig(t, nil)

	c, rec := newWorkspaceContentContext(t, "/api/v1/workspace/content-config")
	if err := GetWorkspaceContentConfig(c); err != nil {
		t.Fatalf("GetWorkspaceContentConfig returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload workspaceContentConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, got %+v", payload)
	}
	if len(payload.Visibility) != 0 {
		t.Fatalf("expected an empty visibility map, got %v", payload.Visibility)
	}
	if len(payload.Labels) != 0 {
		t.Fatalf("expected an empty labels map (no lang given), got %v", payload.Labels)
	}
	if len(payload.Descriptions) != 0 {
		t.Fatalf("expected an empty descriptions map (no lang given), got %v", payload.Descriptions)
	}
}

func TestGetWorkspaceContentConfigReturnsConfiguredVisibility(t *testing.T) {
	withWorkspaceContentConfig(t, map[string]bool{
		"workflows":        false,
		"ws-announcements": true,
	})

	c, rec := newWorkspaceContentContext(t, "/api/v1/workspace/content-config")
	if err := GetWorkspaceContentConfig(c); err != nil {
		t.Fatalf("GetWorkspaceContentConfig returned error: %v", err)
	}

	var payload workspaceContentConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if got, ok := payload.Visibility["workflows"]; !ok || got != false {
		t.Fatalf("workflows=%v ok=%v", got, ok)
	}
	if got, ok := payload.Visibility["ws-announcements"]; !ok || got != true {
		t.Fatalf("ws-announcements=%v ok=%v", got, ok)
	}
}

func TestGetWorkspaceContentConfigReturnsLabelsAndDescriptionsForLang(t *testing.T) {
	withWorkspaceContentConfig(t, nil)
	withWorkspaceContentLabelsDir(t, map[string]string{
		"labels-zh-cn.toml": `
[labels]
ws-kicker = "工作台"

[descriptions]
knowledge_base = "浏览和管理文档与知识工件。"
`,
	})

	c, rec := newWorkspaceContentContext(t, "/api/v1/workspace/content-config?lang=zh-cn")
	if err := GetWorkspaceContentConfig(c); err != nil {
		t.Fatalf("GetWorkspaceContentConfig returned error: %v", err)
	}

	var payload workspaceContentConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Labels["ws-kicker"] != "工作台" {
		t.Fatalf("ws-kicker = %q", payload.Labels["ws-kicker"])
	}
	if payload.Descriptions["knowledge_base"] != "浏览和管理文档与知识工件。" {
		t.Fatalf("knowledge_base description = %q", payload.Descriptions["knowledge_base"])
	}
}
