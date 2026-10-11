-- +goose Up
ALTER TABLE kb.ref_mentions ADD COLUMN IF NOT EXISTS source_table_rows JSONB;
COMMENT ON COLUMN kb.ref_mentions.source_table_rows IS
    'Source table line numbers, logical row IDs, and system-computed row hashes; same shape as kb.metrics.source_table_rows.';

-- +goose Down
ALTER TABLE kb.ref_mentions DROP COLUMN IF EXISTS source_table_rows;
