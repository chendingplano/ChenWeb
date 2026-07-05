package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/docactivity"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var logger = loggerutil.CreateDefaultLogger("DR13")

func isAlreadyHandledReviewStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "completed", "stopped":
		return true
	default:
		return false
	}
}

// DocReviewController manages the review request lifecycle.
type DocReviewController struct {
	DB         *sql.DB
	Translator FindingTranslator
}

// NewDocReviewController creates a DocReviewController.
func NewDocReviewController() *DocReviewController {
	return &DocReviewController{DB: ApiTypes.ProjectDBHandle}
}

// AcceptRequest validates input, resolves requester, stores the request as "accepted"
// and creates the first run row, returning both IDs.
func (c *DocReviewController) AcceptRequest(ctx context.Context, input SubmitRequestInput) (*SubmitResult, error) {
	// Validate document exists.
	var exists bool
	err := c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kb.inputs WHERE id = $1)`, input.InputRecordID).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check document %d: %w", input.InputRecordID, err)
	}
	if !exists {
		return nil, &RequestError{Status: http.StatusUnprocessableEntity, Message: fmt.Sprintf("Document %d not found", input.InputRecordID)}
	}

	// If requester_id is provided (non-zero), optionally validate it exists.
	if input.RequesterID > 0 {
		var userExists bool
		err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identities WHERE id = $1)`, input.RequesterID).Scan(&userExists)
		if err != nil {
			err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kratos.identities WHERE id = $1::text)`, fmt.Sprintf("%d", input.RequesterID)).Scan(&userExists)
		}
		if !userExists {
			logger.Warn("requester_id not found in identities table, proceeding anyway", "requester_id", input.RequesterID, "requester_name", input.RequesterName)
		}
	}

	// Resolve aspect list.
	var aspects []string
	if input.Tier == "custom" {
		aspects = input.Aspects
	} else {
		aspects = ResolveAspectsForTier(input.Tier)
	}
	if len(aspects) == 0 {
		return nil, &RequestError{Status: http.StatusUnprocessableEntity, Message: "At least one aspect must be selected"}
	}

	// Enforce idempotency: if a request exists for this document that is still running/active, reject.
	var activeID int64
	err = c.DB.QueryRowContext(ctx,
		`SELECT id FROM kb.doc_review_requests WHERE input_record_id = $1 AND status IN ('accepted','running') LIMIT 1`,
		input.InputRecordID,
	).Scan(&activeID)
	if err == nil {
		return nil, &RequestError{
			Status:  http.StatusConflict,
			Message: fmt.Sprintf("A review request (ID %d) is already active for this document. Stop it first or wait for completion.", activeID),
		}
	} else if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check active request: %w", err)
	}

	refDocsJSON, err := json.Marshal(input.ReferenceDocs)
	if err != nil {
		return nil, fmt.Errorf("marshal reference docs: %w", err)
	}
	aspectsJSON, err := json.Marshal(aspects)
	if err != nil {
		return nil, fmt.Errorf("marshal aspects: %w", err)
	}
	overridesJSON, err := json.Marshal(input.ModelOverrides)
	if err != nil {
		return nil, fmt.Errorf("marshal model overrides: %w", err)
	}

	var requestID int64
	err = c.DB.QueryRowContext(ctx, `
		INSERT INTO kb.doc_review_requests
			(input_record_id, tier, aspects, reference_docs, notes, model_overrides,
			 requester_name, requester_id, report_template, doc_template, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'accepted')
		RETURNING id`,
		input.InputRecordID, input.Tier, aspectsJSON, refDocsJSON, input.Notes, overridesJSON,
		input.RequesterName, input.RequesterID, input.ReportTemplate, input.DocTemplate,
	).Scan(&requestID)
	if err != nil {
		return nil, fmt.Errorf("insert review request: %w", err)
	}

	runID, err := c.createRun(ctx, requestID, input.InputRecordID, aspects, input.ModelOverrides, input.Notes, input.RequesterName)
	if err != nil {
		return nil, fmt.Errorf("create run for request %d: %w", requestID, err)
	}

	if err := c.seedAspectStatuses(ctx, requestID, input.InputRecordID, runID, aspects); err != nil {
		logger.Warn("failed to seed aspect statuses", "request_id", requestID, "run_id", runID, "error", err)
	}

	logger.Info("review request created", "request_id", requestID, "run_id", runID, "record_id", input.InputRecordID, "tier", input.Tier)

	return &SubmitResult{RequestID: requestID, RunID: runID, Status: "accepted"}, nil
}

// createRun inserts a new kb.doc_review_runs row and returns its id.
func (c *DocReviewController) createRun(ctx context.Context, requestID, recordID int64, aspects []string, modelOverrides map[string]ModelOverride, notes, createdBy string) (int64, error) {
	var runCount int
	_ = c.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.doc_review_runs WHERE request_id = $1`, requestID).Scan(&runCount)

	aspectsJSON, err := json.Marshal(aspects)
	if err != nil {
		return 0, fmt.Errorf("marshal aspects: %w", err)
	}
	overridesJSON, err := json.Marshal(modelOverrides)
	if err != nil {
		return 0, fmt.Errorf("marshal model overrides: %w", err)
	}

	var runID int64
	err = c.DB.QueryRowContext(ctx, `
		INSERT INTO kb.doc_review_runs
			(request_id, input_record_id, run_number, aspects, model_overrides, notes, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7)
		RETURNING id`,
		requestID, recordID, runCount+1, aspectsJSON, overridesJSON, notes, nullableStr(createdBy),
	).Scan(&runID)
	return runID, err
}

