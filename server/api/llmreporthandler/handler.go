package llmreporthandler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmreconcile"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type reportStore interface {
	ListDailyReports(ctx context.Context, limit int) ([]DailyReport, error)
	ListUsageEvents(ctx context.Context, limit int) ([]UsageEvent, error)
	ListCurrentBalances(ctx context.Context, limit int) ([]CurrentBalance, error)
}

type reconciliationRunner interface {
	Run(ctx context.Context) error
	RunWithResult(ctx context.Context) (llmreconcile.RunResult, error)
}

type usageReportRunner interface {
	Run(ctx context.Context) error
	RunWithResult(ctx context.Context) (DailyUsageRunResult, error)
}

var reportStoreFactory = func() reportStore {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	return NewStore(ApiTypes.ProjectDBHandle)
}

var reconciliationRunnerFactory = func() reconciliationRunner {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return nil
	}
	return &llmreconcile.Runner{
		Store:        llmreconcile.NewStore(ApiTypes.ProjectDBHandle),
		BalanceAPI:   &llmreconcile.DeepSeekBalanceClient{},
		ArchiveRoot:  llmCfg.ArchiveRoot,
		WorkspaceTZ:  loc,
		TimezoneName: llmCfg.WorkspaceTimezone,
	}
}

var usageReportRunnerFactory = func() usageReportRunner {
	if ApiTypes.ProjectDBHandle == nil {
		return nil
	}
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return nil
	}
	return &DailyUsageReportRunner{
		DB:           ApiTypes.ProjectDBHandle,
		WorkspaceTZ:  loc,
		TimezoneName: llmCfg.WorkspaceTimezone,
		RunHour:      llmCfg.ReconciliationRunHour,
	}
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

func ListCurrentBalances(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 20)
	rows, err := store.ListCurrentBalances(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list current llm balances", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"balances": rows})
}

func RunReconciliationNow(c echo.Context) error {
	usageRunner := usageReportRunnerFactory()
	runner := reconciliationRunnerFactory()
	if runner == nil || usageRunner == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "llm reconciliation is not available"})
	}

	usageResult, err := usageRunner.RunWithResult(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to generate daily usage reports", "error": err.Error()})
	}

	reconcileResult, err := runner.RunWithResult(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to run llm reconciliation", "error": err.Error()})
	}

	message := "Manual LLM run finished."
	if usageResult.RowsAffected == 0 && reconcileResult.ReportsReconciled == 0 {
		message = "Manual LLM run finished, but no visible daily report rows were created yet. This usually means there are no captured usage events yet, or DeepSeek only has its first balance snapshot so there is not enough history to reconcile yesterday."
	}

	return c.JSON(http.StatusOK, map[string]any{
		"ok":                   true,
		"message":              message,
		"usage_days_processed": usageResult.DaysProcessed,
		"usage_rows_affected":  usageResult.RowsAffected,
		"accounts_considered":  reconcileResult.AccountsConsidered,
		"snapshots_created":    reconcileResult.SnapshotsCreated,
		"reports_reconciled":   reconcileResult.ReportsReconciled,
	})
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
