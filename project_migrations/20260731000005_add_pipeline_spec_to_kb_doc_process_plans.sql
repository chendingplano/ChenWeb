-- +goose Up
ALTER TABLE kb.doc_process_plans
    ADD COLUMN IF NOT EXISTS pipeline_spec JSONB;

-- +goose Down
ALTER TABLE kb.doc_process_plans
    DROP COLUMN IF EXISTS pipeline_spec;
