package docreviews

import (
	"context"
	"encoding/json"
	"testing"
)

// TestRegenerateReport_UsesReportsOwnRun reproduces a bug where regenerating a
// report whose request has since started a newer run would stamp the
// regenerated report with the request's *latest* run id instead of the run id
// the report actually belongs to (kb.doc_review_reports.run_id). This mismatch
// also leaked into the generated Typst PDF filename.
func TestRegenerateReport_UsesReportsOwnRun(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ctx := context.Background()

	var requestID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO kb.doc_review_requests
			(input_record_id, tier, aspects, requester_name, requester_id, status)
		VALUES ($1, 'standard', '[]', 'test', 1, 'accepted')
		RETURNING id`, int64(1)).Scan(&requestID); err != nil {
		t.Fatalf("insert request: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM kb.doc_review_requests WHERE id = $1`, requestID)

	insertRun := func() int64 {
		var id int64
		if err := db.QueryRowContext(ctx, `
			INSERT INTO kb.doc_review_runs (request_id, input_record_id, aspects, status)
			VALUES ($1, $2, '[]', 'completed')
			RETURNING id`, requestID, int64(1)).Scan(&id); err != nil {
			t.Fatalf("insert run: %v", err)
		}
		return id
	}

	// The report under test belongs to the older run...
	oldRunID := insertRun()
	// ...but the request has since started a newer run (e.g. a later re-run).
	newRunID := insertRun()
	if newRunID <= oldRunID {
		t.Fatalf("expected newRunID > oldRunID, got new=%d old=%d", newRunID, oldRunID)
	}
	defer db.ExecContext(ctx, `DELETE FROM kb.doc_review_runs WHERE request_id = $1`, requestID)

	insertTestFinding(t, db, 1, oldRunID, FindingItem{
		Pass: "p1", Aspect: "a1", Severity: "high", FindingType: "metrics",
		Title: "old-run finding", ReviewStatus: "pending",
	})
	defer db.ExecContext(ctx, `DELETE FROM kb.doc_review_findings WHERE run_id = $1`, oldRunID)

	var reportID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO kb.doc_review_reports
			(request_id, input_record_id, run_id, report_json, report_markdown,
			 executive_summary, total_findings, overall_assessment)
		VALUES ($1, $2, $3, '{}', '', '', 0, '')
		RETURNING id`, requestID, int64(1), oldRunID).Scan(&reportID); err != nil {
		t.Fatalf("insert report: %v", err)
	}
	defer db.ExecContext(ctx, `DELETE FROM kb.doc_review_reports WHERE id = $1`, reportID)

	c := &DocReviewController{DB: db}
	if err := c.RegenerateReport(ctx, reportID); err != nil {
		t.Fatalf("RegenerateReport: %v", err)
	}

	var reportJSON []byte
	if err := db.QueryRowContext(ctx, `SELECT report_json FROM kb.doc_review_reports WHERE id = $1`, reportID).Scan(&reportJSON); err != nil {
		t.Fatalf("reload report: %v", err)
	}
	var skeleton ReportSkeleton
	if err := json.Unmarshal(reportJSON, &skeleton); err != nil {
		t.Fatalf("unmarshal report_json: %v", err)
	}

	if skeleton.Meta.RunID != oldRunID {
		t.Errorf("Meta.RunID = %d, want %d (the report's own run, not the request's latest run %d)",
			skeleton.Meta.RunID, oldRunID, newRunID)
	}
}
