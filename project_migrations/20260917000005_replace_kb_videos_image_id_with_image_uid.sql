-- +goose Up
-- Replaces kb.videos.image_id (a surrogate BIGINT reference to kb.images.id,
-- added in 20260722000004_alter_kb_videos_metadata.sql) with image_uid, a
-- reference to the stable kb.images.uid added in
-- 20260917000004_add_uid_to_kb_images.sql. kb.videos and kb.images are
-- synced independently as two datasync table_with_files items, and
-- source/target BIGSERIAL sequences are assigned independently -- a raw
-- copy of image_id has no guaranteed relationship to the correct row in the
-- target's kb.images. uid is portable data, so image_uid survives the sync
-- correctly (see openspec/changes/configurable-data-sync-items design.md
-- Decision 8). Like image_id before it, image_uid carries no REFERENCES
-- constraint -- a soft reference only, by convention: a deleted or
-- not-yet-synced cover just 404s and the UI falls back to a placeholder.
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS image_uid UUID;

UPDATE kb.videos v
SET image_uid = i.uid
FROM kb.images i
WHERE v.image_id = i.id
  AND v.image_uid IS NULL;

ALTER TABLE kb.videos DROP COLUMN IF EXISTS image_id;

-- +goose Down
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS image_id BIGINT;

UPDATE kb.videos v
SET image_id = i.id
FROM kb.images i
WHERE v.image_uid = i.uid
  AND v.image_id IS NULL;

ALTER TABLE kb.videos DROP COLUMN IF EXISTS image_uid;
