package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestQuestionBrowserFilters(t *testing.T) {
	for _, query := range []string{"?run_id=-1", "?input_record_id=wat", "?page=0", "?page_size=101", "?created_from=2026-10-24", "?created_from=2026-10-24T12:00:00Z&created_to=2026-10-24T11:00:00Z", "?page=9999999999999999999999"} {
		c := echo.New().NewContext(httptest.NewRequest("GET", "/"+query, nil), httptest.NewRecorder())
		if _, err := parseQuestionBrowserFilters(c); err == nil {
			t.Errorf("accepted %s", query)
		}
	}
	c := echo.New().NewContext(httptest.NewRequest("GET", "/?purpose=search_metrics&run_id=6&input_record_id=416&lang=zh-cn&created_from=2026-10-24T10:00:00-05:00&created_to=2026-10-24T11:00:00-05:00&page=2&page_size=10", nil), httptest.NewRecorder())
	filters, err := parseQuestionBrowserFilters(c)
	if err != nil {
		t.Fatal(err)
	}
	if filters.Purpose != "search_metrics" || filters.RunID != 6 || filters.RecordID != 416 || filters.Lang != "zh-cn" || filters.Page != 2 || filters.PageSize != 10 || !filters.From.Equal(time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("filters=%+v", filters)
	}
	// Values are parameters, including arbitrary historical purpose/language values.
	filters.Purpose = "x' OR TRUE --"
	where, args := filters.where()
	if strings.Contains(where, filters.Purpose) || len(args) != 6 || args[0] != filters.Purpose {
		t.Fatalf("where=%s args=%v", where, args)
	}
}

func TestQuestionBrowserInvalidRequest(t *testing.T) {
	response := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/?created_from=not-a-date", nil), response)
	if err := ListGeneratedQuestions(c); err != nil || response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d error=%v", response.Code, err)
	}
}
