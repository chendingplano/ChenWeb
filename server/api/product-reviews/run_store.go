package productreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Run status values (spec: Review request and run lifecycle).
const (
	RunPending   = "pending"
	RunRunning   = "running"
	RunCompleted = "completed"
	RunFailed    = "failed"
)

// Request is a persisted review request (kb.product_review_requests). It pins
// the profile_version it was created against; a re-run re-pins it.
type Request struct {
	ID             int64          `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ProfileID      int64          `json:"profile_id"`
	ProfileVersion int            `json:"profile_version"`
	ArtifactTypes  []string       `json:"artifact_types"`
	Filters        map[string]any `json:"filters"`
	Notes          string         `json:"notes"`
	Requester      string         `json:"requester"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// Run is one execution of a request (kb.product_review_runs).
type Run struct {
	ID                  int64           `json:"id"`
	RequestID           int64           `json:"request_id"`
	RunNumber           int             `json:"run_number"`
	Status              string          `json:"status"`
	StartedAt           *time.Time      `json:"started_at"`
	FinishedAt          *time.Time      `json:"finished_at"`
	ResultCount         int             `json:"result_count"`
	AttributedCount     int             `json:"attributed_count"`
	DocumentScopeCount  int             `json:"document_scope_count"`
	ScopedDocumentCount int             `json:"scoped_document_count"`
	TruncatedCount      int             `json:"truncated_count"`
	ReportJSON          json.RawMessage `json:"report_json"`
	ReportMD            string          `json:"report_md"`
	ErrorMessage        string          `json:"error_message"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

// RunCounts are the per-run tallies written at completion.
type RunCounts struct {
	ResultCount         int
	AttributedCount     int
	DocumentScopeCount  int
	ScopedDocumentCount int
	TruncatedCount      int
}

// ResultFilters narrows a run's persisted results (spec: Result persistence).
type ResultFilters struct {
	NodeID        *int64
	Tier          string
	ExcludeTiers  []string
	Path          string
	InputRecordID *int64
	ArtifactType  string
}

// RequestInput creates a request.
type RequestInput struct {
	TenantID       string
	ProfileID      int64
	ProfileVersion int
	ArtifactTypes  []string
	Filters        map[string]any
	Notes          string
	Requester      string
}

// RunStore persists requests, runs, scoped documents, and result rows.
type RunStore struct {
	DB *sql.DB
}

func (s RunStore) CreateRequest(ctx context.Context, in RequestInput) (*Request, error) {
	tenant := strings.TrimSpace(in.TenantID)
	if tenant == "" {
		tenant = "-"
	}
	types := in.ArtifactTypes
	if len(types) == 0 {
		types = []string{"metric"}
	}
	typesJSON, _ := json.Marshal(types)
	filtersJSON, _ := json.Marshal(orEmptyMap(in.Filters))

	r := Request{}
	var filtersRaw, typesRaw []byte
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.product_review_requests
			(tenant_id, profile_id, profile_version, artifact_types, filters, notes, requester)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, tenant_id, profile_id, profile_version, artifact_types, filters,
		          notes, requester, created_at, updated_at`,
		tenant, in.ProfileID, in.ProfileVersion, typesJSON, filtersJSON,
		strings.TrimSpace(in.Notes), strings.TrimSpace(in.Requester),
	).Scan(&r.ID, &r.TenantID, &r.ProfileID, &r.ProfileVersion, &typesRaw, &filtersRaw,
		&r.Notes, &r.Requester, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(typesRaw, &r.ArtifactTypes)
	_ = json.Unmarshal(filtersRaw, &r.Filters)
	return &r, nil
}

// RepinRequestVersion updates a request's pinned profile_version (used on re-run
// so a re-run after a profile edit executes against the current scope).
func (s RunStore) RepinRequestVersion(ctx context.Context, requestID int64, version int) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.product_review_requests SET profile_version = $2, updated_at = NOW()
		WHERE id = $1`, requestID, version)
	return err
}

func (s RunStore) GetRequest(ctx context.Context, id int64) (*Request, error) {
	r := Request{}
	var typesRaw, filtersRaw []byte
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, tenant_id, profile_id, profile_version, artifact_types, filters,
		       notes, requester, created_at, updated_at
		FROM kb.product_review_requests WHERE id = $1`, id,
	).Scan(&r.ID, &r.TenantID, &r.ProfileID, &r.ProfileVersion, &typesRaw, &filtersRaw,
		&r.Notes, &r.Requester, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(typesRaw, &r.ArtifactTypes)
	_ = json.Unmarshal(filtersRaw, &r.Filters)
	return &r, nil
}

