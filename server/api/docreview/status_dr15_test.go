package docreview

import (
	"context"
	"testing"
)

func containsJob(jobs []ActiveJob, requestID int64) bool {
	for _, j := range jobs {
		if j.RequestID == requestID {
			return true
		}
	}
	return false
}

// DR15: accepting a request seeds one pending status row per aspect; the job
// appears in ListActiveJobs; once all aspects finish it drops out of the active
// list and its aspects are marked success.
func TestDR15_SeedActiveAndFinish(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)
	ctx := context.Background()
	ctrl := &DocReviewController{DB: db}

	recID := insertTestInput(t, db, "DR15 doc")
	defer cleanupInputs(t, db, recID)

	res, err := ctrl.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recID,
		Tier:          "custom",
		Aspects:       []string{"grammar_spelling", "completeness"},
		RequesterName: "tester",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer cleanupRequests(t, db, res.RequestID)
	defer db.ExecContext(ctx, `DELETE FROM kb.doc_review_status WHERE request_id = $1`, res.RequestID)

	if res.ReviewRunID == "" {
		t.Fatal("expected review_run_id assigned at accept")
	}

	// Status rows seeded as pending.
	statuses, err := ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	if err != nil {
		t.Fatalf("load statuses: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("want 2 status rows, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.Status != "pending" {
			t.Fatalf("aspect %q: want pending, got %q", s.Aspect, s.Status)
		}
	}

	// The job is listed as active (has unfinished aspects).
	jobs, err := ctrl.ListActiveJobs(ctx)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if !containsJob(jobs, res.RequestID) {
		t.Fatalf("active jobs should include request %d", res.RequestID)
	}

	// Finishing all aspects removes the job from the active list.
	ctrl.finalizeAspectsSuccess(ctx, res.ReviewRunID, recID)
	jobs2, err := ctrl.ListActiveJobs(ctx)
	if err != nil {
		t.Fatalf("list active (2): %v", err)
	}
	if containsJob(jobs2, res.RequestID) {
		t.Fatalf("request %d should be absent from active jobs after finishing", res.RequestID)
	}
	final, _ := ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	for _, s := range final {
		if s.Status != "success" {
			t.Fatalf("aspect %q: want success, got %q", s.Aspect, s.Status)
		}
	}
}

// DR15: failOpenAspects (used on stop / whole-run failure) drives every
// non-finished aspect to failed, removing the job from the monitor.
func TestDR15_FailOpenAspects(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)
	ctx := context.Background()
	ctrl := &DocReviewController{DB: db}

	recID := insertTestInput(t, db, "DR15 stop doc")
	defer cleanupInputs(t, db, recID)

	res, err := ctrl.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recID,
		Tier:          "custom",
		Aspects:       []string{"grammar_spelling"},
		RequesterName: "tester",
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer cleanupRequests(t, db, res.RequestID)
	defer db.ExecContext(ctx, `DELETE FROM kb.doc_review_status WHERE request_id = $1`, res.RequestID)

	ctrl.failOpenAspects(ctx, res.ReviewRunID, "stopped")

	if jobs, _ := ctrl.ListActiveJobs(ctx); containsJob(jobs, res.RequestID) {
		t.Fatalf("stopped request %d should be absent from active jobs", res.RequestID)
	}
	final, _ := ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	for _, s := range final {
		if s.Status != "failed" || s.ErrorMessage != "stopped" {
			t.Fatalf("aspect %q: want failed/stopped, got %q/%q", s.Aspect, s.Status, s.ErrorMessage)
		}
	}
}
