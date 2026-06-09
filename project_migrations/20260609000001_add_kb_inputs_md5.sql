-- +goose Up
ALTER TABLE kb.inputs ADD COLUMN IF NOT EXISTS md5 TEXT;
CREATE INDEX IF NOT EXISTS idx_kb_inputs_md5 ON kb.inputs(md5) WHERE md5 IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_kb_inputs_md5;
ALTER TABLE kb.inputs DROP COLUMN IF EXISTS md5;
