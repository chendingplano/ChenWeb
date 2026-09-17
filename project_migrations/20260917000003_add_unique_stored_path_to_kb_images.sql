-- +goose Up
-- Makes kb.images usable as a table_with_files data-sync natural key (see
-- openspec/changes/configurable-data-sync-items): a valid NaturalKey needs a
-- real UNIQUE constraint that isn't the surrogate id, and kb.images only had
-- its primary key. Unlike kb.videos, kb.images rows are never edited after
-- upload (no update path in imagehandler), so created_at is already a valid
-- CursorCol and no update_time/trigger is needed here -- just the
-- constraint. stored_path is unique in practice already (no duplicates as
-- of this writing), so the constraint is safe to add without a backfill
-- conflict.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'kb_images_stored_path_key'
          AND conrelid = 'kb.images'::regclass
    ) THEN
        ALTER TABLE kb.images ADD CONSTRAINT kb_images_stored_path_key UNIQUE (stored_path);
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.images DROP CONSTRAINT IF EXISTS kb_images_stored_path_key;
