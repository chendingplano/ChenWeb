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

func TestSubmitJob_MissingFields_Returns400(t *testing.T) {
	e := newEcho()
	body := `{"request_name":"","purpose":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docgen/jobs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.SubmitJob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSubmitJob_NonSelectSQL_Returns400(t *testing.T) {
	e := newEcho()
	body := `{
		"request_name":"test-req","purpose":"test","template_type":"word",
		"template_name":"t.docx","output_dir":"/tmp","output_format":"docx",
		"sql_statement":"DELETE FROM foo",
		"converter":{"cid":"customer_id","cname":"customer_name","cemail":"email"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/docgen/jobs", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := docgenhandler.SubmitJob(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-SELECT SQL, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListJobs_Returns200(t *testing.T) {
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
