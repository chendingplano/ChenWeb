package kbhandler

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestListSemanticRetryQueueFiltersByState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.semantic_retry_queue q LEFT JOIN kb.semantic_processing_outcomes o ON o.id = q.outcome_id WHERE 1=1 AND q.state = $1")).
		WithArgs("pending").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT q.id, q.outcome_id").
		WithArgs("pending", 50, 0).
		WillReturnRows(retryQueueColumnNames())

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/semantic-retry-queue?state=pending", nil)
	rec := httptest.NewRecorder()
	if err := ListSemanticRetryQueue(e.NewContext(req, rec)); err != nil {
		t.Fatalf("ListSemanticRetryQueue: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func retryQueueColumnNames() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "outcome_id", "finding_id", "target_dependency_fingerprint",
		"source_input_fingerprint", "state", "attempts",
		"lease_token", "lease_expires_at", "last_error",
		"create_time", "modify_time",
		"input_record_id", "artifact_type", "artifact_id", "stage_term_id",
	})
}
