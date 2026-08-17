package llmreporthandler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmreconcile"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	sharedllm "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

type reportStore interface {
	ListDailyReports(ctx context.Context, limit int) ([]DailyReport, error)
	ListModelActivityReports(ctx context.Context, limit int) ([]ModelActivityReport, error)
	ListUsageEvents(ctx context.Context, limit int) ([]UsageEvent, error)
	ListCurrentBalances(ctx context.Context, limit int) ([]CurrentBalance, error)
	GetTodaySummary(ctx context.Context, workspaceDay time.Time, timezoneName string) (TodaySummary, error)
	ListUsageEventsAdmin(ctx context.Context, page, pageSize int, filters UsageEventAdminFilters) ([]UsageEventAdmin, int64, error)
	GetUsageEventBodyRefs(ctx context.Context, id string) (inputRef, outputRef string, err error)
	ListUsageEventsByIDs(ctx context.Context, ids []string) ([]UsageEventAdmin, error)
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

func ListModelActivityReports(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	limit := intParamDefault(c.QueryParam("limit"), 30)
	rows, err := store.ListModelActivityReports(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list llm model activity reports", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"reports": rows})
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

func GetTodaySummary(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	llmCfg := config.GetLLMConfig()
	loc, err := time.LoadLocation(llmCfg.WorkspaceTimezone)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load workspace timezone", "error": err.Error()})
	}
	now := time.Now().In(loc)
	workspaceDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	summary, err := store.GetTodaySummary(c.Request().Context(), workspaceDay, llmCfg.WorkspaceTimezone)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to load today's llm summary", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"summary": summary})
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

func ListUsageEventsAdmin(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	page := intParamDefault(c.QueryParam("page"), 1)
	pageSize := intParamDefault(c.QueryParam("page_size"), 50)
	filters := UsageEventAdminFilters{
		Model:      c.QueryParam("model"),
		Prompt:     c.QueryParam("prompt"),
		CallReason: c.QueryParam("call_reason"),
		CallLoc:    c.QueryParam("call_loc"),
		MetaKey:    c.QueryParam("meta_key"),
		MetaValue:  c.QueryParam("meta_value"),
	}
	if v, ok := int64Param(c.QueryParam("run_id")); ok {
		filters.RunID = &v
	}
	if v, ok := timeParam(c.QueryParam("started_from")); ok {
		filters.StartedFrom = &v
	}
	if v, ok := timeParam(c.QueryParam("started_to")); ok {
		filters.StartedTo = &v
	}
	if v, ok := int64Param(c.QueryParam("in_tok_min")); ok {
		filters.InTokMin = &v
	}
	if v, ok := int64Param(c.QueryParam("in_tok_max")); ok {
		filters.InTokMax = &v
	}
	if v, ok := int64Param(c.QueryParam("out_tok_min")); ok {
		filters.OutTokMin = &v
	}
	if v, ok := int64Param(c.QueryParam("out_tok_max")); ok {
		filters.OutTokMax = &v
	}
	rows, total, err := store.ListUsageEventsAdmin(c.Request().Context(), page, pageSize, filters)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to list llm usage events", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"events": rows, "total": total, "page": page, "page_size": pageSize})
}

// GetUsageEventsByIDs returns llm_usage_event rows for a comma-separated list
// of ids, e.g. the ids recorded in kb.doc_review_logs.detail.llm_usage_event_ids.
func GetUsageEventsByIDs(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	var ids []string
	for id := range strings.SplitSeq(c.QueryParam("ids"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	rows, err := store.ListUsageEventsByIDs(c.Request().Context(), ids)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to fetch llm usage events", "error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"usage_events": rows})
}

func GetUsageEventBody(c echo.Context) error {
	store := reportStoreFactory()
	if store == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{"ok": false, "message": "project database is not initialized"})
	}
	id := c.Param("id")
	bodyType := c.QueryParam("type")
	if bodyType != "input" && bodyType != "output" {
		return c.JSON(http.StatusBadRequest, map[string]any{"ok": false, "message": "type must be 'input' or 'output'"})
	}
	inputRef, outputRef, err := store.GetUsageEventBodyRefs(c.Request().Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "event not found"})
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to fetch event", "error": err.Error()})
	}
	ref := inputRef
	if bodyType == "output" {
		ref = outputRef
	}
	if ref == "" {
		return c.JSON(http.StatusNotFound, map[string]any{"ok": false, "message": "body not archived for this event"})
	}
	archiveRoot := config.GetLLMConfig().ArchiveRoot
	fullPath := filepath.Join(archiveRoot, ref)
	data, err := sharedllm.ReadGzipFile(fullPath)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"ok": false, "message": "failed to read archive file", "error": err.Error()})
	}
	return c.JSONBlob(http.StatusOK, data)
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

// timeParam parses an RFC3339 timestamp query param. ok is false if raw is empty or invalid.
func timeParam(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// int64Param parses an integer query param. ok is false if raw is empty or invalid.
func int64Param(raw string) (int64, bool) {
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}
