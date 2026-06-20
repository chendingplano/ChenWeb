package llmreporthandler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type reportStore interface {
	ListDailyReports(ctx context.Context, limit int) ([]DailyReport, error)
	ListUsageEvents(ctx context.Context, limit int) ([]UsageEvent, error)
}

var reportStoreFactory = func() reportStore {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	return NewStore(ApiTypes.ProjectDBHandle)
}

func ListDailyReports(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 30)
	rows, err := store.ListDailyReports(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list daily llm reports", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"reports": rows})
}

func ListUsageEvents(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 50)
	rows, err := store.ListUsageEvents(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list llm usage events", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"usage_events": rows})
}

func intParamDefault(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
