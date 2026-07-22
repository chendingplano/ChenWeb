-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

-- One row per uploaded video. Bytes live on the filesystem under VIDEO_DIR
-- (see server/api/videohandler); this table holds only metadata. stored_path is
-- the absolute path on disk; the (page-owned) indirection leaves room to swap in
-- object storage later without changing this contract. Column names avoid SQL
-- reserved words (size -> size_bytes).
CREATE TABLE IF NOT EXISTS kb.videos (
    id           BIGSERIAL   PRIMARY KEY,
    filename     TEXT        NOT NULL,
    stored_path  TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    content_type TEXT        NOT NULL,
    uploaded_by  VARCHAR(255),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_videos_created_at
    ON kb.videos (created_at DESC);

COMMENT ON TABLE kb.videos IS
    'One row per uploaded training video. Bytes are stored on the filesystem '
    'under VIDEO_DIR; this table holds metadata only (original filename, stored '
    'path, size, content type, uploader, timestamp).';

-- +goose Down
DROP TABLE IF EXISTS kb.videos;
