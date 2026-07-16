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
