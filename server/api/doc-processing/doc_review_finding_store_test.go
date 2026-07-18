package docprocessing

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestDocReviewFindingSQLStore_ListDocReviewFindings_AppliesAllFiltersAndPagination(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	inputRecordID, runID := int64(91), int64(7)
	filter := DocReviewFindingFilter{
		InputRecordID: &inputRecordID,
		RunID:         &runID,
		Pass:          "P5",
		Aspect:        "metrics",
		Severity:      "high",
		ReviewStatus:  "pending",
		FindingType:   "issue",
		ArtifactID:    "91_mtc",
		Title:         "Mismatch",
		Page:          2,
		PageSize:      25,
	}
	where := "input_record_id = $1 AND run_id = $2 AND pass = $3 AND aspect = $4 AND severity = $5 AND review_status = $6 AND finding_type = $7 AND artifact_id ILIKE $8 ESCAPE '\\' AND title ILIKE $9 ESCAPE '\\'"
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_findings WHERE "+where)).
		WithArgs(inputRecordID, runID, "P5", "metrics", "high", "pending", "issue", "%91\\_mtc%", "%Mismatch%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(26))
	mock.ExpectQuery(`SELECT id, input_record_id, run_id, pass, aspect, severity, finding_type, title,.*FROM kb\.doc_review_findings\s+WHERE input_record_id = \$1 AND run_id = \$2 AND pass = \$3 AND aspect = \$4 AND severity = \$5 AND review_status = \$6 AND finding_type = \$7 AND artifact_id ILIKE \$8 ESCAPE '\\' AND title ILIKE \$9 ESCAPE '\\'\s+ORDER BY run_id DESC, id ASC\s+LIMIT \$10 OFFSET \$11`).
		WithArgs(inputRecordID, runID, "P5", "metrics", "high", "pending", "issue", "%91\\_mtc%", "%Mismatch%", 25, 25).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "input_record_id", "run_id", "pass", "aspect", "severity", "finding_type", "title",
			"description", "evidence", "location", "suggestion", "confidence", "review_status",
			"artifact_id", "metadata", "reference_doc",
		}).AddRow(42, 91, 7, "P5", "metrics", "high", "issue", "Mismatch", "desc", "ev", "loc", "sugg", 0.9, "pending", "91_mtc_1", `{"run_id":7}`, `{"record_id":12}`))

	rows, total, err := (DocReviewFindingSQLStore{DB: db}).ListDocReviewFindings(context.Background(), filter)
	if err != nil || total != 26 || len(rows) != 1 {
		t.Fatalf("rows=%#v total=%d err=%v", rows, total, err)
	}
	if string(rows[0].Metadata) != `{"run_id":7}` || string(rows[0].ReferenceDoc) != `{"record_id":12}` {
		t.Fatalf("JSON values=%#v", rows[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocReviewFindingSQLStore_ListDocReviewFindings_ReturnsCountError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_findings ")).WillReturnError(errors.New("database unavailable"))

	rows, total, err := (DocReviewFindingSQLStore{DB: db}).ListDocReviewFindings(context.Background(), DocReviewFindingFilter{})
	if err == nil || rows != nil || total != 0 {
		t.Fatalf("rows=%#v total=%d err=%v", rows, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestDocReviewFindingSQLStore_ListDocReviewFindings_EscapesLikeFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM kb.doc_review_findings WHERE artifact_id ILIKE $1 ESCAPE '\\' AND title ILIKE $2 ESCAPE '\\'")).
		WithArgs("%a\\_b\\%c\\\\d%", "%x\\_y\\%z\\\\q%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, input_record_id, run_id, pass, aspect, severity, finding_type, title,.*FROM kb\.doc_review_findings\s+WHERE artifact_id ILIKE \$1 ESCAPE '\\' AND title ILIKE \$2 ESCAPE '\\'\s+ORDER BY run_id DESC, id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs("%a\\_b\\%c\\\\d%", "%x\\_y\\%z\\\\q%", 50, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "input_record_id", "run_id", "pass", "aspect", "severity", "finding_type", "title",
			"description", "evidence", "location", "suggestion", "confidence", "review_status",
			"artifact_id", "metadata", "reference_doc",
		}).AddRow(1, 2, 3, "P5", "metrics", "low", "analysis", "x_y%z\\q", "", "", "", "", 0, "pending", "a_b%c\\d", nil, nil))

	rows, total, err := (DocReviewFindingSQLStore{DB: db}).ListDocReviewFindings(context.Background(), DocReviewFindingFilter{
		ArtifactID: "a_b%c\\d",
		Title:      "x_y%z\\q",
	})
	if err != nil || total != 1 || len(rows) != 1 || rows[0].ArtifactID != "a_b%c\\d" {
		t.Fatalf("rows=%#v total=%d err=%v", rows, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