func (s RunStore) ListRequests(ctx context.Context, profileID int64) ([]Request, error) {
	q := `SELECT id, tenant_id, profile_id, profile_version, artifact_types, filters,
	             notes, requester, created_at, updated_at
	      FROM kb.product_review_requests`
	args := []any{}
	if profileID > 0 {
		q += ` WHERE profile_id = $1`
		args = append(args, profileID)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Request
	for rows.Next() {
		var r Request
		var typesRaw, filtersRaw []byte
		if err := rows.Scan(&r.ID, &r.TenantID, &r.ProfileID, &r.ProfileVersion, &typesRaw, &filtersRaw,
			&r.Notes, &r.Requester, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(typesRaw, &r.ArtifactTypes)
		_ = json.Unmarshal(filtersRaw, &r.Filters)
		out = append(out, r)
	}
	return out, rows.Err()
}

// LatestRequestAndRun returns a profile's most recently created request and,
// if any run has executed against it, that request's most recent run. Both
// are nil when the profile has never had a review started (spec:
// product-review-intake — duplicate detection surfaces this to the caller).
func (s RunStore) LatestRequestAndRun(ctx context.Context, profileID int64) (*Request, *Run, error) {
	reqs, err := s.ListRequests(ctx, profileID)
	if err != nil {
		return nil, nil, err
	}
	if len(reqs) == 0 {
		return nil, nil, nil
	}
	latestReq := reqs[0] // ListRequests orders by created_at DESC
	runs, err := s.ListRuns(ctx, latestReq.ID)
	if err != nil {
		return nil, nil, err
	}
	if len(runs) == 0 {
		return &latestReq, nil, nil
	}
	return &latestReq, &runs[0], nil // ListRuns orders by run_number DESC
}

func (s RunStore) CreateRun(ctx context.Context, requestID int64) (*Run, error) {
	r := Run{}
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO kb.product_review_runs (request_id, run_number, status)
		VALUES ($1,
		        (SELECT COALESCE(MAX(run_number), 0) + 1 FROM kb.product_review_runs WHERE request_id = $1),
		        'pending')
		RETURNING id, request_id, run_number, status, result_count, attributed_count,
		          document_scope_count, scoped_document_count, truncated_count,
		          report_json, report_md, error_message, created_at, updated_at`, requestID,
	).Scan(&r.ID, &r.RequestID, &r.RunNumber, &r.Status, &r.ResultCount, &r.AttributedCount,
		&r.DocumentScopeCount, &r.ScopedDocumentCount, &r.TruncatedCount,
		&r.ReportJSON, &r.ReportMD, &r.ErrorMessage, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s RunStore) MarkRunning(ctx context.Context, runID int64) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.product_review_runs
		SET status = 'running', started_at = NOW(), updated_at = NOW()
		WHERE id = $1`, runID)
	return err
}

func (s RunStore) CompleteRun(ctx context.Context, runID int64, c RunCounts, reportJSON []byte, reportMD string) error {
	if len(reportJSON) == 0 {
		reportJSON = []byte("{}")
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.product_review_runs SET
			status = 'completed', finished_at = NOW(), updated_at = NOW(),
			result_count = $2, attributed_count = $3, document_scope_count = $4,
			scoped_document_count = $5, truncated_count = $6,
			report_json = $7, report_md = $8, error_message = ''
		WHERE id = $1`,
		runID, c.ResultCount, c.AttributedCount, c.DocumentScopeCount,
		c.ScopedDocumentCount, c.TruncatedCount, reportJSON, reportMD)
	return err
}

// FailRun records a failure. The parent request stays re-runnable (spec: Run
// failure is recorded, not lost).
func (s RunStore) FailRun(ctx context.Context, runID int64, msg string) error {
	if strings.TrimSpace(msg) == "" {
		msg = "run failed"
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.product_review_runs
		SET status = 'failed', finished_at = NOW(), updated_at = NOW(), error_message = $2
		WHERE id = $1`, runID, msg)
	return err
}

func (s RunStore) GetRun(ctx context.Context, runID int64) (*Run, error) {
	r := Run{}
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, request_id, run_number, status, started_at, finished_at,
		       result_count, attributed_count, document_scope_count, scoped_document_count,
		       truncated_count, report_json, report_md, error_message, created_at, updated_at
		FROM kb.product_review_runs WHERE id = $1`, runID,
	).Scan(&r.ID, &r.RequestID, &r.RunNumber, &r.Status, &r.StartedAt, &r.FinishedAt,
		&r.ResultCount, &r.AttributedCount, &r.DocumentScopeCount, &r.ScopedDocumentCount,
		&r.TruncatedCount, &r.ReportJSON, &r.ReportMD, &r.ErrorMessage, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s RunStore) scanRuns(rows *sql.Rows) ([]Run, error) {
	defer func() { _ = rows.Close() }()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.RequestID, &r.RunNumber, &r.Status, &r.StartedAt, &r.FinishedAt,
			&r.ResultCount, &r.AttributedCount, &r.DocumentScopeCount, &r.ScopedDocumentCount,
			&r.TruncatedCount, &r.ReportJSON, &r.ReportMD, &r.ErrorMessage, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

const runSelect = `
	SELECT id, request_id, run_number, status, started_at, finished_at,
	       result_count, attributed_count, document_scope_count, scoped_document_count,
	       truncated_count, report_json, report_md, error_message, created_at, updated_at
	FROM kb.product_review_runs`

func (s RunStore) ListRuns(ctx context.Context, requestID int64) ([]Run, error) {
	rows, err := s.DB.QueryContext(ctx, runSelect+` WHERE request_id = $1 ORDER BY run_number DESC`, requestID)
	if err != nil {
		return nil, err
	}
	return s.scanRuns(rows)
}

// PreviousRun returns the highest-numbered run of the request before beforeRunNumber.
func (s RunStore) PreviousRun(ctx context.Context, requestID int64, beforeRunNumber int) (*Run, error) {
	rows, err := s.DB.QueryContext(ctx, runSelect+`
		WHERE request_id = $1 AND run_number < $2
		ORDER BY run_number DESC LIMIT 1`, requestID, beforeRunNumber)
	if err != nil {
		return nil, err
	}
	runs, err := s.scanRuns(rows)
	if err != nil || len(runs) == 0 {
		return nil, err
	}
	return &runs[0], nil
}

// SaveScopedDocs replaces the run's scoped-document set.
func (s RunStore) SaveScopedDocs(ctx context.Context, runID int64, docs []ScopedDoc) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.product_review_run_documents WHERE run_id = $1`, runID); err != nil {
		return err
	}
	for _, d := range docs {
		nodeIDs, _ := json.Marshal(orEmptyInts(d.NodeIDs))
		paths, _ := json.Marshal(orEmpty(d.Paths))
		reasons, _ := json.Marshal(orEmpty(d.Reasons))
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb.product_review_run_documents
				(run_id, input_record_id, fused_score, matching_node_ids, matching_paths, match_reasons, doc_kind)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			runID, d.InputRecordID, d.FusedScore, nodeIDs, paths, reasons, d.DocKind); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SaveResults replaces the run's result rows.
func (s RunStore) SaveResults(ctx context.Context, runID int64, results []Result) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM kb.product_review_results WHERE run_id = $1`, runID); err != nil {
		return err
	}
	for _, r := range results {
		paths, _ := json.Marshal(orEmpty(r.Paths))
		spans := r.LineSpans
		if len(spans) == 0 {
			spans = json.RawMessage("[]")
		}
		var nodeID any
		if r.NodeID != nil {
			nodeID = *r.NodeID
		}
		var sourceRowID any
		if r.SourceRowID != 0 {
			sourceRowID = r.SourceRowID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb.product_review_results
				(run_id, artifact_type, artifact_id, source_row_id, input_record_id,
				 node_id, tier, score, paths, inclusion_reason, source_line_spans)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			runID, r.ArtifactType, r.ArtifactID, sourceRowID, r.InputRecordID,
			nodeID, r.Tier, r.Score, paths, r.InclusionReason, []byte(spans)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s RunStore) LoadScopedDocs(ctx context.Context, runID int64) ([]ScopedDoc, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT input_record_id, fused_score, COALESCE(matching_node_ids,'[]'::jsonb),
		       COALESCE(matching_paths,'[]'::jsonb), COALESCE(match_reasons,'[]'::jsonb), doc_kind
		FROM kb.product_review_run_documents
		WHERE run_id = $1 ORDER BY fused_score DESC, input_record_id`, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []ScopedDoc
	for rows.Next() {
		var d ScopedDoc
		var nodeRaw, pathRaw, reasonRaw []byte
		if err := rows.Scan(&d.InputRecordID, &d.FusedScore, &nodeRaw, &pathRaw, &reasonRaw, &d.DocKind); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(nodeRaw, &d.NodeIDs)
		_ = json.Unmarshal(pathRaw, &d.Paths)
		_ = json.Unmarshal(reasonRaw, &d.Reasons)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s RunStore) LoadResults(ctx context.Context, runID int64, f ResultFilters) ([]Result, error) {
	q := `
		SELECT artifact_type, artifact_id, COALESCE(source_row_id,0), input_record_id,
		       node_id, tier, score, COALESCE(paths,'[]'::jsonb), inclusion_reason,
		       COALESCE(source_line_spans,'[]'::jsonb)
		FROM kb.product_review_results WHERE run_id = $1`
	args := []any{runID}
	add := func(cond string, val any) {
		args = append(args, val)
		q += fmt.Sprintf(" AND %s$%d", cond, len(args))
	}
	if f.NodeID != nil {
		add("node_id = ", *f.NodeID)
	}
	if f.Tier != "" {
		add("tier = ", f.Tier)
	}
	if f.InputRecordID != nil {
		add("input_record_id = ", *f.InputRecordID)
	}
	if f.ArtifactType != "" {
		add("artifact_type = ", f.ArtifactType)
	}
	if f.Path != "" {
		add("paths ? ", f.Path)
	}
	for _, ex := range f.ExcludeTiers {
		add("tier <> ", ex)
	}
	q += ` ORDER BY score DESC, artifact_type, artifact_id`

	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Result
	for rows.Next() {
		var (
			r        Result
			nodeID   sql.NullInt64
			pathsRaw []byte
			spansRaw []byte
		)
		if err := rows.Scan(&r.ArtifactType, &r.ArtifactID, &r.SourceRowID, &r.InputRecordID,
			&nodeID, &r.Tier, &r.Score, &pathsRaw, &r.InclusionReason, &spansRaw); err != nil {
			return nil, err
		}
		if nodeID.Valid {
			v := nodeID.Int64
			r.NodeID = &v
		}
		_ = json.Unmarshal(pathsRaw, &r.Paths)
		r.LineSpans = json.RawMessage(spansRaw)
		out = append(out, r)
	}
	return out, rows.Err()
}

func orEmptyMap(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func orEmptyInts(s []int64) []int64 {
	if s == nil {
		return []int64{}
	}
	return s
}
