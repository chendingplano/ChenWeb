-- +goose Up
ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS input_filename,
    ADD COLUMN IF NOT EXISTS metric_name_en TEXT,
    ADD COLUMN IF NOT EXISTS metric_subject_en TEXT,
    ADD COLUMN IF NOT EXISTS metric_desc_en TEXT,
    ADD COLUMN IF NOT EXISTS metric_context_en TEXT,
    ADD COLUMN IF NOT EXISTS metric_keywords_en JSONB,
    ADD COLUMN IF NOT EXISTS model_name TEXT,
    ADD COLUMN IF NOT EXISTS prompt_name TEXT,
    ADD COLUMN IF NOT EXISTS metric_unit_en TEXT,
    ADD COLUMN IF NOT EXISTS table_name_or_section TEXT;

-- +goose Down
ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS input_filename TEXT NOT NULL DEFAULT '',
    DROP COLUMN IF EXISTS metric_name_en,
    DROP COLUMN IF EXISTS metric_subject_en,
    DROP COLUMN IF EXISTS metric_desc_en,
    DROP COLUMN IF EXISTS metric_context_en,
    DROP COLUMN IF EXISTS metric_keywords_en,
    DROP COLUMN IF EXISTS model_name,
    DROP COLUMN IF EXISTS prompt_name,
    DROP COLUMN IF EXISTS metric_unit_en,
    DROP COLUMN IF EXISTS table_name_or_section;
