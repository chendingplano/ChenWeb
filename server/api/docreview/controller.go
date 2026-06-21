package docreview

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var logger = loggerutil.CreateDefaultLogger("DR13")

// DocReviewController manages the review request lifecycle.
type DocReviewController struct {
	DB *sql.DB
}

// NewDocReviewController creates a DocReviewController.
func NewDocReviewController() *DocReviewController {
	return &DocReviewController{DB: ApiTypes.ProjectDBHandle}
}

// AcceptRequest validates input, resolves requester, stores the request as "accepted".
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

	// Resolve requester.
	var userExists bool
	err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM identities WHERE id = $1)`, input.RequesterID).Scan(&userExists)
	if err != nil {
		// identities table may not exist; try kratos identities
		err = c.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM kratos.identities WHERE id = $1::uuid)`, input.RequesterID).Scan(&userExists)
	}
	if !userExists {
		return nil, &RequestError{
			Status:  http.StatusUnprocessableEntity,
			Message: fmt.Sprintf("User %d (%s) not found. Please register or re-enter the user name.", input.RequesterID, input.RequesterName),
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

	var id int64
	err = c.DB.QueryRowContext(ctx, `
		INSERT INTO kb.doc_review_requests
			(input_record_id, tier, aspects, reference_docs, notes, model_overrides,
			 requester_name, requester_id, report_template, doc_template, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'accepted')
		RETURNING id`,
		input.InputRecordID, input.Tier, aspectsJSON, refDocsJSON, input.Notes, overridesJSON,
		input.RequesterName, input.RequesterID, input.ReportTemplate, input.DocTemplate,
	).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("insert review request: %w", err)
	}

	logger.Info("review request created", "request_id", id, "record_id", input.InputRecordID, "tier", input.Tier)

	return &SubmitResult{RequestID: id, Status: "accepted"}, nil
}

// RunReview transitions the request to "running", delegates to ReviewProcessor, then completes.
func (c *DocReviewController) RunReview(ctx context.Context, requestID int64) error {
	// Load request.
	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("load request %d: %w", requestID, err)
	}
	if req.Status != "accepted" {
		return fmt.Errorf("request %d is in status %q, expected 'accepted'", requestID, req.Status)
	}

	reviewRunID := fmt.Sprintf("%d_review_%s", req.InputRecordID, time.Now().UTC().Format("20060102T150405"))

	// Update status -> running.
	_, err = c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'running', review_run_id = $1, start_time = NOW() WHERE id = $2`,
		reviewRunID, requestID,
	)
	if err != nil {
		return fmt.Errorf("update request %d to running: %w", requestID, err)
	}
	logger.Info("review request started", "request_id", requestID, "review_run_id", reviewRunID)

	// Build and run ReviewProcessor.
	llmClient := &llmclients.OpenAIJSONClient{
		HTTPClient: &http.Client{Timeout: 100 * time.Second},
	}
	inputStore := docprocessing.DocMetadataSQLStore{DB: c.DB}
	entityStore := docprocessing.EntityRelationSQLStore{DB: c.DB}
	findingsStore := docprocessing.ReviewFindingsSQLStore{DB: c.DB}

	processor := docprocessing.NewReviewProcessor(inputStore, entityStore, findingsStore, llmClient, nil)
	err = processor.PostProcessIndex(ctx, req.InputRecordID)
	if err != nil {
		// Update status -> failed.
		errMsg := err.Error()
		c.DB.ExecContext(ctx,
			`UPDATE kb.doc_review_requests SET status = 'failed', end_time = NOW(), error_message = $1 WHERE id = $2`,
			errMsg, requestID,
		)
		logger.Info("review request failed", "request_id", requestID, "error", errMsg)
		return fmt.Errorf("review failed for record %d: %w", req.InputRecordID, err)
	}

	// Update status -> completed.
	_, err = c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'completed', end_time = NOW() WHERE id = $1`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("update request %d to completed: %w", requestID, err)
	}
	logger.Info("review request completed", "request_id", requestID, "review_run_id", reviewRunID)
	return nil
}

// GetRequest returns the request status row.
func (c *DocReviewController) GetRequest(ctx context.Context, requestID int64) (*RequestStatus, error) {
	return c.loadRequest(ctx, requestID)
}

