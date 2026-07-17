package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDocReviewLogSQLStore_ListDocReviewLogs_AppliesAllFiltersAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	start := time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 17, 11, 0, 0, 0, time.UTC)
	inputRecordID, runID := int64(91), int64(7)
	filter := DocReviewLogFilter{InputRecordID: &inputRecordID, RunID: &runID, Pass: "P5", Aspect: "metrics", UnitType: "metric", Outcome: "findings_emitted", CreateTimeStart: &start, CreateTimeEnd: &end, Page: 2, PageSize: 25}
	where := "input_record_id = $1 AND run_id = $2 AND pass = $3 AND aspect = $4 AND unit_type = $5 AND outcome = $6 AND create_time >= $7 AND create_time <= $8"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_logs WHERE "+where)).WithArgs(inputRecordID, runID, "P5", "metrics", "metric", "findings_emitted", start, end).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(26))
	mock.ExpectQuery(`SELECT id, input_record_id, run_id, pass, aspect, unit_type, unit_key,.*FROM kb\.doc_review_logs\s+WHERE input_record_id = \$1 AND run_id = \$2 AND pass = \$3 AND aspect = \$4 AND unit_type = \$5 AND outcome = \$6 AND create_time >= \$7 AND create_time <= \$8\s+ORDER BY create_time DESC, id DESC\s+LIMIT \$9 OFFSET \$10`).WithArgs(inputRecordID, runID, "P5", "metrics", "metric", "findings_emitted", start, end, 25, 25).WillReturnRows(sqlmock.NewRows([]string{"id", "input_record_id", "run_id", "pass", "aspect", "unit_type", "unit_key", "unit_location", "matched_units", "findings", "outcome", "detail", "create_time"}).AddRow(42, 91, 7, "P5", "metrics", "metric", "91_mtc_1", `{"line":12}`, nil, `[{"title":"Mismatch"}]`, "findings_emitted", `{"source":"review"}`, "2026-07-17T10:30:00+00:00"))

	rows, total, err := (DocReviewLogSQLStore{DB: db}).ListDocReviewLogs(context.Background(), filter)
	if err != nil || total != 26 || len(rows) != 1 {
		t.Fatalf("rows=%#v total=%d err=%v", rows, total, err)
	}
	if string(rows[0].UnitLocation) != `{"line":12}` || rows[0].MatchedUnits != nil || string(rows[0].Findings) != `[{"title":"Mismatch"}]` || string(rows[0].Detail) != `{"source":"review"}` {
		t.Fatalf("JSON values=%#v", rows[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocReviewLogSQLStore_ListDocReviewLogs_ReturnsCountError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_logs ")).WillReturnError(errors.New("database unavailable"))
	rows, total, err := (DocReviewLogSQLStore{DB: db}).ListDocReviewLogs(context.Background(), DocReviewLogFilter{})
	if err == nil || rows != nil || total != 0 {
		t.Fatalf("rows=%#v total=%d err=%v", rows, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

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
