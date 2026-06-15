-- +goose Up
-- Change 02 from ADR 2026061302 amendment: entity_en is promoted to a
-- dictionary key the same way relation predicates are. Each normalized
-- English entity name is a first-class artifact connected to entity instances.
CREATE TABLE IF NOT EXISTS kb.entity_names (
    name_id       BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name_key      TEXT        NOT NULL,
    name_raw      TEXT        NOT NULL DEFAULT '',
    display_names JSONB       NOT NULL DEFAULT '[]'::jsonb,
    aliases       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    name_desc     TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'pending_review',
    seen_count    BIGINT      NOT NULL DEFAULT 0,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
    modify_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT entity_names_key_uniq UNIQUE (name_key)
);
CREATE INDEX IF NOT EXISTS idx_kb_entity_names_seen_count
    ON kb.entity_names USING btree (seen_count DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.entity_names;
