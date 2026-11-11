package docreviews

import (
	"context"
	"database/sql"
	"fmt"
)

// ReviewStatusStore persists live per-reviewer progress into kb.doc_review_status.
type ReviewStatusStore interface {
	UpdateAspectProgress(ctx context.Context, runID int64, aspect string, progress float64, findingCount int) error
}

// ReviewAspectResultStore finishes an aspect after its findings are durably
// saved, or records its execution/persistence failure. Progress alone does not
// establish successful completion.
type ReviewAspectResultStore interface {
	FinishAspect(ctx context.Context, runID int64, aspect string, reviewErr error) error
}

// ReviewStatusSQLStore updates kb.doc_review_status rows in PostgreSQL.
type ReviewStatusSQLStore struct {
	DB *sql.DB
}

func (s ReviewStatusSQLStore) UpdateAspectProgress(ctx context.Context, runID int64, aspect string, progress float64, findingCount int) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	if runID <= 0 || aspect == "" {
		return nil
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	if findingCount < 0 {
		findingCount = 0
	}

	// Unit progress is provisional until the processor saves findings and calls
	// FinishAspect. In particular, 100% of model calls is not durable success.
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status
		SET status = 'running',
		    progress = GREATEST(progress, $3),
		    finding_count = GREATEST(finding_count, $4),
		    modify_time = NOW()
		WHERE run_id = $1 AND aspect = $2 AND status NOT IN ('success', 'failed')`,
		runID, aspect, progress, findingCount)
	if err != nil {
		return fmt.Errorf("update aspect progress for run %d/%s: %w", runID, aspect, err)
	}
	return nil
}

func (s ReviewStatusSQLStore) FinishAspect(ctx context.Context, runID int64, aspect string, reviewErr error) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	if runID <= 0 || aspect == "" {
		return nil
	}
	status, errorMessage := "success", ""
	if reviewErr != nil {
		status, errorMessage = "failed", reviewErr.Error()
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status s
		SET status = $3,
		    progress = CASE WHEN $3 = 'success' THEN 1 ELSE progress END,
		    finding_count = (SELECT COUNT(*) FROM kb.doc_review_findings f
		                     WHERE f.run_id = s.run_id AND f.aspect = s.aspect),
		    error_message = $4,
		    end_time = NOW(), modify_time = NOW()
		WHERE s.run_id = $1 AND s.aspect = $2 AND s.status NOT IN ('success', 'failed')`,
		runID, aspect, status, errorMessage)
	if err != nil {
		return fmt.Errorf("finish aspect for run %d/%s: %w", runID, aspect, err)
	}
	return nil
}
