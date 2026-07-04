package docreviews

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestReviewLogsSQLStore_SaveLogs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	store := ReviewLogsSQLStore{DB: db}
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.doc_review_logs
    (input_record_id, run_id, pass, aspect, unit_type, unit_key,
     unit_location, matched_units, findings, outcome, detail)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`)).
		WithArgs(
			int64(416),
			int64(28),
			"P5",
			"metrics",
			"metric",
			"416_m_7",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			"findings_emitted",
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	inserted, err := store.SaveLogs(context.Background(), []ReviewLogEntry{{
		InputRecordID: 416,
		RunID:         28,
		Pass:          "P5",
		Aspect:        "metrics",
		UnitType:      "metric",
		UnitKey:       "416_m_7",
		UnitLocation:  map[string]any{"line_spans": []string{"120:124"}},
		MatchedUnits:  []map[string]any{{"metric_id": "2002_m_3"}},
		Findings: []ReviewFinding{{
			Pass:        "P5",
			Aspect:      "metrics",
			Title:       "Conflict",
			Description: "values differ",
		}},
		Outcome: "findings_emitted",
		Detail:  map[string]any{"match_count": 1},
	}})
	if err != nil {
		t.Fatalf("SaveLogs returned error: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("SaveLogs inserted = %d, want 1", inserted)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
