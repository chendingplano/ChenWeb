package docreviews

import (
	"context"
	"database/sql"
	"fmt"
)

// ReviewStatusStore persists live per-reviewer progress into kb.doc_review_status.
type ReviewStatusStore interface {
	UpdateAspectProgress(ctx context.Context, reviewRunID, aspect string, progress float64, findingCount int) error
}

// ReviewStatusSQLStore updates kb.doc_review_status rows in PostgreSQL.
type ReviewStatusSQLStore struct {
	DB *sql.DB
}

func (s ReviewStatusSQLStore) UpdateAspectProgress(ctx context.Context, reviewRunID, aspect string, progress float64, findingCount int) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	if reviewRunID == "" || aspect == "" {
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

	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status
		SET status = CASE WHEN status = 'pending' THEN 'running' ELSE status END,
		    progress = GREATEST(progress, $3),
		    finding_count = GREATEST(finding_count, $4),
		    modify_time = NOW()
		WHERE review_run_id = $1 AND aspect = $2`,
		reviewRunID, aspect, progress, findingCount)
	if err != nil {
		return fmt.Errorf("update aspect progress for %s/%s: %w", reviewRunID, aspect, err)
	}
	return nil
}
