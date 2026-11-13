-- +goose Up
ALTER TABLE kb.doc_group_exclusions
    ADD COLUMN IF NOT EXISTS create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- +goose Down
ALTER TABLE kb.doc_group_exclusions
    DROP COLUMN IF EXISTS update_time,
    DROP COLUMN IF EXISTS create_time;
