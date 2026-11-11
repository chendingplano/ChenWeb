package docreviews

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAcceptRequestRejectsUnsupportedSelectedReferencesBeforePersistence(t *testing.T) {
	// A nonempty selection must never be accepted and silently dropped, even
	// when a caller submits directly rather than using the disabled UI.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = (&DocReviewController{DB: db}).AcceptRequest(context.Background(), SubmitRequestInput{
		InputRecordID: 42,
		ReferenceDocs: []ReferenceDoc{{RecordID: 99, Title: "Supporting policy"}},
	})
	var requestErr *RequestError
	if !errors.As(err, &requestErr) || requestErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("selected references accepted: %v", err)
	}
	if !strings.Contains(requestErr.Message, "reference_docs") {
		t.Fatalf("error does not explain how to proceed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRunReviewFailsHistoricalRequestWithUnsupportedSelectedReferences(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`(?s)SELECT r\.id, r\.input_record_id, r\.review_depth.*FROM kb\.doc_review_requests`).
		WithArgs(int64(11)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "input_record_id", "review_depth", "tier", "aspects", "reference_docs", "notes", "model_overrides",
		"requester_name", "requester_id", "report_template", "doc_template", "status", "create_time",
		"run_id", "run_status", "start_time", "end_time", "error_message",
	}).AddRow(11, 42, 1, "custom", `["legal_compliance"]`, `[{"record_id":99,"title":"Supporting policy"}]`, "", `{}`,
		"user", 0, "", "", "accepted", "2026-11-11", nil, "", "", "", ""))
	mock.ExpectQuery(`SELECT llm_run_id, user_id FROM kb.doc_review_runs`).WithArgs(int64(21), int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"llm_run_id", "user_id"}).AddRow(0, "actor"))
	mock.ExpectExec(`(?s)UPDATE kb.doc_review_runs SET status = 'running'`).WithArgs(int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE kb.doc_review_requests SET status = 'running'`).WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE kb.doc_review_status.*SET status = 'running'`).WithArgs(int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE kb.doc_review_runs SET status = 'failed'`).WithArgs(sqlmock.AnyArg(), int64(21)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE kb.doc_review_requests SET status = 'failed'`).WithArgs(int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)UPDATE kb.doc_review_status.*SET status = 'failed'`).WithArgs(int64(21), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	err = (&DocReviewController{DB: db}).RunReview(context.Background(), 11, 21)
	if err == nil || !strings.Contains(err.Error(), "Supporting-document selection is currently unavailable") {
		t.Fatalf("historical reference selection silently ignored: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