// GetRequestWithFindings returns the request with its findings.
func (c *DocReviewController) GetRequestWithFindings(ctx context.Context, requestID int64) (*RequestWithFindings, error) {
	req, err := c.loadRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	result := &RequestWithFindings{Request: *req}

	// Only return findings if completed.
	if req.Status == "completed" && req.ReviewRunID != "" {
		rows, err := c.DB.QueryContext(ctx, `
			SELECT id, pass, aspect, severity, finding_type, title, description,
			       COALESCE(evidence,''), COALESCE(location,''), COALESCE(suggestion,''),
			       COALESCE(confidence,0), COALESCE(review_status,'pending')
			FROM kb.doc_review_findings
			WHERE input_record_id = $1 AND review_run_id = $2
			ORDER BY id`, req.InputRecordID, req.ReviewRunID)
		if err != nil {
			return nil, fmt.Errorf("load findings: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var f FindingItem
			if err := rows.Scan(&f.ID, &f.Pass, &f.Aspect, &f.Severity, &f.FindingType,
				&f.Title, &f.Description, &f.Evidence, &f.Location, &f.Suggestion,
				&f.Confidence, &f.ReviewStatus); err != nil {
				return nil, fmt.Errorf("scan finding: %w", err)
			}
			result.Findings = append(result.Findings, f)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate findings: %w", err)
		}
	}
	return result, nil
}

// StopRequest transitions a running request to stopped.
func (c *DocReviewController) StopRequest(ctx context.Context, requestID int64) error {
	res, err := c.DB.ExecContext(ctx,
		`UPDATE kb.doc_review_requests SET status = 'stopped', end_time = NOW() WHERE id = $1 AND status = 'running'`,
		requestID,
	)
	if err != nil {
		return fmt.Errorf("stop request %d: %w", requestID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("request %d is not in 'running' status", requestID)
	}
	logger.Info("review request stopped", "request_id", requestID)
	return nil
}

// UpdateFinding updates review_status and reviewed_by on a finding.
func (c *DocReviewController) UpdateFinding(ctx context.Context, findingID int64, reviewStatus string, reviewedBy string) error {
	allowed := map[string]bool{"pending": true, "accepted": true, "rejected": true, "deferred": true}
	if !allowed[reviewStatus] {
		return fmt.Errorf("invalid review_status: %q (must be pending/accepted/rejected/deferred)", reviewStatus)
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
	return nil
}

// loadRequest fetches one request row.
func (c *DocReviewController) loadRequest(ctx context.Context, id int64) (*RequestStatus, error) {
	var req RequestStatus
	var aspectsJSON, refDocsJSON, overridesJSON sql.NullString
	var reviewRunID, notes, errorMsg, startTime, endTime sql.NullString
	var reportTmpl, docTmpl sql.NullString

	err := c.DB.QueryRowContext(ctx, `
		SELECT id, input_record_id, COALESCE(review_run_id,''), tier, aspects::text,
		       COALESCE(reference_docs::text,''), COALESCE(notes,''), COALESCE(model_overrides::text,''),
		       requester_name, requester_id, COALESCE(report_template,''), COALESCE(doc_template,''),
		       status, create_time::text, COALESCE(start_time::text,''), COALESCE(end_time::text,''),
		       COALESCE(error_message,'')
		FROM kb.doc_review_requests WHERE id = $1`, id,
	).Scan(&req.ID, &req.InputRecordID, &reviewRunID, &req.Tier, &aspectsJSON,
		&refDocsJSON, &notes, &overridesJSON,
		&req.RequesterName, &req.RequesterID, &reportTmpl, &docTmpl,
		&req.Status, &req.CreateTime, &startTime, &endTime, &errorMsg)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("request %d not found", id)
		}
		return nil, fmt.Errorf("load request %d: %w", id, err)
	}

	req.ReviewRunID = reviewRunID.String
	req.Notes = notes.String
	req.ReportTemplate = reportTmpl.String
	req.DocTemplate = docTmpl.String
	req.StartTime = startTime.String
	req.EndTime = endTime.String
	req.ErrorMessage = errorMsg.String

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

// RequestError is an HTTP-level error with status code.
type RequestError struct {
	Status  int
	Message string
}

func (e *RequestError) Error() string { return e.Message }
