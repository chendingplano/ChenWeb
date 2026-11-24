package productreviews

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestExportProductReviewReportPDF(t *testing.T) {
	for _, tc := range []struct {
		name, id, status, markdown string
		missing                    bool
		want                       int
	}{
		{name: "invalid ID", id: "bad", want: http.StatusBadRequest},
		{name: "missing run", id: "42", missing: true, want: http.StatusNotFound},
		{name: "running", id: "42", status: "running", want: http.StatusConflict},
		{name: "failed", id: "42", status: "failed", want: http.StatusConflict},
		{name: "empty report", id: "42", status: "completed", want: http.StatusConflict},
		{name: "PDF", id: "42", status: "completed", markdown: "# Product review 产品审查\n\n| Node | Metrics |\n|---|---:|\n| 显示器 | 3 |\n\n## Coverage gaps\n\nNone.\n", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want == http.StatusOK {
				for _, tool := range []string{"pandoc", "typst"} {
					if _, err := exec.LookPath(tool); err != nil {
						t.Skipf("PDF integration test requires %s", tool)
					}
				}
				t.Setenv("DOCUMENT_PDF_ENGINE", "typst")
				t.Setenv("DOCUMENT_PDF_FONT", "Helvetica")
			}
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()
			if tc.id == "42" {
				rows := runGetRow(42, 1, 1, tc.status, "{}")
				if tc.missing {
					rows = sqlmock.NewRows(rowsColumnsForPDFTest())
				} else if tc.markdown != "" {
					rows = sqlmock.NewRows(rowsColumnsForPDFTest()).AddRow(42, 1, 1, tc.status, nil, nil,
						3, 3, 0, 1, 0, []byte(`{}`), tc.markdown, "", time.Now(), time.Now())
				}
				mock.ExpectQuery("SELECT id, request_id, run_number, status").WithArgs(int64(42)).WillReturnRows(rows)
			}
			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
			c.SetParamNames("run_id")
			c.SetParamValues(tc.id)
			if err := ExportProductReviewReportPDF(c); err != nil {
				t.Fatal(err)
			}
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.want == http.StatusOK {
				if !strings.HasPrefix(rec.Body.String(), "%PDF-") || rec.Header().Get("Content-Type") != "application/pdf" {
					t.Fatalf("response is not a PDF: %v", rec.Header())
				}
				if rec.Header().Get("Content-Disposition") != "inline; filename=product-review-run-42.pdf" {
					t.Fatal("PDF must open inline with the run ID in its filename")
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func rowsColumnsForPDFTest() []string {
	return []string{"id", "request_id", "run_number", "status", "started_at", "finished_at",
		"result_count", "attributed_count", "document_scope_count", "scoped_document_count",
		"truncated_count", "report_json", "report_md", "error_message", "created_at", "updated_at"}
}
