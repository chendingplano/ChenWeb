-- +goose Up
ALTER TABLE kb.inputs
    ADD COLUMN IF NOT EXISTS processing_mode TEXT NOT NULL DEFAULT 'auto';

UPDATE kb.inputs
SET processing_mode = 'auto'
WHERE processing_mode IS NULL OR BTRIM(processing_mode) = '';

ALTER TABLE kb.inputs
    DROP CONSTRAINT IF EXISTS kb_inputs_processing_mode_check;

ALTER TABLE kb.inputs
    ADD CONSTRAINT kb_inputs_processing_mode_check
    CHECK (processing_mode IN ('auto', 'upload_only', 'pdf_parsing'));

CREATE INDEX IF NOT EXISTS idx_kb_inputs_processing_mode
    ON kb.inputs (processing_mode);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_inputs_processing_mode;
ALTER TABLE kb.inputs DROP CONSTRAINT IF EXISTS kb_inputs_processing_mode_check;
ALTER TABLE kb.inputs DROP COLUMN IF EXISTS processing_mode;