// RunReview transitions the run to "running", delegates to ReviewProcessor, then completes.
func (c *DocReviewController) RunReview(ctx context.Context, requestID, runID int64) error {
	// Load request.
	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("load request %d: %w", requestID, err)
	}
	if isAlreadyHandledReviewStatus(req.Status) {
		logger.Info("review request already handled; skipping duplicate run",
			"request_id", requestID,
			"status", req.Status,
		)
		return nil
	}
	if req.Status != "accepted" {
		return fmt.Errorf("request %d is in status %q, expected 'accepted'", requestID, req.Status)
	}

	// Transition run -> running (atomic: only one worker wins).
	res, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_runs SET status = 'running', start_time = NOW()
		 WHERE id = $1 AND status IN ('pending', 'accepted')`,
		runID,
	)
	if err != nil {
		return fmt.Errorf("update run %d to running: %w", runID, err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("check running transition for run %d: %w", runID, err)
	}
	if rowsAffected == 0 {
		// Another worker claimed this run.
		logger.Info("run already claimed by another worker; skipping", "run_id", runID, "request_id", requestID)
		return nil
	}

	// Mirror on the request row.
	c.DB.ExecContext(ctx, `UPDATE kb.doc_review_requests SET status = 'running' WHERE id = $1`, requestID)

	c.markAspectsRunning(ctx, runID)
	logger.Info("review run started", "request_id", requestID, "run_id", runID)

	llmClient := &llmclients.OpenAIJSONClient{
		HTTPClient: &http.Client{Timeout: 100 * time.Second},
	}
	inputStore := docprocessing.DocMetadataSQLStore{DB: c.DB}
	entityStore := docprocessing.EntityRelationSQLStore{DB: c.DB}
	findingsStore := ReviewFindingsSQLStore{DB: c.DB, Languages: docReviewReportLanguagesFromEnv()}
	reviewLogsStore := ReviewLogsSQLStore{DB: c.DB}
	statusStore := ReviewStatusSQLStore{DB: c.DB}

	processor := NewReviewProcessor(inputStore, entityStore, findingsStore, llmClient, nil)
	processor.RunID = runID
	processor.RequestedAspects = append([]string(nil), req.Aspects...)
	processor.ReviewLogsStore = reviewLogsStore
	processor.StatusStore = statusStore
	err = processor.PostProcessIndex(ctx, req.InputRecordID)
	if err != nil {
		errMsg := err.Error()
		c.DB.ExecContext(ctx,
			`UPDATE kb.doc_review_runs SET status = 'failed', end_time = NOW(), error_message = $1 WHERE id = $2`,
			errMsg, runID,
		)
		c.DB.ExecContext(ctx, `UPDATE kb.doc_review_requests SET status = 'failed' WHERE id = $1`, requestID)
		c.failOpenAspects(ctx, runID, errMsg)
		logger.Info("review run failed", "request_id", requestID, "run_id", runID, "error", errMsg)
		return fmt.Errorf("review failed for record %d: %w", req.InputRecordID, err)
	}

	// If the request was stopped while running, do not override that terminal state.
	if cur, _ := c.loadRequest(ctx, requestID); cur != nil && cur.Status == "stopped" {
		logger.Info("review request was stopped; skipping completion", "request_id", requestID)
		return nil
	}

	c.finalizeAspectsSuccess(ctx, runID, req.InputRecordID)
	c.DB.ExecContext(ctx, `UPDATE kb.doc_review_runs SET status = 'completed', end_time = NOW() WHERE id = $1`, runID)
	c.DB.ExecContext(ctx, `UPDATE kb.doc_review_requests SET status = 'completed' WHERE id = $1`, requestID)
	logger.Info("review run completed", "request_id", requestID, "run_id", runID)
	return nil
}

// RunReviewAndReport runs the review and, on success, builds + persists the report.
// Intended to be launched in a background goroutine (with a detached context) so
// the submit HTTP request can return immediately and the live monitor (DR15) can
// observe the job while it runs.
func (c *DocReviewController) RunReviewAndReport(ctx context.Context, requestID, runID int64) {
	if err := c.RunReview(ctx, requestID, runID); err != nil {
		logger.Warn("background review run failed", "request_id", requestID, "run_id", runID, "error", err)
		return
	}
	req, err := c.GetRequestWithFindings(ctx, requestID, RequestFindingsOptions{})
	if err != nil || req == nil || req.Request.Status != "completed" {
		return
	}
	gen := NewDocReviewReportGenerator()
	report, buildErr := gen.Build(ctx, &req.Request, req.Findings)
	if buildErr != nil {
		logger.Warn("background report build failed", "request_id", requestID, "error", buildErr)
		return
	}
	if _, perr := gen.Persist(ctx, &req.Request, report); perr != nil {
		logger.Warn("background report persist failed", "request_id", requestID, "error", perr)
		return
	}
	if typErr := GenerateTypstReport(ctx, requestID, report, &req.Request); typErr != nil {
		logger.Warn("typst PDF generation failed", "request_id", requestID, "error", typErr)
	}
}

// GetRequest returns the request status row.
func (c *DocReviewController) GetRequest(ctx context.Context, requestID int64) (*RequestStatus, error) {
	return c.loadRequest(ctx, requestID)
}

// GetRequestWithFindings returns the request with its findings.
func (c *DocReviewController) GetRequestWithFindings(ctx context.Context, requestID int64, opts RequestFindingsOptions) (*RequestWithFindings, error) {
	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	result := &RequestWithFindings{Request: *req, Packages: configuredPackageOrder()}
	languageCode := supportedLanguageCode(opts.Language)
	passFilter := strings.TrimSpace(opts.Pass)
	aspectFilter := strings.TrimSpace(opts.Aspect)

	// Load per-aspect statuses for the latest run.
	if req.LatestRunID > 0 {
		statusRows, err := c.DB.QueryContext(ctx, `
			SELECT aspect, COALESCE(pass,''), status, COALESCE(progress,0), COALESCE(finding_count,0),
			       COALESCE(error_message,''), COALESCE(start_time::text,''), COALESCE(end_time::text,'')
			FROM kb.doc_review_status
			WHERE run_id = $1
			ORDER BY id`, req.LatestRunID)
		if err != nil {
			return nil, fmt.Errorf("load aspect statuses: %w", err)
		}
		defer statusRows.Close()
		for statusRows.Next() {
			var s AspectStatus
			if err := statusRows.Scan(&s.Aspect, &s.Pass, &s.Status, &s.Progress, &s.FindingCount,
				&s.ErrorMessage, &s.StartTime, &s.EndTime); err != nil {
				return nil, fmt.Errorf("scan aspect status: %w", err)
			}
			result.AspectStatuses = append(result.AspectStatuses, s)
		}
		if err := statusRows.Err(); err != nil {
			return nil, fmt.Errorf("iterate aspect statuses: %w", err)
		}
	}

	// Only return findings if completed.
	if req.Status == "completed" && req.LatestRunID > 0 {
		rows, err := c.DB.QueryContext(ctx, `
			SELECT id, pass, aspect, severity, finding_type, title, description,
			       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
			       COALESCE(confidence,0), COALESCE(review_status,'pending'), COALESCE(metadata, '{}'::jsonb)::text
			FROM kb.doc_review_findings
			WHERE input_record_id = $1 AND run_id = $2
			  AND ($3 = '' OR pass = $3)
			  AND ($4 = '' OR aspect = $4)
			ORDER BY CASE severity WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END, id ASC`,
			req.InputRecordID, req.LatestRunID, passFilter, aspectFilter)
		if err != nil {
			return nil, fmt.Errorf("load findings: %w", err)
		}
		defer rows.Close()
		var localized []FindingItem
		metadataByFindingID := map[int64][]byte{}
		for rows.Next() {
			var f FindingItem
			var metadata string
			if err := rows.Scan(&f.ID, &f.Pass, &f.Aspect, &f.Severity, &f.FindingType,
				&f.Title, &f.Description, &f.Evidence, &f.Location, &f.Suggestion,
				&f.Confidence, &f.ReviewStatus, &metadata); err != nil {
				return nil, fmt.Errorf("scan finding: %w", err)
			}
			metadataByFindingID[f.ID] = []byte(metadata)
			applyFindingMetadata(&f, []byte(metadata))
			localized = append(localized, f)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate findings: %w", err)
		}
		if languageCode != "" {
			localized, err = c.localizeFindings(ctx, languageCode, localized, metadataByFindingID)
			if err != nil {
				return nil, err
			}
		}
		result.Findings = localized
	}
	return result, nil
}

// StopRequest transitions an accepted or running request to stopped.
func (c *DocReviewController) StopRequest(ctx context.Context, requestID int64) error {
	res, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'stopped' WHERE id = $1 AND status IN ('accepted', 'running')`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("stop request %d: %w", requestID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("request %d is not in a stoppable state (accepted or running)", requestID)
	}
	// Drive the active run and its aspects to a terminal state.
	var runID int64
	_ = c.DB.QueryRowContext(ctx,
		`SELECT id FROM kb.doc_review_runs WHERE request_id = $1 AND status IN ('pending','running') ORDER BY id DESC LIMIT 1`,
		requestID,
	).Scan(&runID)
	if runID > 0 {
		c.DB.ExecContext(ctx, `UPDATE kb.doc_review_runs SET status = 'stopped', end_time = NOW() WHERE id = $1`, runID)
		c.failOpenAspects(ctx, runID, "stopped")
	}
	logger.Info("review request stopped", "request_id", requestID, "run_id", runID)
	return nil
}

