-- +goose Up
-- ADR 2026081401 DR6: extract_metrics flags the specific kb.metrics row when its
-- value_range_type has no approved governed mapping (kb.metric_value_range_type_map),
-- so the affected metric is directly identifiable without joining through
-- kb.semantic_decision_candidates. Additive, nullable; NULL for every existing row is
-- the correct historical value -- this column is only ever set going forward.
ALTER TABLE IF EXISTS kb.metrics
    ADD COLUMN IF NOT EXISTS value_range_type_error TEXT;

-- +goose Down
ALTER TABLE IF EXISTS kb.metrics
    DROP COLUMN IF EXISTS value_range_type_error;
