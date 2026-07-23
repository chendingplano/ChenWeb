-- +goose Up
ALTER TABLE kb.entities
    ADD COLUMN IF NOT EXISTS object_link_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS object_link_attempts INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS object_link_last_attempt_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS object_link_fingerprint TEXT;

ALTER TABLE kb.entities
    DROP CONSTRAINT IF EXISTS chk_kb_entities_object_link_status;

ALTER TABLE kb.entities
    ADD CONSTRAINT chk_kb_entities_object_link_status
        CHECK (object_link_status IN ('pending', 'excluded', 'linked', 'deferred', 'exhausted'));

CREATE INDEX IF NOT EXISTS idx_kb_entities_object_link_status
    ON kb.entities (object_link_status);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_entities_object_link_status;

ALTER TABLE kb.entities
    DROP CONSTRAINT IF EXISTS chk_kb_entities_object_link_status;

ALTER TABLE kb.entities
    DROP COLUMN IF EXISTS object_link_status,
    DROP COLUMN IF EXISTS object_link_attempts,
    DROP COLUMN IF EXISTS object_link_last_attempt_at,
    DROP COLUMN IF EXISTS object_link_fingerprint;
