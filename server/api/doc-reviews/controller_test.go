package docreviews

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
)

// connectTestDB opens a test PostgreSQL connection or skips the test.
func connectTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		dsn = "postgres://localhost:5432/deepdoc?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("No test database available: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Skipf("Test database not reachable: %v", err)
	}
	return db
}

// ensureTables creates the minimal schema needed for controller tests if it
// doesn't already exist. This avoids requiring migration state for tests.
func ensureTables(t *testing.T, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE SCHEMA IF NOT EXISTS kb`,
		`CREATE TABLE IF NOT EXISTS kb.inputs (
			id BIGSERIAL PRIMARY KEY,
			title TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS kb.doc_review_requests (
			id BIGSERIAL PRIMARY KEY,
			input_record_id BIGINT NOT NULL,
			tier TEXT NOT NULL DEFAULT '',
			aspects JSONB DEFAULT '[]',
			reference_docs JSONB DEFAULT '[]',
			notes TEXT DEFAULT '',
			model_overrides JSONB DEFAULT '{}',
			requester_name TEXT DEFAULT '',
			requester_id BIGINT DEFAULT 0,
			report_template TEXT DEFAULT '',
			doc_template TEXT DEFAULT '',
			status TEXT NOT NULL DEFAULT 'accepted',
			create_time TIMESTAMPTZ DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS kb.doc_review_runs (
			id BIGSERIAL PRIMARY KEY,
			request_id BIGINT NOT NULL,
			input_record_id BIGINT NOT NULL,
			run_number INT NOT NULL DEFAULT 1,
			aspects JSONB NOT NULL DEFAULT '[]',
			model_overrides JSONB,
			notes TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			created_by TEXT,
			create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			start_time TIMESTAMPTZ,
			end_time TIMESTAMPTZ,
			error_message TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS kb.doc_review_findings (
			id BIGSERIAL PRIMARY KEY,
			input_record_id BIGINT NOT NULL DEFAULT 0,
			run_id BIGINT NOT NULL DEFAULT 0,
			pass TEXT NOT NULL DEFAULT '',
			aspect TEXT NOT NULL DEFAULT '',
			severity TEXT NOT NULL DEFAULT 'medium',
			finding_type TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			description TEXT DEFAULT '',
			evidence TEXT DEFAULT '',
			location TEXT DEFAULT '',
			suggestion TEXT DEFAULT '',
			confidence DOUBLE PRECISION DEFAULT 0,
			metadata JSONB DEFAULT '{}',
			review_status TEXT DEFAULT 'pending',
			reviewed_by TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS kb.doc_review_status (
			id BIGSERIAL PRIMARY KEY,
			request_id BIGINT NOT NULL,
			input_record_id BIGINT NOT NULL,
			run_id BIGINT NOT NULL,
			aspect TEXT NOT NULL,
			pass TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			progress DOUBLE PRECISION NOT NULL DEFAULT 0,
			finding_count INT NOT NULL DEFAULT 0,
			error_message TEXT,
			start_time TIMESTAMPTZ,
			end_time TIMESTAMPTZ,
			create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (run_id, aspect)
		)`,
		`CREATE TABLE IF NOT EXISTS kb.doc_review_reports (
			id BIGSERIAL PRIMARY KEY,
			request_id BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS kb.doc_review_logs (
			id BIGSERIAL PRIMARY KEY,
			input_record_id BIGINT NOT NULL,
			run_id BIGINT NOT NULL,
			pass TEXT NOT NULL DEFAULT '',
			aspect TEXT NOT NULL DEFAULT '',
			unit_type TEXT NOT NULL DEFAULT '',
			unit_key TEXT NOT NULL DEFAULT '',
			unit_location JSONB DEFAULT '{}'::jsonb,
			matched_units JSONB DEFAULT '[]'::jsonb,
			findings JSONB DEFAULT '[]'::jsonb,
			outcome TEXT NOT NULL DEFAULT '',
			detail JSONB DEFAULT '{}'::jsonb,
			create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (run_id, aspect, unit_type, unit_key)
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(context.Background(), s); err != nil {
			t.Fatalf("ensure schema: %v", err)
		}
	}
}

// cleanupInputs removes test rows we create.
func cleanupInputs(t *testing.T, db *sql.DB, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		db.ExecContext(context.Background(), `DELETE FROM kb.inputs WHERE id = $1`, id)
	}
}

// cleanupRequests removes test request rows.
func cleanupRequests(t *testing.T, db *sql.DB, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		db.ExecContext(context.Background(), `DELETE FROM kb.doc_review_requests WHERE id = $1`, id)
	}
}

// cleanupFindings removes test finding rows.
func cleanupFindings(t *testing.T, db *sql.DB, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		db.ExecContext(context.Background(), `DELETE FROM kb.doc_review_findings WHERE id = $1`, id)
	}
}

// insertTestInput creates a document input record for testing.
func insertTestInput(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(),
		`INSERT INTO kb.inputs (title) VALUES ($1) RETURNING id`, title,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert test input: %v", err)
	}
	return id
}

// insertTestFinding inserts a finding row directly and returns its ID.
func insertTestFinding(t *testing.T, db *sql.DB, inputRecordID int64, runID int64, finding FindingItem) int64 {
	t.Helper()
	var id int64
	err := db.QueryRowContext(context.Background(), `
		INSERT INTO kb.doc_review_findings
			(input_record_id, run_id, pass, aspect, severity, finding_type,
			 title, description, evidence, location, suggestion, confidence, review_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id`,
		inputRecordID, runID, finding.Pass, finding.Aspect,
		finding.Severity, finding.FindingType, finding.Title,
		finding.Description, finding.Evidence, finding.Location,
		finding.Suggestion, finding.Confidence, finding.ReviewStatus,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert test finding: %v", err)
	}
	return id
}

func TestController_AcceptRequest(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test Accept Request Doc")
	defer cleanupInputs(t, db, recordID)

	result, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}

	if result.Status != "accepted" {
		t.Errorf("Status = %q, want %q", result.Status, "accepted")
	}
	if result.RequestID == 0 {
		t.Error("RequestID is 0")
	}
	if result.RunID == 0 {
		t.Error("RunID is 0")
	}

	// Verify in DB.
	var status string
	err = db.QueryRowContext(ctx, `SELECT status FROM kb.doc_review_requests WHERE id = $1`, result.RequestID).Scan(&status)
	if err != nil {
		t.Fatalf("query request: %v", err)
	}
	if status != "accepted" {
		t.Errorf("DB status = %q, want %q", status, "accepted")
	}

	// Verify run was created.
	var runStatus string
	err = db.QueryRowContext(ctx, `SELECT status FROM kb.doc_review_runs WHERE id = $1`, result.RunID).Scan(&runStatus)
	if err != nil {
		t.Fatalf("query run: %v", err)
	}
	if runStatus != "pending" {
		t.Errorf("run status = %q, want %q", runStatus, "pending")
	}

	// Cleanup.
	cleanupRequests(t, db, result.RequestID)
}

