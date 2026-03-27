-- +goose Up
-- +goose StatementBegin
ALTER TABLE kb.inputs
ADD COLUMN IF NOT EXISTS md5 VARCHAR(64);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS idx_kb_inputs_md5 ON kb.inputs (md5);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_kb_inputs_md5;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE kb.inputs
DROP COLUMN IF EXISTS md5;
-- +goose StatementEnd
