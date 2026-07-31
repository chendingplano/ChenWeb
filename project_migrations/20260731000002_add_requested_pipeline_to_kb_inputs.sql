-- +goose Up
ALTER TABLE kb.inputs
    ADD COLUMN IF NOT EXISTS requested_pipeline TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_inputs_requested_pipeline
    ON kb.inputs (requested_pipeline);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_inputs_requested_pipeline;

ALTER TABLE kb.inputs
    DROP COLUMN IF EXISTS requested_pipeline;
