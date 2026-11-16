-- +goose Up
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status)
VALUES ('multi_ranges', 'multi-ranges', 'approved')
ON CONFLICT (raw_value) DO UPDATE SET canonical_bucket = EXCLUDED.canonical_bucket, status = EXCLUDED.status;
ALTER TABLE testbed.metrics DROP CONSTRAINT IF EXISTS metrics_value_range_type_check;
ALTER TABLE testbed.metrics ADD CONSTRAINT metrics_value_range_type_check CHECK
    (value_range_type IN ('lower_bound', 'upper_bound', 'exact', 'range', 'multi-ranges', 'qualitative', 'limit_absent'));

-- +goose Down
ALTER TABLE testbed.metrics DROP CONSTRAINT IF EXISTS metrics_value_range_type_check;
ALTER TABLE testbed.metrics ADD CONSTRAINT metrics_value_range_type_check CHECK
    (value_range_type IN ('lower_bound', 'upper_bound', 'exact', 'range', 'qualitative', 'limit_absent'));
DELETE FROM kb.metric_value_range_type_map WHERE raw_value = 'multi_ranges' AND canonical_bucket = 'multi-ranges';