func TestController_IdempotencyGuard(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test Idempotency Doc")
	defer cleanupInputs(t, db, recordID)

	// First request should succeed.
	first, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err != nil {
		t.Fatalf("First AcceptRequest: %v", err)
	}
	defer cleanupRequests(t, db, first.RequestID)

	// Second request for same record while first is still "accepted" should conflict.
	_, err = c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user-2",
	})
	if err == nil {
		t.Fatal("Second AcceptRequest should have returned conflict error, got nil")
	}
	reqErr, ok := err.(*RequestError)
	if !ok {
		t.Fatalf("Second AcceptRequest error type = %T, want *RequestError", err)
	}
	if reqErr.Status != 409 {
		t.Errorf("Error status = %d, want 409", reqErr.Status)
	}
	if reqErr.Message == "" {
		t.Error("Error message is empty")
	}

	// Mark the first request as completed.
	_, err = db.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'completed' WHERE id = $1`,
		first.RequestID,
	)
	if err != nil {
		t.Fatalf("update first request to completed: %v", err)
	}

	// Third request for same record after completion should succeed (no active conflict).
	third, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user-3",
	})
	if err != nil {
		t.Fatalf("Third AcceptRequest after completion: %v", err)
	}
	defer cleanupRequests(t, db, third.RequestID)
	if third.Status != "accepted" {
		t.Errorf("Third request status = %q, want %q", third.Status, "accepted")
	}
}

func TestController_AcceptRequest_DocumentNotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	// Use a non-existent record ID.
	_, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: 999999999,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err == nil {
		t.Fatal("AcceptRequest should have returned error for non-existent document")
	}
	reqErr, ok := err.(*RequestError)
	if !ok {
		t.Fatalf("Error type = %T, want *RequestError", err)
	}
	if reqErr.Status != 422 {
		t.Errorf("Error status = %d, want 422", reqErr.Status)
	}
}

func TestController_StopRequest(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test Stop Request Doc")
	defer cleanupInputs(t, db, recordID)

	// Create a request via AcceptRequest.
	result, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	defer cleanupRequests(t, db, result.RequestID)

	// Manually set it to 'running' so StopRequest will work.
	_, err = db.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'running' WHERE id = $1`,
		result.RequestID,
	)
	if err != nil {
		t.Fatalf("update to running: %v", err)
	}
	_, err = db.ExecContext(ctx,
		`UPDATE kb.doc_review_runs SET status = 'running' WHERE id = $1`,
		result.RunID,
	)
	if err != nil {
		t.Fatalf("update run to running: %v", err)
	}

	// Now stop it.
	err = c.StopRequest(ctx, result.RequestID)
	if err != nil {
		t.Fatalf("StopRequest: %v", err)
	}

	// Verify request status in DB.
	var status string
	err = db.QueryRowContext(ctx,
		`SELECT status FROM kb.doc_review_requests WHERE id = $1`, result.RequestID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("query request: %v", err)
	}
	if status != "stopped" {
		t.Errorf("DB status = %q, want %q", status, "stopped")
	}

	// Stopping again should error.
	err = c.StopRequest(ctx, result.RequestID)
	if err == nil {
		t.Fatal("Second StopRequest should error, got nil")
	}
}

