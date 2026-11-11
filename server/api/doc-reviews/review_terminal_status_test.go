package docreviews

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReviewCompletionReturnsEachTerminalWriteFailure(t *testing.T) {
	for _, failingWrite := range []string{"read status", "aspects", "run", "request"} {
		t.Run(failingWrite, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			want := errors.New("terminal status unavailable")
			read := mock.ExpectQuery(`SELECT status FROM kb.doc_review_requests WHERE id = \$1`).WithArgs(int64(11))
			if failingWrite == "read status" {
				read.WillReturnError(want)
			} else {
				read.WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("running"))
				for _, write := range []struct {
					name, query string
				}{
					{"aspects", `UPDATE kb.doc_review_status s`},
					{"run", `UPDATE kb.doc_review_runs SET status = 'completed'.*AND status = 'running'`},
					{"request", `UPDATE kb.doc_review_requests SET status = 'completed'.*AND status = 'running'`},
				} {
					expect := mock.ExpectExec(write.query)
					if failingWrite == write.name {
						expect.WillReturnError(want)
						break
					}
					expect.WillReturnResult(sqlmock.NewResult(0, 1))
				}
			}
			if err := (&DocReviewController{DB: db}).completeReviewRun(context.Background(), 11, 21, 42); !errors.Is(err, want) {
				t.Fatalf("terminal write failure reported as success: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReviewCompletionPreservesStoppedRequest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT status FROM kb.doc_review_requests`).WithArgs(int64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"status"}).AddRow("stopped"))
	if err := (&DocReviewController{DB: db}).completeReviewRun(context.Background(), 11, 21, 42); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewFailureCleanupSurvivesCancellationAndReportsAllWriteErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := errors.New("model failed")
	runErr, requestErr, aspectErr := errors.New("run write failed"), errors.New("request write failed"), errors.New("aspect write failed")
	// All three writes must run even when the review context is cancelled or
	// an earlier cleanup write fails. Terminal stopped rows are excluded.
	mock.ExpectExec(`UPDATE kb.doc_review_runs SET status = 'failed'.*AND status <> 'stopped'`).
		WithArgs(want.Error(), int64(21)).WillReturnError(runErr)
	mock.ExpectExec(`UPDATE kb.doc_review_requests SET status = 'failed'.*AND status <> 'stopped'`).
		WithArgs(int64(11)).WillReturnError(requestErr)
	mock.ExpectExec(`UPDATE kb.doc_review_status`).WithArgs(int64(21), want.Error()).WillReturnError(aspectErr)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = (&DocReviewController{DB: db}).failReviewRun(ctx, 11, 21, want)
	for _, expected := range []error{want, runErr, requestErr, aspectErr} {
		if !errors.Is(err, expected) {
			t.Errorf("failure lost %v: %v", expected, err)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
