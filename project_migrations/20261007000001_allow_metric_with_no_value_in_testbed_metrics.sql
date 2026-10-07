-- +goose Up
-- extract-metrics-benchmark rules 4.1.0+: A4 rows (a named metric whose value the document leaves
-- open) are stored with value_class 'metric-with-no-value'. Benchmark-only; kb.metrics never holds it.
ALTER TABLE testbed.metrics DROP CONSTRAINT IF EXISTS metrics_value_class_check;
ALTER TABLE testbed.metrics ADD CONSTRAINT metrics_value_class_check CHECK (value_class IN
    ('observation', 'requirement', 'target', 'reference', 'design_capability', 'definition',
     'metric-with-no-value'));

-- +goose Down
-- Fails while rows with value_class 'metric-with-no-value' exist; remove those runs first.
ALTER TABLE testbed.metrics DROP CONSTRAINT IF EXISTS metrics_value_class_check;
ALTER TABLE testbed.metrics ADD CONSTRAINT metrics_value_class_check CHECK (value_class IN
    ('observation', 'requirement', 'target', 'reference', 'design_capability', 'definition'));
