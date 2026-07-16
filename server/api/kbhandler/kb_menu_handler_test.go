package kbhandler

import (
	"encoding/json"
	"net/http"
	"testing"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// withKnowledgeMenusConfig points AppConfig.KnowledgeMenus at cfg for the
// duration of the test.
func withKnowledgeMenusConfig(t *testing.T, cfg map[string]bool) {
	t.Helper()
	old := appconfig.AppConfig.KnowledgeMenus
	appconfig.AppConfig.KnowledgeMenus = cfg
	t.Cleanup(func() { appconfig.AppConfig.KnowledgeMenus = old })
}

func TestGetKbMenuConfigReturnsEmptyMapWhenUnconfigured(t *testing.T) {
	withKnowledgeMenusConfig(t, nil)

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/menu-config", "")
	if err := GetKbMenuConfig(c); err != nil {
		t.Fatalf("GetKbMenuConfig returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload kbMenuConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, got %+v", payload)
	}
	if len(payload.Menus) != 0 {
		t.Fatalf("expected an empty menus map, got %v", payload.Menus)
	}
}

func TestGetKbMenuConfigReturnsConfiguredOverrides(t *testing.T) {
	withKnowledgeMenusConfig(t, map[string]bool{
		"kb-doc-wiki": false,
		"kb-metrics":  true,
	})

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/menu-config", "")
	if err := GetKbMenuConfig(c); err != nil {
		t.Fatalf("GetKbMenuConfig returned error: %v", err)
	}

	var payload kbMenuConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, got %+v", payload)
	}
	if got, ok := payload.Menus["kb-doc-wiki"]; !ok || got != false {
		t.Fatalf("kb-doc-wiki=%v ok=%v", got, ok)
	}
	if got, ok := payload.Menus["kb-metrics"]; !ok || got != true {
		t.Fatalf("kb-metrics=%v ok=%v", got, ok)
	}
}

func TestGetKbMenuConfigOmittedLangReturnsEmptyLabels(t *testing.T) {
	withKnowledgeMenusConfig(t, nil)
	withKnowledgeMenuLabelsDir(t, map[string]string{
		"labels-zh-cn.toml": `[labels]
kb-metrics = "指标"`,
	})

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/menu-config", "")
	if err := GetKbMenuConfig(c); err != nil {
		t.Fatalf("GetKbMenuConfig returned error: %v", err)
	}

	var payload kbMenuConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(payload.Labels) != 0 {
		t.Fatalf("expected empty labels for omitted lang, got %v", payload.Labels)
	}
}

func TestGetKbMenuConfigReturnsLabelsForConfiguredLang(t *testing.T) {
	withKnowledgeMenusConfig(t, nil)
	withKnowledgeMenuLabelsDir(t, map[string]string{
		"labels-zh-cn.toml": `[labels]
kb-metrics = "指标"
kb-doc-wiki = "知识百科"`,
	})

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/menu-config?lang=zh-cn", "")
	if err := GetKbMenuConfig(c); err != nil {
		t.Fatalf("GetKbMenuConfig returned error: %v", err)
	}

	var payload kbMenuConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(payload.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d (%v)", len(payload.Labels), payload.Labels)
	}
	if payload.Labels["kb-metrics"] != "指标" {
		t.Fatalf("kb-metrics label = %q", payload.Labels["kb-metrics"])
	}
}

func TestGetKbMenuConfigLangWithNoMatchingFileReturnsEmptyLabels(t *testing.T) {
	withKnowledgeMenusConfig(t, map[string]bool{"kb-metrics": false})
	withKnowledgeMenuLabelsDir(t, map[string]string{
		"labels-zh-cn.toml": `[labels]
kb-metrics = "指标"`,
	})

	c, rec := newKnowledgeStoreContext(t, http.MethodGet, "/api/v1/kb/menu-config?lang=fr", "")
	if err := GetKbMenuConfig(c); err != nil {
		t.Fatalf("GetKbMenuConfig returned error: %v", err)
	}

	var payload kbMenuConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(payload.Labels) != 0 {
		t.Fatalf("expected empty labels for lang with no file, got %v", payload.Labels)
	}
	if got, ok := payload.Menus["kb-metrics"]; !ok || got != false {
		t.Fatalf("expected menus unaffected by lang, kb-metrics=%v ok=%v", got, ok)
	}
}
