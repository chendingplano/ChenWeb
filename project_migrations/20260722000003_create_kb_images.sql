-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

-- One row per library image. Bytes live on the filesystem under IMAGE_DIR
-- (see server/api/imagehandler); this table holds metadata only. `origin`
-- distinguishes user uploads ('upload') from AI-generated covers ('generated');
-- `prompt` records the generation prompt when origin='generated'.
CREATE TABLE IF NOT EXISTS kb.images (
    id           BIGSERIAL   PRIMARY KEY,
    filename     TEXT        NOT NULL,
    stored_path  TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    content_type TEXT        NOT NULL,
    origin       VARCHAR(16) NOT NULL DEFAULT 'upload',
    prompt       TEXT,
    created_by   VARCHAR(255),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_images_created_at
    ON kb.images (created_at DESC);

COMMENT ON TABLE kb.images IS
    'Library images (covers). Bytes stored on the filesystem under IMAGE_DIR; '
    'metadata only here. origin = upload | generated; prompt set when generated.';

-- +goose Down
DROP TABLE IF EXISTS kb.images;
