package docprocessing

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDocReviewLogSQLStore_ListDocReviewLogs_EscapesUnitKey(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_logs WHERE unit_key ILIKE $1 ESCAPE '\\'")).WithArgs("%a\\_b\\%c\\\\d%").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, input_record_id, run_id, pass, aspect, unit_type, unit_key,.*COALESCE\(to_char\(create_time, 'YYYY-MM-DD"T"HH24:MI:SSTZH:TZM'\), ''\).*FROM kb\.doc_review_logs\s+WHERE unit_key ILIKE \$1 ESCAPE '\\'\s+ORDER BY create_time DESC, id DESC\s+LIMIT \$2 OFFSET \$3`).WithArgs("%a\\_b\\%c\\\\d%", 50, 0).WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "run_id", "pass", "aspect", "unit_type", "unit_key", "unit_location", "matched_units", "findings", "outcome", "detail", "create_time"}).AddRow(1, 2, 3, "P5", "metrics", "metric", "a_b%c\\d", nil, nil, nil, "no_findings", nil, "2026-07-17T10:30:00+00:00"))
	rows, total, err := (DocReviewLogSQLStore{DB: db}).ListDocReviewLogs(context.Background(), DocReviewLogFilter{UnitKey: "a_b%c\\d"})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].CreateTime != "2026-07-17T10:30:00+00:00" {
		t.Fatalf("rows=%#v total=%d err=%v", rows, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
