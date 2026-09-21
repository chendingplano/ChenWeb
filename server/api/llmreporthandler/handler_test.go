package llmreporthandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmreconcile"
	"github.com/labstack/echo/v4"
)

type stubReportStore struct {
	daily                []DailyReport
	model                []ModelActivityReport
	usage                []UsageEvent
	bal                  []CurrentBalance
	balanceHistory       []BalanceHistory
	hourlyBalanceReports []HourlyBalanceReport
	sum                  TodaySummary
	usageByIDs           []UsageEventAdmin
	modelFilters         ModelActivityReportFilters
	balanceFilters       ModelActivityReportFilters
}

func (s *stubReportStore) ListDailyReports(_ context.Context, limit int) ([]DailyReport, error) {
	return s.daily, nil
}

func (s *stubReportStore) ListModelActivityReports(_ context.Context, limit int, filters ModelActivityReportFilters) ([]ModelActivityReport, error) {
	s.modelFilters = filters
	return s.model, nil
}

func (s *stubReportStore) ListUsageEvents(_ context.Context, limit int) ([]UsageEvent, error) {
	return s.usage, nil
}

func (s *stubReportStore) ListCurrentBalances(_ context.Context, limit int) ([]CurrentBalance, error) {
	return s.bal, nil
}

func (s *stubReportStore) ListBalanceHistory(_ context.Context, limit int) ([]BalanceHistory, error) {
	return s.balanceHistory, nil
}

func (s *stubReportStore) ListHourlyBalanceReports(_ context.Context, limit int, frequency string, filters ModelActivityReportFilters) ([]HourlyBalanceReport, error) {
	s.balanceFilters = filters
	return s.hourlyBalanceReports, nil
}

func (s *stubReportStore) GetTodaySummary(_ context.Context, workspaceDay time.Time, timezoneName string) (TodaySummary, error) {
	return s.sum, nil
}

func (s *stubReportStore) ListUsageEventsAdmin(_ context.Context, page, pageSize int, filters UsageEventAdminFilters) ([]UsageEventAdmin, int64, error) {
	return nil, 0, nil
}

func (s *stubReportStore) GetUsageEventBodyRefs(_ context.Context, id string) (string, string, error) {
	return "", "", nil
}

func (s *stubReportStore) ListUsageEventsByIDs(_ context.Context, ids []string) ([]UsageEventAdmin, error) {
	return s.usageByIDs, nil
}

type stubReconciliationRunner struct {
	runCount int
	result   llmreconcile.RunResult
}

func (r *stubReconciliationRunner) Run(_ context.Context) error {
	r.runCount++
	return nil
}

func (r *stubReconciliationRunner) RunWithResult(_ context.Context) (llmreconcile.RunResult, error) {
	r.runCount++
	return r.result, nil
}

type stubUsageReportRunner struct {
	result DailyUsageRunResult
}

func (r *stubUsageReportRunner) Run(_ context.Context) error {
	return nil
}

func (r *stubUsageReportRunner) RunWithResult(_ context.Context) (DailyUsageRunResult, error) {
	return r.result, nil
}

