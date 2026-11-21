package papersearch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

type Handler struct {
	Search func(context.Context, string) ([]byte, error)
}

func (h Handler) Handle(c echo.Context) error {
	var request struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, 4096)).Decode(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid_request"})
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > 1000 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid_query"})
	}
	search := h.Search
	if search == nil {
		search = runSearch
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), 50*time.Second)
	defer cancel()
	logger := loggerutil.CreateDefaultLogger("20261121-732")
	logger.Info("Scientific paper search started")
	result, err := search(ctx, query)
	if err != nil || !json.Valid(result) {
		logger.Error("Scientific paper search failed", "error", err)
		return c.JSON(http.StatusBadGateway, map[string]string{"error": "search_failed"})
	}
	logger.Info("Scientific paper search completed")
	return c.Blob(http.StatusOK, "application/json", result)
}

func runSearch(ctx context.Context, query string) ([]byte, error) {
	binary := strings.TrimSpace(os.Getenv("PAPER_SEARCH_MCP_COMMAND"))
	if binary == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		binary = filepath.Join(home, "Workspace", "ThirdParty-2", "paper-search-mcp", ".venv", "bin", "paper-search")
	}
	cmd := exec.CommandContext(ctx, binary, "search", query, "-s", "fast", "-n", "10", "--source-timeout", "30")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if len(output) > 4*1024*1024 {
		return nil, errors.New("search response too large")
	}
	return output, nil
}
