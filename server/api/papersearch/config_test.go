package papersearch

import (
	"context"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSaveAndSecrets(t *testing.T) {
	t.Setenv("PAPER_SEARCH_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	config, err := loadConfig()
	if err != nil || config.Source != "fast" || config.MaxResults != 10 {
		t.Fatalf("defaults: %+v %v", config, err)
	}
	err = saveConfig(configUpdate{Source: "fastest", MaxResults: 5, Timeout: 20, Credentials: map[string]string{"OPENALEX_API_KEY": "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	config, err = loadConfig()
	if err != nil || config.Credentials["OPENALEX_API_KEY"] != "secret" || config.Source != "fastest" {
		t.Fatalf("saved: %+v %v", config, err)
	}
	info, err := os.Stat(os.Getenv("PAPER_SEARCH_CONFIG_FILE"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("file permissions: %v %v", info, err)
	}
	err = saveConfig(configUpdate{Source: "fast", MaxResults: 10, Timeout: 30, Credentials: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	config, _ = loadConfig()
	if config.Credentials["OPENALEX_API_KEY"] != "secret" {
		t.Fatal("blank update erased credential")
	}
	err = saveConfig(configUpdate{Source: "fast", MaxResults: 10, Timeout: 30, Clear: []string{"OPENALEX_API_KEY"}})
	if err != nil {
		t.Fatal(err)
	}
	config, _ = loadConfig()
	if _, ok := config.Credentials["OPENALEX_API_KEY"]; ok {
		t.Fatal("credential was not removed")
	}
	err = saveConfig(configUpdate{Source: "invalid", MaxResults: 10, Timeout: 30})
	if err == nil {
		t.Fatal("invalid source accepted")
	}
}

func TestConfigAuthorizationAndRedaction(t *testing.T) {
	t.Setenv("PAPER_SEARCH_CONFIG_FILE", filepath.Join(t.TempDir(), "config.json"))
	if err := saveConfig(configUpdate{Source: "fast", MaxResults: 10, Timeout: 30, Credentials: map[string]string{"CORE_API_KEY": "secret-value"}}); err != nil {
		t.Fatal(err)
	}
	old := EchoFactory.DefaultAuthenticator
	defer func() { EchoFactory.DefaultAuthenticator = old }()
	e := echo.New()
	e.GET("/config", GetConfig)
	e.PUT("/config", PutConfig)
	request := func(method string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		e.ServeHTTP(r, httptest.NewRequest(method, "/config", strings.NewReader(`{"source":"fast","max_results":10,"timeout":30}`)))
		return r
	}
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{UserId: "u", Roles: []string{"user"}}, nil
	}
	for _, method := range []string{"GET", "PUT"} {
		if r := request(method); r.Code != http.StatusForbidden {
			t.Fatalf("%s: %d", method, r.Code)
		}
	}
	EchoFactory.DefaultAuthenticator = func(ApiTypes.RequestContext) (*ApiTypes.UserInfo, error) {
		return &ApiTypes.UserInfo{UserId: "admin", Admin: true}, nil
	}
	r := request("GET")
	if r.Code != 200 || strings.Contains(r.Body.String(), "secret-value") || !strings.Contains(r.Body.String(), `"CORE_API_KEY":true`) {
		t.Fatalf("unexpected response: %s", r.Body.String())
	}
}

func TestConfiguredSearchInvocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PAPER_SEARCH_CONFIG_FILE", filepath.Join(dir, "config.json"))
	script := filepath.Join(dir, "search")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"args\":\"%s\",\"key\":\"%s\"}' \"$*\" \"$PAPER_SEARCH_MCP_CORE_API_KEY\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PAPER_SEARCH_MCP_COMMAND", script)
	t.Setenv("PAPER_SEARCH_MCP_CORE_API_KEY", "inherited")
	if err := saveConfig(configUpdate{Source: "fastest", MaxResults: 3, Timeout: 12, Credentials: map[string]string{"CORE_API_KEY": "saved"}}); err != nil {
		t.Fatal(err)
	}
	b, err := runSearch(context.Background(), "some query")
	if err != nil || !strings.Contains(string(b), "fastest -n 3 --source-timeout 12") || !strings.Contains(string(b), `"key":"saved"`) {
		t.Fatalf("unexpected output %s: %v", b, err)
	}
}
