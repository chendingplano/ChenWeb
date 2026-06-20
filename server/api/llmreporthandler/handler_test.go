package llmreporthandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/llmreconcile"
	"github.com/labstack/echo/v4"
)

type stubReportStore struct {
	daily []DailyReport
	usage []UsageEvent
	bal   []CurrentBalance
}

func (s *stubReportStore) ListDailyReports(_ context.Context, limit int) ([]DailyReport, error) {
	return s.daily, nil
}

func (s *stubReportStore) ListUsageEvents(_ context.Context, limit int) ([]UsageEvent, error) {
	return s.usage, nil
}

func (s *stubReportStore) ListCurrentBalances(_ context.Context, limit int) ([]CurrentBalance, error) {
	return s.bal, nil
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
	if !strings.Contains(rec.Body.String(), `"spend_amount":6.25`) {
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
	if !strings.Contains(rec.Body.String(), `"deepseek-v4-flash"`) {
		t.Fatalf("unexpected body = %s", rec.Body.String())
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
