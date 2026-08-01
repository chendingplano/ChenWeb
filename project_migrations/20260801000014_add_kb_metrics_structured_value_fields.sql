-- +goose Up
-- Structured value fields for kb.metrics (openspec change
-- extract-metrics-structured-output; ADR 2026072901 §8.2 extract_metrics):
-- value_min/value_max carry the two numeric endpoints of a 'range'
-- value_range_type row so normalize_assertions never re-parses free text;
-- condition carries the applicability/scope clause. value_range_type,
-- value_class, metric_value, metric_unit already exist (20260507100002).
ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS value_min DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS value_max DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS condition TEXT;

-- +goose Down
ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS condition,
    DROP COLUMN IF EXISTS value_max,
    DROP COLUMN IF EXISTS value_min;