// RestartRequest re-arms an unfinished review. It resets the latest non-completed
// run and every aspect back to pending so a fresh trigger re-runs from scratch.
// Returns the run_id so the caller can re-publish the JetStream event.
func (c *DocReviewController) RestartRequest(ctx context.Context, requestID int64) (int64, error) {
	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return 0, err
	}
	if req.Status == "completed" {
		return 0, &RequestError{
			Status:  http.StatusConflict,
			Message: fmt.Sprintf("request %d is already completed; nothing to restart", requestID),
		}
	}

	// Find the latest run for this request.
	var runID int64
	var runStatus string
	err = c.DB.QueryRowContext(ctx,
		`SELECT id, status FROM kb.doc_review_runs WHERE request_id = $1 ORDER BY id DESC LIMIT 1`,
		requestID,
	).Scan(&runID, &runStatus)
	if err == sql.ErrNoRows || runStatus == "completed" {
		return 0, &RequestError{
			Status:  http.StatusConflict,
			Message: fmt.Sprintf("request %d has no restartable run", requestID),
		}
	}
	if err != nil {
		return 0, fmt.Errorf("find latest run for request %d: %w", requestID, err)
	}

	// Reset the run and request to 'pending'/'accepted'.
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_runs SET status = 'pending', start_time = NULL, end_time = NULL, error_message = NULL WHERE id = $1`,
		runID,
	); err != nil {
		return 0, fmt.Errorf("reset run %d: %w", runID, err)
	}
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'accepted' WHERE id = $1`,
		requestID,
	); err != nil {
		return 0, fmt.Errorf("reset request %d to accepted: %w", requestID, err)
	}
	// Reset every aspect of the run back to 'pending'.
	if _, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_status
		 SET status = 'pending', progress = 0, finding_count = 0, error_message = NULL,
		     start_time = NULL, end_time = NULL, modify_time = NOW()
		 WHERE run_id = $1`,
		runID,
	); err != nil {
		return 0, fmt.Errorf("reset aspect statuses for run %d: %w", runID, err)
	}

	logger.Info("review request restarted", "request_id", requestID, "run_id", runID)
	return runID, nil
}

// RecoverStalledReviews finds review requests left unfinished by a previous process
// and re-arms each for a fresh run. Returns the (requestID, runID) pairs that were
// recovered so the caller can re-trigger them.
func (c *DocReviewController) RecoverStalledReviews(ctx context.Context) ([]StalledRun, error) {
	rows, err := c.DB.QueryContext(ctx,
		`SELECT id FROM kb.doc_review_requests WHERE status IN ('accepted','running') ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list stalled reviews: %w", err)
	}
	defer rows.Close()
	var requestIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan stalled review id: %w", err)
		}
		requestIDs = append(requestIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stalled reviews: %w", err)
	}

	var recovered []StalledRun
	for _, id := range requestIDs {
		runID, err := c.RestartRequest(ctx, id)
		if err != nil {
			logger.Warn("failed to re-arm stalled review", "request_id", id, "error", err)
			continue
		}
		recovered = append(recovered, StalledRun{RequestID: id, RunID: runID})
	}
	if len(recovered) > 0 {
		logger.Info("recovered stalled doc-review requests", "count", len(recovered))
	}
	return recovered, nil
}

