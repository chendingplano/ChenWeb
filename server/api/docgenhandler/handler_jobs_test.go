package docgenhandler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/docgenhandler"
	"github.com/labstack/echo/v4"
)

func newEcho() *echo.Echo { return echo.New() }

// TestSubmitJob_Unauthenticated_Returns401 verifies that unauthenticated
// callers receive 401 before any field validation occurs.
func TestSubmitJob_Unauthenticated_Returns401(t *testing.T) {
	e := newEcho()
	body := `{"request_name":"test","purpose":"test","template_type":"word","template_name":"t.docx","output_dir":"/tmp","output_format":"docx","sql_statement":"SELECT 1","converter":{"cid":"customer_id","cname":"customer_name","cemail":"email"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docgen/jobs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.SubmitJob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListJobs_Returns200or500(t *testing.T) {
	e := newEcho()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docgen/jobs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.ListJobs(c); err != nil {
		t.Fatal(err)
	}
	// Without a live DB, expect either 200 or 500
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 200 or 500, got %d", rec.Code)
	}
}
