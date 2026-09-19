-- +goose Up
-- Add portable row identities for data sync. The BIGSERIAL ids are local to
-- each database and must not be used as sync natural keys.
ALTER TABLE kb.page_def
    ADD COLUMN IF NOT EXISTS uid UUID NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE kb.page_config
    ADD COLUMN IF NOT EXISTS uid UUID NOT NULL DEFAULT gen_random_uuid();

-- Use UNIQUE constraints (rather than only unique indexes) so the data-sync
-- natural-key discovery endpoint offers uid as a candidate key.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'kb_page_def_uid_key'
          AND conrelid = 'kb.page_def'::regclass
    ) THEN
        ALTER TABLE kb.page_def ADD CONSTRAINT kb_page_def_uid_key UNIQUE (uid);
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'kb_page_config_uid_key'
          AND conrelid = 'kb.page_config'::regclass
    ) THEN
        ALTER TABLE kb.page_config ADD CONSTRAINT kb_page_config_uid_key UNIQUE (uid);
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.page_config DROP CONSTRAINT IF EXISTS kb_page_config_uid_key;
ALTER TABLE kb.page_config DROP COLUMN IF EXISTS uid;
ALTER TABLE kb.page_def DROP CONSTRAINT IF EXISTS kb_page_def_uid_key;
ALTER TABLE kb.page_def DROP COLUMN IF EXISTS uid;