// UpdateFinding updates review_status and reviewed_by on a finding.
func (c *DocReviewController) UpdateFinding(ctx context.Context, findingID int64, reviewStatus string, reviewedBy string) error {
	allowed := map[string]bool{
		"pending": true, "accepted": true, "rejected": true, "deferred": true,
		"deleted": true, "fixed": true, "corrected": true,
	}
	if !allowed[reviewStatus] {
		return fmt.Errorf("invalid review_status: %q (must be pending/accepted/rejected/deferred/deleted/fixed/corrected)", reviewStatus)
	}
	// Capture finding context before the update so a delete can be logged with the
	// original finding details (non-fatal if it cannot be loaded).
	var fctx *findingContext
	if reviewStatus == "deleted" {
		fctx, _ = c.loadFindingContext(ctx, findingID)
	}

	res, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_findings SET review_status = $1, reviewed_by = $2 WHERE id = $3`,
		reviewStatus, reviewedBy, findingID,
	)
	if err != nil {
		return fmt.Errorf("update finding %d: %w", findingID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("finding %d not found", findingID)
	}
	logger.Info("finding updated", "finding_id", findingID, "review_status", reviewStatus, "reviewed_by", reviewedBy)

	if reviewStatus == "deleted" && fctx != nil {
		docactivity.Log(ctx, c.DB, docactivity.Activity{
			ActivityType:  docactivity.TypeFindingDelete,
			InputRecordID: fctx.InputRecordID,
			RunID:         fctx.RunID,
			ReportID:      c.resolveReportID(ctx, fctx.RunID),
			FindingID:     fctx.ID,
			Location:      fctx.Location,
			OldContent:    fctx.Title,
			Actor:         reviewedBy,
			Detail: map[string]any{
				"title":       fctx.Title,
				"aspect":      fctx.Aspect,
				"severity":    fctx.Severity,
				"description": fctx.Description,
				"suggestion":  fctx.Suggestion,
			},
		})
	}
	return nil
}

// loadRequest fetches one request row, joining to the latest run for timing fields.
func (c *DocReviewController) loadRequest(ctx context.Context, id int64) (*RequestStatus, error) {
	var req RequestStatus
	var aspectsJSON, refDocsJSON, overridesJSON sql.NullString
	var notes, reportTmpl, docTmpl sql.NullString
	var latestRunID sql.NullInt64
	var startTime, endTime, errorMsg sql.NullString

	err := c.DB.QueryRowContext(ctx, `
		SELECT r.id, r.input_record_id, r.tier, r.aspects::text,
		       COALESCE(r.reference_docs::text,''), COALESCE(r.notes,''), COALESCE(r.model_overrides::text,''),
		       r.requester_name, r.requester_id, COALESCE(r.report_template,''), COALESCE(r.doc_template,''),
		       r.status, r.create_time::text,
		       lr.id, COALESCE(lr.start_time::text,''), COALESCE(lr.end_time::text,''), COALESCE(lr.error_message,'')
		FROM kb.doc_review_requests r
		LEFT JOIN LATERAL (
		    SELECT id, start_time, end_time, error_message
		    FROM kb.doc_review_runs
		    WHERE request_id = r.id
		    ORDER BY id DESC
		    LIMIT 1
		) lr ON true
		WHERE r.id = $1`, id,
	).Scan(&req.ID, &req.InputRecordID, &req.Tier, &aspectsJSON,
		&refDocsJSON, &notes, &overridesJSON,
		&req.RequesterName, &req.RequesterID, &reportTmpl, &docTmpl,
		&req.Status, &req.CreateTime,
		&latestRunID, &startTime, &endTime, &errorMsg)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("request %d not found", id)
		}
		return nil, fmt.Errorf("load request %d: %w", id, err)
	}

	if latestRunID.Valid {
		req.LatestRunID = latestRunID.Int64
	}
	req.Notes = notes.String
	req.ReportTemplate = reportTmpl.String
	req.DocTemplate = docTmpl.String
	req.StartTime = startTime.String
	req.EndTime = endTime.String
	req.ErrorMessage = errorMsg.String

	// Surface the latest report id (if any) so the GUI can link to it after
	// an async run completes.
	var reportID sql.NullInt64
	_ = c.DB.QueryRowContext(ctx,
		`SELECT id FROM kb.doc_review_reports WHERE request_id = $1 ORDER BY id DESC LIMIT 1`, id,
	).Scan(&reportID)
	if reportID.Valid {
		req.ReportID = reportID.Int64
	}

	if aspectsJSON.Valid {
		json.Unmarshal([]byte(aspectsJSON.String), &req.Aspects)
	}
	if refDocsJSON.Valid && refDocsJSON.String != "" {
		json.Unmarshal([]byte(refDocsJSON.String), &req.ReferenceDocs)
	}
	if overridesJSON.Valid && overridesJSON.String != "" {
		json.Unmarshal([]byte(overridesJSON.String), &req.ModelOverrides)
	}
	return &req, nil
}

// ── DR15: per-aspect status tracking & live monitor ──────────────────────────

// aspectPassMap returns aspect name -> pass ("P1".."P6") from the aspect catalog.
func aspectPassMap() map[string]string {
	m := make(map[string]string)
	for _, a := range ListAspects() {
		m[a.Name] = a.Group
	}
	return m
}

// seedAspectStatuses inserts one kb.doc_review_status row per aspect (status
// 'pending') for a freshly-created run.
func (c *DocReviewController) seedAspectStatuses(ctx context.Context, requestID, recordID int64, runID int64, aspects []string) error {
	if len(aspects) == 0 {
		return nil
	}
	passByName := aspectPassMap()
	for _, aspect := range aspects {
		_, err := c.DB.ExecContext(ctx, `
			INSERT INTO kb.doc_review_status (request_id, input_record_id, run_id, aspect, pass, status)
			VALUES ($1, $2, $3, $4, $5, 'pending')
			ON CONFLICT (run_id, aspect) DO NOTHING`,
			requestID, recordID, runID, aspect, nullableStr(passByName[aspect]),
		)
		if err != nil {
			return fmt.Errorf("seed aspect %q: %w", aspect, err)
		}
	}
	return nil
}

// markAspectsRunning flips every pending aspect of a run to 'running'.
func (c *DocReviewController) markAspectsRunning(ctx context.Context, runID int64) {
	_, err := c.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status
		SET status = 'running', start_time = NOW(), progress = 0, modify_time = NOW()
		WHERE run_id = $1 AND status = 'pending'`, runID)
	if err != nil {
		logger.Warn("mark aspects running failed", "run_id", runID, "error", err)
	}
}

