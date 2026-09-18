-- +goose Up
-- Adds a stable, portable identifier for kb.images rows (see
-- openspec/changes/configurable-data-sync-items design.md Decision 8).
-- stored_path is rewritten to a fresh local path every time a
-- table_with_files sync materializes this row's file on a new instance, so
-- it can't be used as a cross-table reference from kb.videos.image_uid; uid
-- is assigned once at creation and travels as plain synced data, never
-- touched by file materialization.
ALTER TABLE kb.images ADD COLUMN IF NOT EXISTS uid UUID NOT NULL DEFAULT gen_random_uuid();

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'kb_images_uid_key'
          AND conrelid = 'kb.images'::regclass
    ) THEN
        ALTER TABLE kb.images ADD CONSTRAINT kb_images_uid_key UNIQUE (uid);
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.images DROP CONSTRAINT IF EXISTS kb_images_uid_key;
ALTER TABLE kb.images DROP COLUMN IF EXISTS uid;
