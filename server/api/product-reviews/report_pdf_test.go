package productreviews

import (
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestExportProductReviewReportPDF(t *testing.T) {
	for _, tc := range []struct {
		name, id, status string
		missing          bool
		want             int
	}{
		{name: "invalid ID", id: "bad", want: http.StatusBadRequest},
		{name: "missing run", id: "42", missing: true, want: http.StatusNotFound},
		{name: "running", id: "42", status: "running", want: http.StatusConflict},
		{name: "failed", id: "42", status: "failed", want: http.StatusConflict},
		{name: "legacy run without markdown", id: "42", status: "completed", want: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.want == http.StatusOK {
				for _, tool := range []string{"typst"} {
					if _, err := exec.LookPath(tool); err != nil {
						t.Skipf("PDF integration test requires %s", tool)
					}
				}
				template, _ := filepath.Abs("../../../docs/doc-templates/template-document-report.typ")
				t.Setenv("DOC_REVIEW_TEMPLATE_FILENAME", template)
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
				rows := runGetRow(42, 1, 1, tc.status, `{"profile_id":7,"profile_version":3}`)
				if tc.missing {
					rows = sqlmock.NewRows(rowsColumnsForPDFTest())
				}
				mock.ExpectQuery("SELECT id, request_id, run_number, status").WithArgs(int64(42)).WillReturnRows(rows)
			}
			if tc.want == http.StatusOK {
				mock.ExpectQuery("FROM kb.product_review_requests").WithArgs(int64(1)).WillReturnRows(requestReturnRow(1, 7, 9))
				mock.ExpectQuery("FROM kb.product_profiles").WithArgs(int64(7)).WillReturnRows(profileRows(7, "Demo product", 9, "ready", false, 0))
				mock.ExpectQuery("FROM kb.product_profile_nodes").WithArgs(int64(7)).WillReturnRows(nodeRowsFrom(7))
				mock.ExpectQuery("FROM kb.product_review_results").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"artifact_id"}))
				mock.ExpectQuery("FROM kb.product_review_run_documents").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}))
				mock.ExpectQuery("SELECT r.artifact_id, to_jsonb").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"artifact_id", "metric"}))
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
