package docreviews

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestAspectProgressAtOneRemainsRunningUntilPersistence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(`(?s)UPDATE kb.doc_review_status\s+SET status = 'running',\s+progress = GREATEST\(progress, \$3\),\s+finding_count = GREATEST\(finding_count, \$4\),\s+modify_time = NOW\(\)\s+WHERE run_id = \$1 AND aspect = \$2 AND status NOT IN \('success', 'failed'\)`).
		WithArgs(int64(21), "legal_compliance", 1.0, 3).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := (ReviewStatusSQLStore{DB: db}).UpdateAspectProgress(context.Background(), 21, "legal_compliance", 1, 3); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFinishAspectPersistsSuccessOrFailureWithDurableFindingCount(t *testing.T) {
	for _, tc := range []struct {
		name, status, message string
		reviewErr             error
	}{
		{name: "saved findings", status: "success"},
		{name: "model failed", status: "failed", message: "model unavailable", reviewErr: errors.New("model unavailable")},
		{name: "save failed", status: "failed", message: "findings insert failed", reviewErr: errors.New("findings insert failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectExec(`(?s)UPDATE kb.doc_review_status s.*SET status = \$3,.*finding_count = \(SELECT COUNT\(\*\) FROM kb.doc_review_findings f.*f.run_id = s.run_id AND f.aspect = s.aspect.*error_message = \$4,.*s.status NOT IN \('success', 'failed'\)`).
				WithArgs(int64(21), "legal_compliance", tc.status, tc.message).
				WillReturnResult(sqlmock.NewResult(0, 1))
			if err := (ReviewStatusSQLStore{DB: db}).FinishAspect(context.Background(), 21, "legal_compliance", tc.reviewErr); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type failingProgressStore struct{ err error }

func (s failingProgressStore) UpdateAspectProgress(context.Context, int64, string, float64, int) error {
	return s.err
}

func TestProgressPersistenceFailureReachesReviewer(t *testing.T) {
	want := errors.New("status database unavailable")
	p := ReviewProcessor{StatusStore: failingProgressStore{err: want}, Logger: &captureLogger{}}
	tracker := newReviewerProgressTracker(1, p.makeProgressReporter(21, "legal_compliance"))
	if err := tracker.add(0); !errors.Is(err, want) {
		t.Fatalf("progress persistence failure swallowed: %v", err)
	}
}

func TestProgressPersistenceFailureStopsRemainingReviewerUnits(t *testing.T) {
	want := errors.New("status database unavailable")
	p := ReviewProcessor{StatusStore: failingProgressStore{err: want}, Logger: &captureLogger{}}
	calls := 0
	_, err := runReviewerConcurrent(context.Background(), 1, 3,
		ReviewerConfig{MaxFindings: 10, MaxAnalyses: 10}, "legal_compliance", p.Logger, 42,
		p.makeProgressReporter(21, "legal_compliance"),
		func(context.Context, int) ([]ReviewFinding, error) {
			calls++
			return nil, nil
		})
	if !errors.Is(err, want) {
		t.Fatalf("reviewer hid status persistence failure: %v", err)
	}
	if calls != 1 {
		t.Fatalf("reviewer continued after progress persistence failure: %d calls", calls)
	}
}

func TestFinishAspectReturnsPersistenceFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := errors.New("status write unavailable")
	mock.ExpectExec(`UPDATE kb.doc_review_status`).WillReturnError(want)
	if err := (ReviewStatusSQLStore{DB: db}).FinishAspect(context.Background(), 21, "legal_compliance", nil); !errors.Is(err, want) {
		t.Fatalf("completion write failure swallowed: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