func TestListDailyReportsReturnsRows(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore {
		return &stubReportStore{
			daily: []DailyReport{{
				AccountID:    "acct_1",
				AccountName:  "deepseek:api.deepseek.com",
				SpendAmount:  6.25,
				RequestCount: 8,
			}},
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/reports/daily", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListDailyReports(c); err != nil {
		t.Fatalf("ListDailyReports() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"spend_amount":6.25`) || !strings.Contains(rec.Body.String(), `"account_name":"deepseek:api.deepseek.com"`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestListUsageEventsReturnsRows(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore {
		return &stubReportStore{
			usage: []UsageEvent{{
				ID:               "evt_1",
				AccountName:      "deepseek:api.deepseek.com",
				ModelName:        "deepseek-v4-flash",
				PromptName:       "extract-products-v2",
				RequestStartedAt: time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC),
			}},
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/usage-events", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListUsageEvents(c); err != nil {
		t.Fatalf("ListUsageEvents() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"deepseek-v4-flash"`) || !strings.Contains(rec.Body.String(), `"account_name":"deepseek:api.deepseek.com"`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestListModelActivityReportsReturnsRows(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore {
		return &stubReportStore{
			model: []ModelActivityReport{{
				Provider:     "deepseek",
				ModelName:    "deepseek-v4-flash",
				CurrencyCode: "CNY",
				WorkspaceDay: "2026-06-20",
				SpendAmount:  11.88,
				RequestCount: 1380,
			}},
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/reports/models", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListModelActivityReports(c); err != nil {
		t.Fatalf("ListModelActivityReports() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"model_name":"deepseek-v4-flash"`) || !strings.Contains(rec.Body.String(), `"workspace_day":"2026-06-20"`) || !strings.Contains(rec.Body.String(), `"spend_amount":11.88`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestListModelActivityReportsRejectsInvalidDate(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore { return &stubReportStore{} }

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/reports/models?from=not-a-date", nil)
	rec := httptest.NewRecorder()
	if err := ListModelActivityReports(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListModelActivityReports() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListHourlyBalanceReportsRejectsInvalidDate(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore { return &stubReportStore{} }

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/balances/hourly?from=not-a-date", nil)
	rec := httptest.NewRecorder()
	if err := ListHourlyBalanceReports(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListHourlyBalanceReports() error = %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListHourlyBalanceReportsForwardsDateAndAPIKeyFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".models.toml")
	if err := os.WriteFile(path, []byte(`[model-api-keys]
provider-key = [{ api_key = 'provider-secret-ref' }]
`), 0600); err != nil {
		t.Fatalf("write models TOML: %v", err)
	}
	t.Setenv("CHENWEB_MODELS_TOML", path)

	store := &stubReportStore{}
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore { return store }

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/balances/hourly?frequency=daily&from=2026-09-01&to=2026-09-19&api_key=provider-key", nil)
	rec := httptest.NewRecorder()
	if err := ListHourlyBalanceReports(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListHourlyBalanceReports() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if store.balanceFilters.From == nil || store.balanceFilters.From.Format("2006-01-02") != "2026-09-01" ||
		store.balanceFilters.To == nil || store.balanceFilters.To.Format("2006-01-02") != "2026-09-19" ||
		store.balanceFilters.APIKeyRef != "provider-secret-ref" {
		t.Fatalf("unexpected balance filters = %+v", store.balanceFilters)
	}
}

func TestLoadModelAPIKeyOptionsReturnsNamesWithoutSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".models.toml")
	if err := os.WriteFile(path, []byte(`[model-api-keys]
	provider-pro = [
	api_key = 'sk-primary'
]

[provider-pro]
model_name = 'provider-pro'
api_key = 'sk-primary'
`), 0600); err != nil {
		t.Fatalf("write models TOML: %v", err)
	}
	t.Setenv("CHENWEB_MODELS_TOML", path)

	options, refs, err := loadModelAPIKeyOptions()
	if err != nil {
		t.Fatalf("loadModelAPIKeyOptions() error = %v", err)
	}
	if len(options) != 1 || options[0].Name != "provider-pro" {
		t.Fatalf("unexpected options = %+v", options)
	}
	if refs["provider-pro"] != "sk-primary" {
		t.Fatalf("unexpected reference map = %+v", refs)
	}
	ref, err := apiKeyRefForName("provider-pro", refs)
	if err != nil || ref != "sk-primary" {
		t.Fatalf("apiKeyRefForName() = %q, %v; want sk-primary", ref, err)
	}
}

// TestListModelActivityReportsDoesNotRestrictModelsForSelectedAlias guards
// against reintroducing a model-name guess on top of the api_key_ref filter:
// an API key can legitimately be declared against one model profile in
// .models.toml while production traffic actually uses a different model, so
// the handler must scope by account only and let every real model for that
// account come back.
func TestListModelActivityReportsDoesNotRestrictModelsForSelectedAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".models.toml")
	if err := os.WriteFile(path, []byte(`[model-api-keys]
alias-name = [{ api_key = 'sk-shared' }]

[declared-profile]
model_name = 'declared-model'
api_key = 'sk-shared'
`), 0600); err != nil {
		t.Fatalf("write models TOML: %v", err)
	}
	t.Setenv("CHENWEB_MODELS_TOML", path)

	store := &stubReportStore{}
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore { return store }

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/reports/models?api_key=alias-name", nil)
	rec := httptest.NewRecorder()
	if err := ListModelActivityReports(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListModelActivityReports() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if store.modelFilters.APIKeyRef != "sk-shared" {
		t.Fatalf("api key ref = %q, want sk-shared", store.modelFilters.APIKeyRef)
	}
	if len(store.modelFilters.ModelNames) != 0 {
		t.Fatalf("model filters = %#v, want none (any model actually used under this key must come back)", store.modelFilters.ModelNames)
	}
}

func TestListCurrentBalancesReturnsRows(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore {
		return &stubReportStore{
			bal: []CurrentBalance{{
				AccountID:     "acct_1",
				AccountName:   "deepseek:api.deepseek.com",
				Provider:      "deepseek",
				WorkspaceDay:  time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC),
				CapturedAt:    time.Date(2026, 6, 20, 15, 42, 19, 0, time.UTC),
				BalanceAmount: 475.59,
				CurrencyCode:  "CNY",
			}},
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/balances/current", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListCurrentBalances(c); err != nil {
		t.Fatalf("ListCurrentBalances() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"balance_amount":475.59`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestGetTodaySummaryReturnsRow(t *testing.T) {
	prev := reportStoreFactory
	t.Cleanup(func() { reportStoreFactory = prev })
	reportStoreFactory = func() reportStore {
		return &stubReportStore{
			sum: TodaySummary{
				WorkspaceDay: "2026-06-20",
				TimezoneName: "America/Chicago",
				SpendAmount:  12.34,
				CurrencyCode: "CNY",
				RequestCount: 7,
				TotalTokens:  4567,
				ErrorCount:   2,
			},
		}
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/llm/summary/today", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetTodaySummary(c); err != nil {
		t.Fatalf("GetTodaySummary() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"spend_amount":12.34`) || !strings.Contains(body, `"request_count":7`) || !strings.Contains(body, `"total_tokens":4567`) {
		t.Fatalf("unexpected body = %s", body)
	}
}

func TestRunReconciliationNowRunsRunner(t *testing.T) {
	prev := reconciliationRunnerFactory
	prevUsage := usageReportRunnerFactory
	t.Cleanup(func() {
		reconciliationRunnerFactory = prev
		usageReportRunnerFactory = prevUsage
	})
	runner := &stubReconciliationRunner{
		result: llmreconcile.RunResult{
			AccountsConsidered: 1,
			SnapshotsCreated:   1,
			ReportsReconciled:  0,
		},
	}
	reconciliationRunnerFactory = func() reconciliationRunner {
		return runner
	}
	usageReportRunnerFactory = func() usageReportRunner { return &stubUsageReportRunner{} }

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/llm/reconciliation/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := RunReconciliationNow(c); err != nil {
		t.Fatalf("RunReconciliationNow() error = %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if runner.runCount != 1 {
		t.Fatalf("runner.runCount = %d, want 1", runner.runCount)
	}
	if !strings.Contains(rec.Body.String(), `"ok":true`) || !strings.Contains(rec.Body.String(), `"snapshots_created":1`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
	}
}

func TestRunReconciliationNowReturnsServiceUnavailableWithoutRunner(t *testing.T) {
	prev := reconciliationRunnerFactory
	prevUsage := usageReportRunnerFactory
	t.Cleanup(func() {
		reconciliationRunnerFactory = prev
		usageReportRunnerFactory = prevUsage
	})
	reconciliationRunnerFactory = func() reconciliationRunner { return nil }
	usageReportRunnerFactory = func() usageReportRunner { return &stubUsageReportRunner{} }

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/llm/reconciliation/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := RunReconciliationNow(c); err != nil {
		t.Fatalf("RunReconciliationNow() error = %v", err)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}
