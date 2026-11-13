-- +goose Up
ALTER TABLE kb.doc_group_members
    ADD COLUMN IF NOT EXISTS create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- +goose Down
ALTER TABLE kb.doc_group_members
    DROP COLUMN IF EXISTS update_time,
    DROP COLUMN IF EXISTS create_time;
