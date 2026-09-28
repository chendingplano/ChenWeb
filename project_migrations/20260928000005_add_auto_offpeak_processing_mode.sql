-- +goose Up
ALTER TABLE kb.inputs
    DROP CONSTRAINT IF EXISTS kb_inputs_processing_mode_check;

ALTER TABLE kb.inputs
    ADD CONSTRAINT kb_inputs_processing_mode_check
    CHECK (processing_mode IN ('auto', 'auto_offpeak', 'upload_only', 'pdf_parsing'));

-- +goose Down
UPDATE kb.inputs SET processing_mode = 'auto' WHERE processing_mode = 'auto_offpeak';

ALTER TABLE kb.inputs
    DROP CONSTRAINT IF EXISTS kb_inputs_processing_mode_check;

ALTER TABLE kb.inputs
    ADD CONSTRAINT kb_inputs_processing_mode_check
    CHECK (processing_mode IN ('auto', 'upload_only', 'pdf_parsing'));
