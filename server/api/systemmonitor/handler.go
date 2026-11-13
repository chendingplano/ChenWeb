package systemmonitor

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const maxBytes = 256 * 1024

type source struct {
	id, env, fallback string
	directory         bool
}

var sources = []source{
	{"chenweb", "GO_BACKEND_LOG_DIR", "chenweb", true},
	{"doc-processor", "DOC_PROCESSOR_LOG_DIR", "doc-processor", true},
	{"kratos", "KRATOS_LOG_FILE", "kratos.log", false},
	{"nats", "NATS_LOG_FILE", "nats.log", false},
	{"doc-service", "DOC_SERVICE_LOG_DIR", "doc-service", true},
	{"pdf-parser", "PDF_PARSER_LOG_FILE", "pdf-parser.log", false},
	{"caddy", "CADDY_LOG_FILE", "caddy.log", false},
	{"parser-result-converter", "PARSER_RESULT_CONVERTER_LOG_DIR", "parser-result-converter", true},
	{"deepseek-watcher", "DEEPSEEK_WATCH_LOG", "deepseek-traffic.log", false},
}

func readTail(path string, limit int64) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	truncated := st.Size() > limit
	if truncated {
		if _, err = f.Seek(-limit, io.SeekEnd); err != nil {
			return "", false, err
		}
	}
	b, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return "", false, err
	}
	if truncated {
		if i := strings.IndexByte(string(b), '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	return string(b), truncated, nil
}

func sourcePath(s source) string {
	root := os.Getenv("SYSTEM_MONITOR_LOG_ROOT")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, "Apps", "ChenWebLog", "system-monitor")
	}
	p := os.Getenv(s.env)
	if p == "" {
		p = filepath.Join(root, s.fallback)
	}
	if s.directory {
		// lumberjack writes app.log; filewriter writes log_XX files.
		candidates, _ := filepath.Glob(filepath.Join(p, "log_*"))
		candidates = append(candidates, filepath.Join(p, "app.log"))
		var newest string
		var newestTime int64
		for _, candidate := range candidates {
			if st, err := os.Stat(candidate); err == nil && (newest == "" || st.ModTime().UnixNano() > newestTime) {
				newest, newestTime = candidate, st.ModTime().UnixNano()
			}
		}
		if newest != "" {
			return newest
		}
		return filepath.Join(p, "app.log")
	}
	return p
}

func List(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261113-421")
	defer rc.Close()
	user := rc.IsAuthenticated()
	if user == nil {
		return c.JSON(http.StatusUnauthorized, map[string]any{"status": false})
	}
	admin := user.Admin || user.IsOwner
	for _, role := range user.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "admin" || role == "root" {
			admin = true
		}
	}
	if !admin {
		return c.JSON(http.StatusForbidden, map[string]any{"status": false})
	}
	rows := make([]map[string]any, 0, len(sources))
	for _, s := range sources {
		content, truncated, err := readTail(sourcePath(s), maxBytes)
		state := "ok"
		if os.IsNotExist(err) {
			state = "missing"
		} else if err != nil {
			state = "error"
			rc.GetLogger().Error("system monitor log read failed", "source", s.id, "error", err)
		}
		rows = append(rows, map[string]any{"id": s.id, "content": content, "truncated": truncated, "state": state})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "sources": rows})
}
