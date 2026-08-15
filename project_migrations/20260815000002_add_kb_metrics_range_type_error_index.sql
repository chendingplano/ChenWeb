-- +goose Up
-- Speeds up the Resolve Metric Range Types admin page's list query, which is always
-- scoped to WHERE value_range_type_error IS NOT NULL plus an optional input_record_id
-- and/or created_at range filter (see openspec/changes/resolve-metric-range-types).
-- Partial: the indexed row set is only the (small) subset of kb.metrics with an
-- unmapped value_range_type, so this costs little at write/storage time.
CREATE INDEX IF NOT EXISTS idx_kb_metrics_range_type_error
    ON kb.metrics (input_record_id, created_at)
    WHERE value_range_type_error IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metrics_range_type_error;
