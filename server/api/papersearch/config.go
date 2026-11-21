package papersearch

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var configMu sync.Mutex
var allowedCredentials = map[string]bool{
	"SEMANTIC_SCHOLAR_API_KEY": true, "OPENALEX_API_KEY": true, "OPENALEX_EMAIL": true,
	"CORE_API_KEY": true, "UNPAYWALL_EMAIL": true, "DOAJ_API_KEY": true,
	"ZENODO_ACCESS_TOKEN": true, "GOOGLE_SCHOLAR_PROXY_URL": true, "OPENAIRE_API_KEY": true,
	"CITESEERX_API_KEY": true, "IEEE_API_KEY": true, "WOS_API_KEY": true, "SCOPUS_API_KEY": true, "SCOPUS_INST_TOKEN": true,
}

type config struct {
	Source      string            `json:"source"`
	MaxResults  int               `json:"max_results"`
	Timeout     int               `json:"timeout"`
	Credentials map[string]string `json:"credentials"`
}
type configUpdate struct {
	Source      string            `json:"source"`
	MaxResults  int               `json:"max_results"`
	Timeout     int               `json:"timeout"`
	Credentials map[string]string `json:"credentials"`
	Clear       []string          `json:"clear"`
}

func configPath() (string, error) {
	if p := strings.TrimSpace(os.Getenv("PAPER_SEARCH_CONFIG_FILE")); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "paper-search-mcp", "chenweb-config.json"), nil
}
func loadConfig() (config, error) {
	configMu.Lock()
	defer configMu.Unlock()
	return loadConfigLocked()
}
func loadConfigLocked() (config, error) {
	c := config{Source: "fast", MaxResults: 10, Timeout: 30, Credentials: map[string]string{}}
	p, err := configPath()
	if err != nil {
		return c, err
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Credentials == nil {
		c.Credentials = map[string]string{}
	}
	return c, nil
}
func saveConfig(u configUpdate) error {
	if u.Source != "fast" && u.Source != "fastest" && u.Source != "all" {
		return errors.New("invalid source preset")
	}
	if u.MaxResults < 1 || u.MaxResults > 50 || u.Timeout < 5 || u.Timeout > 60 {
		return errors.New("invalid limits")
	}
	for k, v := range u.Credentials {
		if !allowedCredentials[k] || len(v) > 4096 || strings.ContainsAny(v, "\x00\r\n") {
			return errors.New("invalid credential")
		}
	}
	for _, k := range u.Clear {
		if !allowedCredentials[k] {
			return errors.New("invalid credential")
		}
	}
	configMu.Lock()
	defer configMu.Unlock()
	c, err := loadConfigLocked()
	if err != nil {
		return err
	}
	c.Source = u.Source
	c.MaxResults = u.MaxResults
	c.Timeout = u.Timeout
	for k, v := range u.Credentials {
		if strings.TrimSpace(v) != "" {
			c.Credentials[k] = strings.TrimSpace(v)
		}
	}
	for _, k := range u.Clear {
		delete(c.Credentials, k)
	}
	p, err := configPath()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".chenweb-config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
func admin(c echo.Context, loc string) bool {
	rc := EchoFactory.NewFromEcho(c, loc)
	defer rc.Close()
	u := rc.IsAuthenticated()
	if u == nil {
		_ = c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication_required"})
		return false
	}
	if id := strings.TrimSpace(u.UserId); id == "" || id == "-" {
		_ = c.JSON(http.StatusUnauthorized, map[string]string{"error": "authentication_required"})
		return false
	}
	ok := u.Admin || u.IsOwner
	for _, role := range u.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		ok = ok || role == "admin" || role == "root"
	}
	if !ok {
		_ = c.JSON(http.StatusForbidden, map[string]string{"error": "admin_required"})
	}
	return ok
}
func GetConfig(c echo.Context) error {
	if !admin(c, "20261121-733") {
		return nil
	}
	cfg, err := loadConfig()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "config_read_failed"})
	}
	set := map[string]bool{}
	for k, v := range cfg.Credentials {
		set[k] = v != ""
	}
	return c.JSON(http.StatusOK, map[string]any{"source": cfg.Source, "max_results": cfg.MaxResults, "timeout": cfg.Timeout, "configured": set})
}
func PutConfig(c echo.Context) error {
	if !admin(c, "20261121-734") {
		return nil
	}
	var u configUpdate
	dec := json.NewDecoder(io.LimitReader(c.Request().Body, 65536))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&u); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid_config"})
	}
	if err := saveConfig(u); err != nil {
		loggerutil.CreateDefaultLogger("20261121-734").Error("paper search configuration save failed", "error", err)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "config_save_failed"})
	}
	loggerutil.CreateDefaultLogger("20261121-734").Info("paper search configuration saved")
	return c.JSON(http.StatusOK, map[string]bool{"saved": true})
}
