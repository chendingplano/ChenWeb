package docreviews

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

var summaryColumns = []string{"id", "pass", "aspect", "severity", "finding_type", "title", "location", "line_start", "line_end",
	"review_status", "proc_status", "confidence", "artifact_id", "storage_version", "activity_count", "search_result_count",
	"selected_candidate_count", "relevant_candidate_count", "judged_candidate_count", "verdict_counts", "sev_rank"}

func TestListRunFindingSummariesPagesWithACursor(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM kb.doc_review_findings`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))
	// A limit of 2 asks for 3 rows; the third proves another page exists.
	mock.ExpectQuery(`SELECT id, pass, aspect, .* ORDER BY CASE severity .* LIMIT \$7`).
		WithArgs(int64(116), "compliance", sqlmock.AnyArg(), true, int64(2), int64(11900), 3).
		WillReturnRows(sqlmock.NewRows(summaryColumns).
			AddRow(11901, "P5", "compliance", "medium", "compliance_assessment", "A", "1-2", 1, 2, "pending", "success", 0.5, "", 2, 1, 60, 30, 4, 4, `{"compatible":4}`, 2).
			AddRow(11902, "P5", "compliance", "info", "compliance_no_grounding", "B", "5-6", 0, 0, "pending", "success", 0.5, "", 1, nil, nil, nil, nil, nil, `{}`, 4).
			AddRow(11903, "P5", "compliance", "info", "compliance_no_grounding", "C", "7", 7, 7, "pending", "success", 0.5, "", 2, 1, 0, 0, 0, 0, `{}`, 4))
	ctrl := &DocReviewController{DB: db}
	page, err := ctrl.ListRunFindingSummaries(context.Background(), 116, "compliance", []string{"deleted"}, "2:11900", 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 3 || len(page.Items) != 2 || page.NextCursor != "4:11902" {
		t.Fatalf("page = %+v", page)
	}
	if c := page.Items[0].Counts; c == nil || c.Judged != 4 || c.Verdicts["compatible"] != 4 || c.SearchResults != 60 {
		t.Errorf("counts = %+v", page.Items[0].Counts)
	}
	// A legacy finding has no counters; its line range comes from its location.
	if b := page.Items[1]; b.Counts != nil || b.LineStart != 5 || b.LineEnd != 6 {
		t.Errorf("legacy summary = %+v", b)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestComplianceReadRejectsBadInput(t *testing.T) {
	ctrl := &DocReviewController{}
	if _, err := ctrl.ListRunFindingSummaries(context.Background(), 1, "compliance", nil, "x", 0); !errors.Is(err, ErrComplianceBadRequest) {
		t.Errorf("bad cursor: %v", err)
	}
	if _, err := ctrl.ListRunFindingSummaries(context.Background(), 1, "", nil, "", 0); !errors.Is(err, ErrComplianceBadRequest) {
		t.Errorf("missing aspect: %v", err)
	}
	if _, err := ctrl.ListFindingCandidates(context.Background(), 1, "kept-ish", "", 0); !errors.Is(err, ErrComplianceBadRequest) {
		t.Errorf("bad filter: %v", err)
	}
	if _, err := ctrl.ListSearchResults(context.Background(), 1, "two", 0); !errors.Is(err, ErrComplianceBadRequest) {
		t.Errorf("bad rank cursor: %v", err)
	}
	if complianceLimit(0, 25) != 25 || complianceLimit(500, 25) != compliancePageMax {
		t.Error("limits are not bounded")
	}
}

// A finding that was not migrated returns its stored content and nothing from the new tables.
func TestGetFindingDetailsLegacyPath(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`FROM kb.doc_review_findings WHERE id = \$1`).WithArgs(int64(11502)).WillReturnRows(sqlmock.NewRows(
		[]string{"id", "run_id", "pass", "aspect", "severity", "finding_type", "title", "location", "line_start", "line_end", "review_status",
			"proc_status", "confidence", "artifact_id", "storage_version", "description", "evidence", "suggestion", "error_msg",
			"activity_count", "search_result_count", "selected_candidate_count", "relevant_candidate_count", "judged_candidate_count", "verdict_counts"}).
		AddRow(11502, 98, "P5", "compliance", "info", "compliance_grounding_candidates", "t", "18-18", 0, 0, "pending", "", 0.5, "", 1,
			"Retrieved 25 candidate artifact(s)", "ev", "", "", nil, nil, nil, nil, nil, "{}"))
	d, err := (&DocReviewController{DB: db}).GetFindingDetails(context.Background(), 11502, FindingDetailsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Legacy == nil || d.Legacy.Description != "Retrieved 25 candidate artifact(s)" || d.Activities != nil || d.Finding.LineStart != 18 {
		t.Fatalf("details = %+v", d)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

func TestListFindingCandidatesCursorAndFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := []string{"id", "activity_id", "label", "local_label", "candidate_type", "candidate_id", "candidate_record_id", "document_title",
		"relevance_answer", "relevance_score", "kept", "status", "verdict", "binding", "summary", "note", "display_order"}
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM kb.doc_review_compliance_candidates c .* AND c.status = 'judged'`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(`FROM kb.doc_review_compliance_candidates c .* c.status = 'judged' AND .* ORDER BY c.display_order, c.id LIMIT \$5`).
		WithArgs(int64(11970), false, int64(0), int64(0), 2).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, 5000, "run test", "a1", "provision", "812_prv_167", 812, "GB/T 25127.2", "yes", 7, true, "judged", "compatible", "mandatory", "s", "", 0).
			AddRow(2, 5000, "run test", "a1", "provision", "811_prv_161", 811, "GB/T 25127.1", "yes", 6, true, "judged", "compatible", "mandatory", "s", "", 1))
	page, err := (&DocReviewController{DB: db}).ListFindingCandidates(context.Background(), 11970, "", "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.NextCursor != "0:1" || *page.Items[0].RelevanceScore != 7 {
		t.Fatalf("page = %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}
