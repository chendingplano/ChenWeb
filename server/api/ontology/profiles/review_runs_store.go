package profiles

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terms"
)

// ReviewRun records one concrete evaluation of an immutable review scope,
// pinning the assertion watermark it evaluated against. The scope stays
// reusable across many runs; each run is a separate, reproducible snapshot,
// mirroring how kb.ontology_comparison_runs pins state separately from
// kb.ontology_comparison_scopes.
type ReviewRun struct {
	ID                 int64     `json:"id"`
	ReviewScopeID      string    `json:"review_scope_id"`
	InputRecordID      int64     `json:"input_record_id"`
	AssertionWatermark string    `json:"assertion_watermark"`
	CreateTime         time.Time `json:"create_time"`
}

type ReviewRunStore struct{ DB terms.DBX }

func (s ReviewRunStore) CreateRun(ctx context.Context, run ReviewRun) (ReviewRun, error) {
	if s.DB == nil {
		return ReviewRun{}, errors.New("db is nil")
	}
	if strings.TrimSpace(run.ReviewScopeID) == "" || run.InputRecordID == 0 || strings.TrimSpace(run.AssertionWatermark) == "" {
		return ReviewRun{}, errors.New("review run provenance is required")
	}
	const stmt = `INSERT INTO kb.ontology_review_runs (review_scope_id, input_record_id, assertion_watermark) VALUES ($1, $2, $3) RETURNING id, review_scope_id, input_record_id, assertion_watermark, create_time`
	var out ReviewRun
	err := s.DB.QueryRowContext(ctx, stmt, run.ReviewScopeID, run.InputRecordID, run.AssertionWatermark).
		Scan(&out.ID, &out.ReviewScopeID, &out.InputRecordID, &out.AssertionWatermark, &out.CreateTime)
	return out, err
}
