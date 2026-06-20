package llmreporthandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type stubReportStore struct {
	daily []DailyReport
	usage []UsageEvent
}

func (s *stubReportStore) ListDailyReports(_ context.Context, limit int) ([]DailyReport, error) {
	return s.daily, nil
}

func (s *stubReportStore) ListUsageEvents(_ context.Context, limit int) ([]UsageEvent, error) {
	return s.usage, nil
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
