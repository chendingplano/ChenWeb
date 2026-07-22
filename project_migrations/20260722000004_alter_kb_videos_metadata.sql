-- +goose Up
-- Enrich kb.videos with descriptive metadata and a cover image reference.
-- Additive and backward-compatible: existing rows keep working (responses fall
-- back to `filename` when `name` is empty). `source` ∈ {Recording, Web}; `url`
-- carries the external link when source='Web'. `image_id` is a soft (nullable)
-- reference to kb.images(id) — no cascade; a deleted cover just 404s and the UI
-- falls back to a placeholder.
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS name        TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS source      VARCHAR(16) NOT NULL DEFAULT 'Recording';
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS url         TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS image_id    BIGINT;

-- +goose Down
ALTER TABLE kb.videos DROP COLUMN IF EXISTS image_id;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS url;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS source;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS description;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS name;