// finalizeAspectsSuccess marks every non-finished aspect of a run 'success',
// stamping each aspect's finding count from kb.doc_review_findings.
func (c *DocReviewController) finalizeAspectsSuccess(ctx context.Context, runID int64, recordID int64) {
	_, err := c.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status s
		SET status = 'success',
		    progress = 1,
		    end_time = NOW(),
		    modify_time = NOW(),
		    finding_count = COALESCE((
		        SELECT COUNT(*) FROM kb.doc_review_findings f
		        WHERE f.run_id = $1 AND f.input_record_id = $2 AND f.aspect = s.aspect
		    ), 0)
		WHERE s.run_id = $1 AND s.status NOT IN ('success', 'failed')`,
		runID, recordID)
	if err != nil {
		logger.Warn("finalize aspects success failed", "run_id", runID, "error", err)
	}
}

// failOpenAspects drives every non-finished aspect of a run to 'failed' with the
// given message (used on whole-run failure and on stop).
func (c *DocReviewController) failOpenAspects(ctx context.Context, runID int64, errMsg string) {
	_, err := c.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status
		SET status = 'failed', error_message = $2, end_time = NOW(), modify_time = NOW()
		WHERE run_id = $1 AND status NOT IN ('success', 'failed')`,
		runID, errMsg)
	if err != nil {
		logger.Warn("fail open aspects failed", "run_id", runID, "error", err)
	}
}

