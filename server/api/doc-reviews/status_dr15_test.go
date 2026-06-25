package docreviews

import (
	"context"
	"math"
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
		if s.Progress != 0 {
			t.Fatalf("aspect %q: want progress 0, got %v", s.Aspect, s.Progress)
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
		if s.Progress != 1 {
			t.Fatalf("aspect %q: want progress 1, got %v", s.Aspect, s.Progress)
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

func TestDR15_UpdateAspectProgress(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)
	ctx := context.Background()
	ctrl := &DocReviewController{DB: db}

	recID := insertTestInput(t, db, "DR15 progress doc")
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

	ctrl.markAspectsRunning(ctx, res.ReviewRunID)

	store := ReviewStatusSQLStore{DB: db}
	if err := store.UpdateAspectProgress(ctx, res.ReviewRunID, "grammar_spelling", 0.5, 3); err != nil {
		t.Fatalf("update progress: %v", err)
	}

	statuses, err := ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	if err != nil {
		t.Fatalf("load statuses: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("want 1 status row, got %d", len(statuses))
	}
	got := statuses[0]
	if got.Status != "running" {
		t.Fatalf("want running status, got %q", got.Status)
	}
	if math.Abs(got.Progress-0.5) > 1e-9 {
		t.Fatalf("want progress 0.5, got %v", got.Progress)
	}
	if got.FindingCount != 3 {
		t.Fatalf("want finding count 3, got %d", got.FindingCount)
	}

	if err := store.UpdateAspectProgress(ctx, res.ReviewRunID, "grammar_spelling", 0.25, 1); err != nil {
		t.Fatalf("update progress second time: %v", err)
	}
	statuses, err = ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	if err != nil {
		t.Fatalf("load statuses second time: %v", err)
	}
	got = statuses[0]
	if math.Abs(got.Progress-0.5) > 1e-9 {
		t.Fatalf("progress regressed: got %v", got.Progress)
	}
	if got.FindingCount != 3 {
		t.Fatalf("finding count regressed: got %d", got.FindingCount)
	}

	// Reaching 100% flips the aspect to 'success' immediately, rather than
	// lingering on 'running' until the whole run is finalized.
	if err := store.UpdateAspectProgress(ctx, res.ReviewRunID, "grammar_spelling", 1.0, 5); err != nil {
		t.Fatalf("update progress to 100%%: %v", err)
	}
	statuses, err = ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	if err != nil {
		t.Fatalf("load statuses after completion: %v", err)
	}
	got = statuses[0]
	if got.Status != "success" {
		t.Fatalf("want success status at 100%%, got %q", got.Status)
	}
	if math.Abs(got.Progress-1.0) > 1e-9 {
		t.Fatalf("want progress 1, got %v", got.Progress)
	}
	if got.FindingCount != 5 {
		t.Fatalf("want finding count 5, got %d", got.FindingCount)
	}

	// 'success' is terminal: a stray later progress report must not revert it.
	if err := store.UpdateAspectProgress(ctx, res.ReviewRunID, "grammar_spelling", 0.5, 1); err != nil {
		t.Fatalf("update progress after success: %v", err)
	}
	statuses, err = ctrl.loadAspectStatuses(ctx, res.ReviewRunID)
	if err != nil {
		t.Fatalf("load statuses after stray update: %v", err)
	}
	got = statuses[0]
	if got.Status != "success" {
		t.Fatalf("success regressed to %q", got.Status)
	}
}
