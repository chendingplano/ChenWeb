package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ProductionPipelinePolicy is the versioned activation envelope DR6 names:
// exactly one policy is ever "active", and kb.pipeline_bindings/
// kb.pipeline_rules rows are authored under a specific policy_id, so
// resolution only ever sees the bindings/rules that belong to whichever
// policy is currently active.
type ProductionPipelinePolicy struct {
	ID          int64
	Version     int
	Status      string
	SourceRef   string
	ActivatedAt time.Time
	ActivatedBy string
}

// PipelinePolicyStore reads the currently active policy.
type PipelinePolicyStore interface {
	GetActivePolicy(ctx context.Context) (ProductionPipelinePolicy, error)
}

type PipelinePolicySQLStore struct {
	DB *sql.DB
}

func (s PipelinePolicySQLStore) GetActivePolicy(ctx context.Context) (ProductionPipelinePolicy, error) {
	if s.DB == nil {
		return ProductionPipelinePolicy{}, errors.New("db is nil")
	}
	const stmt = `
SELECT id, version, status, COALESCE(source_ref, ''), activated_at, COALESCE(activated_by, '')
FROM kb.pipeline_policies
WHERE status = 'active'
LIMIT 1`
	var (
		policy      ProductionPipelinePolicy
		activatedAt sql.NullTime
	)
	err := s.DB.QueryRowContext(ctx, stmt).Scan(
		&policy.ID, &policy.Version, &policy.Status, &policy.SourceRef, &activatedAt, &policy.ActivatedBy,
	)
	if err != nil {
		return ProductionPipelinePolicy{}, err
	}
	if activatedAt.Valid {
		policy.ActivatedAt = activatedAt.Time
	}
	return policy, nil
}
