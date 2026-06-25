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

	// Status transitions on progress: pending/running -> success once the
	// reviewer has finished every unit (progress reaches 1.0); 'success' and
	// 'failed' are terminal and never reverted. An aspect at 100% must read
	// 'success', not linger on 'running' until the whole run is finalized.
	// finding_count is monotonic and, at completion, equals the count of this
	// aspect's persisted findings (findings save 1:1), so finalizeAspectsSuccess
	// safely skips an aspect this marks done. end_time is stamped on the
	// running -> success transition only.
	_, err := s.DB.ExecContext(ctx, `
		UPDATE kb.doc_review_status
		SET status = CASE
		        WHEN status IN ('success', 'failed') THEN status
		        WHEN GREATEST(progress, $3) >= 1 THEN 'success'
		        ELSE 'running'
		    END,
		    progress = GREATEST(progress, $3),
		    finding_count = GREATEST(finding_count, $4),
		    end_time = CASE
		        WHEN status NOT IN ('success', 'failed') AND GREATEST(progress, $3) >= 1
		            THEN NOW()
		        ELSE end_time
		    END,
		    modify_time = NOW()
		WHERE review_run_id = $1 AND aspect = $2`,
		reviewRunID, aspect, progress, findingCount)
	if err != nil {
		return fmt.Errorf("update aspect progress for %s/%s: %w", reviewRunID, aspect, err)
	}
	return nil
}
