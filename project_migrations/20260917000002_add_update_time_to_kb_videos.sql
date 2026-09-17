-- +goose Up
-- Makes kb.videos syncable as a table_with_files data-sync item (see
-- openspec/changes/configurable-data-sync-items): a valid CursorCol needs a
-- column reliably bumped on every UPDATE (kb.videos only had insert-time
-- created_at), and a valid NaturalKey needs a real UNIQUE constraint that
-- isn't the surrogate id. stored_path is unique in practice already --
-- UploadVideo names it "<UnixNano>_<sanitized-filename>" -- so the
-- constraint is safe to add without a backfill conflict.
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ;
UPDATE kb.videos SET update_time = created_at WHERE update_time IS NULL;
ALTER TABLE kb.videos ALTER COLUMN update_time SET NOT NULL;
ALTER TABLE kb.videos ALTER COLUMN update_time SET DEFAULT NOW();

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'kb_videos_stored_path_key'
          AND conrelid = 'kb.videos'::regclass
    ) THEN
        ALTER TABLE kb.videos ADD CONSTRAINT kb_videos_stored_path_key UNIQUE (stored_path);
    END IF;
END $$;
-- +goose StatementEnd

-- Reuses kb.set_update_time(), created in
-- 20260915000002_add_kb_set_update_time_trigger.sql for kb.product_names.
DROP TRIGGER IF EXISTS trg_kb_videos_set_update_time ON kb.videos;
CREATE TRIGGER trg_kb_videos_set_update_time
    BEFORE UPDATE ON kb.videos
    FOR EACH ROW
    EXECUTE FUNCTION kb.set_update_time();

-- +goose Down
-- Only drops what this migration added -- kb.set_update_time() itself is
-- owned by 20260915000002 and also used by kb.product_names' trigger, so it
-- must not be dropped here (same dependent-order caveat noted there).
DROP TRIGGER IF EXISTS trg_kb_videos_set_update_time ON kb.videos;
ALTER TABLE kb.videos DROP CONSTRAINT IF EXISTS kb_videos_stored_path_key;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS update_time;
