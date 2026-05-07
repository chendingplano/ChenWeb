-- +goose Up
ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS metric_value TEXT,
    ADD COLUMN IF NOT EXISTS value_data_type TEXT,
    ADD COLUMN IF NOT EXISTS value_range_type TEXT,
    ADD COLUMN IF NOT EXISTS value_class TEXT,
    ADD COLUMN IF NOT EXISTS value_class_en TEXT;

-- +goose Down
ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS metric_value,
    DROP COLUMN IF EXISTS value_data_type,
    DROP COLUMN IF EXISTS value_range_type,
    DROP COLUMN IF EXISTS value_class,
    DROP COLUMN IF EXISTS value_class_en;