// ListActiveJobs returns every review request that still has at least one
// unfinished aspect (status NOT IN 'success','failed'), each with its full
// per-aspect status list.
func (c *DocReviewController) ListActiveJobs(ctx context.Context) ([]ActiveJob, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT r.id, r.input_record_id, COALESCE(lr.id, 0), r.tier, r.status,
		       r.requester_name, COALESCE(i.title, i.file_name, ''),
		       r.create_time::text, COALESCE(lr.start_time::text, '')
		FROM kb.doc_review_requests r
		LEFT JOIN LATERAL (
		    SELECT id, start_time
		    FROM kb.doc_review_runs
		    WHERE request_id = r.id
		    ORDER BY id DESC
		    LIMIT 1
		) lr ON true
		LEFT JOIN kb.inputs i ON i.id = r.input_record_id
		WHERE r.status NOT IN ('stopped', 'completed', 'failed')
		AND EXISTS (
		    SELECT 1 FROM kb.doc_review_status s
		    WHERE s.request_id = r.id AND s.status NOT IN ('success', 'failed')
		)
		ORDER BY r.create_time DESC`)
	if err != nil {
		return nil, fmt.Errorf("list active jobs: %w", err)
	}
	defer rows.Close()

	var jobs []ActiveJob
	for rows.Next() {
		var j ActiveJob
		if err := rows.Scan(&j.RequestID, &j.InputRecordID, &j.RunID, &j.Tier, &j.Status,
			&j.RequesterName, &j.DocTitle, &j.CreateTime, &j.StartTime); err != nil {
			return nil, fmt.Errorf("scan active job: %w", err)
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active jobs: %w", err)
	}

	// Attach per-aspect status to each job.
	for i := range jobs {
		if jobs[i].RunID > 0 {
			aspects, err := c.loadAspectStatuses(ctx, jobs[i].RunID)
			if err != nil {
				return nil, err
			}
			jobs[i].Aspects = aspects
		}
	}
	return jobs, nil
}

// ListRequests returns document-review requests matching the given filter,
// newest first.
func (c *DocReviewController) ListRequests(ctx context.Context, f RequestListFilter) ([]RequestListItem, error) {
	var conds []string
	var args []any

	if f.RequestID != "" {
		if id, err := strconv.ParseInt(strings.TrimSpace(f.RequestID), 10, 64); err == nil {
			args = append(args, id)
			conds = append(conds, fmt.Sprintf("r.id = $%d", len(args)))
		}
	}
	if t := strings.TrimSpace(f.DocTitle); t != "" {
		args = append(args, "%"+t+"%")
		conds = append(conds, fmt.Sprintf("(COALESCE(i.title,'') ILIKE $%d OR COALESCE(i.file_name,'') ILIKE $%d)", len(args), len(args)))
	}
	if rq := strings.TrimSpace(f.RequesterName); rq != "" {
		args = append(args, "%"+rq+"%")
		conds = append(conds, fmt.Sprintf("r.requester_name ILIKE $%d", len(args)))
	}
	if f.Tier != "" && f.Tier != "all" {
		args = append(args, f.Tier)
		conds = append(conds, fmt.Sprintf("r.tier = $%d", len(args)))
	}
	if f.Status != "" && f.Status != "all" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("r.status = $%d", len(args)))
	}
	if f.CreateStart != "" {
		args = append(args, f.CreateStart)
		conds = append(conds, fmt.Sprintf("r.create_time >= $%d::timestamptz", len(args)))
	}
	if f.CreateEnd != "" {
		args = append(args, f.CreateEnd)
		conds = append(conds, fmt.Sprintf("r.create_time <= $%d::timestamptz", len(args)))
	}

	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args = append(args, limit)
	limitClause := fmt.Sprintf("LIMIT $%d", len(args))

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT r.id, r.input_record_id, COALESCE(i.title, i.file_name, ''), r.tier, r.status,
		       r.requester_name, COALESCE(jsonb_array_length(r.aspects), 0),
		       r.create_time::text,
		       COALESCE((SELECT rn.start_time::text FROM kb.doc_review_runs rn WHERE rn.request_id = r.id ORDER BY rn.id DESC LIMIT 1), ''),
		       COALESCE((SELECT rn.end_time::text FROM kb.doc_review_runs rn WHERE rn.request_id = r.id ORDER BY rn.id DESC LIMIT 1), ''),
		       COALESCE((SELECT rep.id FROM kb.doc_review_reports rep WHERE rep.request_id = r.id ORDER BY rep.id DESC LIMIT 1), 0),
		       COALESCE((SELECT rep.total_findings FROM kb.doc_review_reports rep WHERE rep.request_id = r.id ORDER BY rep.id DESC LIMIT 1), 0)
		FROM kb.doc_review_requests r
		LEFT JOIN kb.inputs i ON i.id = r.input_record_id
		%s
		ORDER BY r.create_time DESC
		%s`, where, limitClause)

	rows, err := c.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list requests: %w", err)
	}
	defer rows.Close()

	var items []RequestListItem
	for rows.Next() {
		var it RequestListItem
		if err := rows.Scan(&it.RequestID, &it.InputRecordID, &it.DocTitle, &it.Tier, &it.Status,
			&it.RequesterName, &it.AspectCount, &it.CreateTime, &it.StartTime, &it.EndTime,
			&it.ReportID, &it.TotalFindings); err != nil {
			return nil, fmt.Errorf("scan request list item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate request list: %w", err)
	}
	return items, nil
}

// loadAspectStatuses returns the kb.doc_review_status rows for a run, ordered by id.
func (c *DocReviewController) loadAspectStatuses(ctx context.Context, runID int64) ([]AspectStatus, error) {
	rows, err := c.DB.QueryContext(ctx, `
		SELECT aspect, COALESCE(pass,''), status, COALESCE(progress,0), finding_count, COALESCE(error_message,''),
		       COALESCE(start_time::text,''), COALESCE(end_time::text,'')
		FROM kb.doc_review_status
		WHERE run_id = $1
		ORDER BY id`, runID)
	if err != nil {
		return nil, fmt.Errorf("load aspect statuses: %w", err)
	}
	defer rows.Close()
	var out []AspectStatus
	for rows.Next() {
		var a AspectStatus
		if err := rows.Scan(&a.Aspect, &a.Pass, &a.Status, &a.Progress, &a.FindingCount, &a.ErrorMessage,
			&a.StartTime, &a.EndTime); err != nil {
			return nil, fmt.Errorf("scan aspect status: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// nullableStr returns a NULL-able string (empty -> NULL) for optional columns.
func nullableStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// RequestError is an HTTP-level error with status code.
type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string { return e.Message }