func TestController_RestartRequest_CreatesNewRunForCompletedRequest(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test Restart Completed Request Doc")
	defer cleanupInputs(t, db, recordID)

	result, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	defer cleanupRequests(t, db, result.RequestID)

	findingID := insertTestFinding(t, db, recordID, result.RunID, FindingItem{
		Pass:         "P5",
		Aspect:       "provisions",
		Severity:     "medium",
		FindingType:  "issue",
		Title:        "Stale finding",
		Description:  "Should be cleared before rerun",
		Confidence:   0.9,
		ReviewStatus: "pending",
	})
	defer cleanupFindings(t, db, findingID)

	if _, err := db.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'completed' WHERE id = $1`,
		result.RequestID,
	); err != nil {
		t.Fatalf("update request to completed: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE kb.doc_review_runs
		 SET status = 'completed', start_time = NOW() - interval '1 minute', end_time = NOW(), error_message = 'old error'
		 WHERE id = $1`,
		result.RunID,
	); err != nil {
		t.Fatalf("update run to completed: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE kb.doc_review_status
		 SET status = 'success', progress = 1, finding_count = 1, error_message = 'old error',
		     start_time = NOW() - interval '1 minute', end_time = NOW()
		 WHERE run_id = $1`,
		result.RunID,
	); err != nil {
		t.Fatalf("update aspect statuses to success: %v", err)
	}

	runID, err := c.RestartRequest(ctx, result.RequestID)
	if err != nil {
		t.Fatalf("RestartRequest: %v", err)
	}
	if runID == result.RunID {
		t.Fatalf("RestartRequest runID = %d, want a new run id", runID)
	}

	var requestStatus string
	if err := db.QueryRowContext(ctx,
		`SELECT status FROM kb.doc_review_requests WHERE id = $1`,
		result.RequestID,
	).Scan(&requestStatus); err != nil {
		t.Fatalf("select request status: %v", err)
	}
	if requestStatus != "accepted" {
		t.Errorf("request status = %q, want accepted", requestStatus)
	}

	var runStatus string
	var runStart, runEnd sql.NullTime
	var runError sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT status, start_time, end_time, error_message FROM kb.doc_review_runs WHERE id = $1`,
		result.RunID,
	).Scan(&runStatus, &runStart, &runEnd, &runError); err != nil {
		t.Fatalf("select run status: %v", err)
	}
	if runStatus != "completed" {
		t.Errorf("old run status = %q, want completed", runStatus)
	}
	if !runStart.Valid {
		t.Error("old run start_time should remain set")
	}
	if !runEnd.Valid {
		t.Error("old run end_time should remain set")
	}
	if !runError.Valid || runError.String != "old error" {
		t.Errorf("old run error_message = %q, want %q", runError.String, "old error")
	}

	var newRunNumber int
	var newRunStatus string
	var newRunRecordID int64
	if err := db.QueryRowContext(ctx,
		`SELECT run_number, status, input_record_id FROM kb.doc_review_runs WHERE id = $1`,
		runID,
	).Scan(&newRunNumber, &newRunStatus, &newRunRecordID); err != nil {
		t.Fatalf("select new run row: %v", err)
	}
	if newRunNumber != 2 {
		t.Errorf("new run_number = %d, want 2", newRunNumber)
	}
	if newRunStatus != "pending" {
		t.Errorf("new run status = %q, want pending", newRunStatus)
	}
	if newRunRecordID != recordID {
		t.Errorf("new run input_record_id = %d, want %d", newRunRecordID, recordID)
	}

	var latestRunID int64
	if err := db.QueryRowContext(ctx,
		`SELECT id FROM kb.doc_review_runs WHERE request_id = $1 ORDER BY id DESC LIMIT 1`,
		result.RequestID,
	).Scan(&latestRunID); err != nil {
		t.Fatalf("select latest run id: %v", err)
	}
	if latestRunID != runID {
		t.Errorf("latest run id = %d, want %d", latestRunID, runID)
	}

	var aspectStatus string
	var aspectProgress float64
	var aspectFindingCount int
	var aspectStart, aspectEnd sql.NullTime
	var aspectError sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT status, progress, finding_count, start_time, end_time, error_message
		 FROM kb.doc_review_status
		 WHERE run_id = $1
		 ORDER BY id
		 LIMIT 1`,
		runID,
	).Scan(&aspectStatus, &aspectProgress, &aspectFindingCount, &aspectStart, &aspectEnd, &aspectError); err != nil {
		t.Fatalf("select aspect status: %v", err)
	}
	if aspectStatus != "pending" {
		t.Errorf("aspect status = %q, want pending", aspectStatus)
	}
	if aspectProgress != 0 {
		t.Errorf("aspect progress = %v, want 0", aspectProgress)
	}
	if aspectFindingCount != 0 {
		t.Errorf("aspect finding_count = %d, want 0", aspectFindingCount)
	}
	if aspectStart.Valid {
		t.Error("aspect start_time should be cleared")
	}
	if aspectEnd.Valid {
		t.Error("aspect end_time should be cleared")
	}
	if aspectError.Valid && aspectError.String != "" {
		t.Errorf("aspect error_message = %q, want empty", aspectError.String)
	}

	var oldFindingCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb.doc_review_findings WHERE run_id = $1`,
		result.RunID,
	).Scan(&oldFindingCount); err != nil {
		t.Fatalf("count old findings: %v", err)
	}
	if oldFindingCount != 1 {
		t.Errorf("old finding count = %d, want 1", oldFindingCount)
	}

	var newFindingCount int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM kb.doc_review_findings WHERE run_id = $1`,
		runID,
	).Scan(&newFindingCount); err != nil {
		t.Fatalf("count new findings: %v", err)
	}
	if newFindingCount != 0 {
		t.Errorf("new finding count = %d, want 0", newFindingCount)
	}
}

func TestController_RunReview_SkipsAlreadyHandledRequests(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test RunReview Skip Duplicate Doc")
	defer cleanupInputs(t, db, recordID)

	for _, status := range []string{"running", "completed", "stopped"} {
		t.Run(status, func(t *testing.T) {
			result, err := c.AcceptRequest(ctx, SubmitRequestInput{
				InputRecordID: recordID,
				Tier:          "must_review",
				RequesterName: "test-user",
			})
			if err != nil {
				t.Fatalf("AcceptRequest: %v", err)
			}
			defer cleanupRequests(t, db, result.RequestID)

			_, err = db.ExecContext(ctx,
				`UPDATE kb.doc_review_requests SET status = $1 WHERE id = $2`,
				status, result.RequestID,
			)
			if err != nil {
				t.Fatalf("update request to %s: %v", status, err)
			}

			if err := c.RunReview(ctx, result.RequestID, result.RunID); err != nil {
				t.Fatalf("RunReview(%s) should be a no-op, got error: %v", status, err)
			}
		})
	}
}

func TestIsAlreadyHandledReviewStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "accepted", want: false},
		{status: "running", want: true},
		{status: "completed", want: true},
		{status: "stopped", want: true},
		{status: "failed", want: false},
		{status: "", want: false},
	}

	for _, tc := range cases {
		if got := isAlreadyHandledReviewStatus(tc.status); got != tc.want {
			t.Fatalf("isAlreadyHandledReviewStatus(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestController_UpdateFinding(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test UpdateFinding Doc")
	defer cleanupInputs(t, db, recordID)

	// Insert a finding directly via DB using run_id 1 (no real run needed for this test).
	findingID := insertTestFinding(t, db, recordID, 1, FindingItem{
		Pass:         "P1",
		Aspect:       "grammar_spelling",
		Severity:     "medium",
		FindingType:  "style_issue",
		Title:        "Inconsistent capitalization",
		Description:  "Found mixed case usage",
		Evidence:     "Section 2.3",
		Location:     "Line 42",
		Suggestion:   "Use title case consistently",
		Confidence:   0.9,
		ReviewStatus: "pending",
	})
	defer cleanupFindings(t, db, findingID)

	// Update finding.
	err := c.UpdateFinding(ctx, findingID, "accepted", "reviewer-1")
	if err != nil {
		t.Fatalf("UpdateFinding: %v", err)
	}

	// Verify in DB.
	var reviewStatus, reviewedBy string
	err = db.QueryRowContext(ctx,
		`SELECT review_status, reviewed_by FROM kb.doc_review_findings WHERE id = $1`, findingID,
	).Scan(&reviewStatus, &reviewedBy)
	if err != nil {
		t.Fatalf("query finding: %v", err)
	}
	if reviewStatus != "accepted" {
		t.Errorf("review_status = %q, want %q", reviewStatus, "accepted")
	}
	if reviewedBy != "reviewer-1" {
		t.Errorf("reviewed_by = %q, want %q", reviewedBy, "reviewer-1")
	}
}

func TestController_UpdateFinding_InvalidStatus(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	err := c.UpdateFinding(ctx, 0, "invalid_status", "reviewer-1")
	if err == nil {
		t.Fatal("UpdateFinding with invalid status should error, got nil")
	}
}

func TestController_UpdateFinding_NotFound(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	err := c.UpdateFinding(ctx, 999999999, "accepted", "reviewer-1")
	if err == nil {
		t.Fatal("UpdateFinding for non-existent finding should error, got nil")
	}
}

func TestController_GetRequest(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test GetRequest Doc")
	defer cleanupInputs(t, db, recordID)

	result, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	defer cleanupRequests(t, db, result.RequestID)

	req, err := c.GetRequest(ctx, result.RequestID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	if req.ID != result.RequestID {
		t.Errorf("req.ID = %d, want %d", req.ID, result.RequestID)
	}
	if req.Status != "accepted" {
		t.Errorf("req.Status = %q, want %q", req.Status, "accepted")
	}
	if req.InputRecordID != recordID {
		t.Errorf("req.InputRecordID = %d, want %d", req.InputRecordID, recordID)
	}
	if req.CreateTime == "" {
		t.Error("req.CreateTime is empty")
	}
	if req.LatestRunID != result.RunID {
		t.Errorf("req.LatestRunID = %d, want %d", req.LatestRunID, result.RunID)
	}

	// Non-existent request.
	_, err = c.GetRequest(ctx, 999999999)
	if err == nil {
		t.Fatal("GetRequest for non-existent ID should error, got nil")
	}
	fmt.Println("Got expected error for non-existent request:", err)
}

func TestController_GetRequestWithFindings(t *testing.T) {
	db := connectTestDB(t)
	defer db.Close()
	ensureTables(t, db)

	c := &DocReviewController{DB: db}
	ctx := context.Background()

	recordID := insertTestInput(t, db, "Test GetRequestWithFindings Doc")
	defer cleanupInputs(t, db, recordID)

	result, err := c.AcceptRequest(ctx, SubmitRequestInput{
		InputRecordID: recordID,
		Tier:          "must_review",
		RequesterName: "test-user",
	})
	if err != nil {
		t.Fatalf("AcceptRequest: %v", err)
	}
	defer cleanupRequests(t, db, result.RequestID)

	// Manually set request and run to completed so findings get loaded.
	_, err = db.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'completed' WHERE id = $1`,
		result.RequestID,
	)
	if err != nil {
		t.Fatalf("update request to completed: %v", err)
	}
	_, err = db.ExecContext(ctx,
		`UPDATE kb.doc_review_runs SET status = 'completed', end_time = NOW() WHERE id = $1`,
		result.RunID,
	)
	if err != nil {
		t.Fatalf("update run to completed: %v", err)
	}

	// Insert a finding scoped to the run.
	findingID := insertTestFinding(t, db, recordID, result.RunID, FindingItem{
		Pass:         "P1",
		Aspect:       "grammar_spelling",
		Severity:     "low",
		FindingType:  "typo",
		Title:        "Minor typo",
		Description:  "Found a typo",
		Evidence:     "Section 1",
		Location:     "Line 10",
		Suggestion:   "Fix typo",
		Confidence:   0.95,
		ReviewStatus: "pending",
	})
	defer cleanupFindings(t, db, findingID)

	// Get request with findings.
	rwf, err := c.GetRequestWithFindings(ctx, result.RequestID, RequestFindingsOptions{})
	if err != nil {
		t.Fatalf("GetRequestWithFindings: %v", err)
	}
	if len(rwf.Findings) != 1 {
		t.Fatalf("Findings has %d entries, want 1", len(rwf.Findings))
	}
	if rwf.Findings[0].Title != "Minor typo" {
		t.Errorf("Finding title = %q, want %q", rwf.Findings[0].Title, "Minor typo")
	}
	if rwf.Findings[0].ReviewStatus != "pending" {
		t.Errorf("Finding review_status = %q, want %q", rwf.Findings[0].ReviewStatus, "pending")
	}
}
