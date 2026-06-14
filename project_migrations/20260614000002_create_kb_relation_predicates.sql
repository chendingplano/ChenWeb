-- +goose Up
-- ADR 2026061401 — relation predicates are first-class artifacts (D2). This is the
-- predicate dictionary referenced by the canonical relation store: each distinct
-- normalized English predicate gets one row, and (relation_predicate)->(relation)
-- edges in kb.artifact_connections point at predicate_key.
CREATE TABLE IF NOT EXISTS kb.relation_predicates (
    predicate_id   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    predicate_key  TEXT        NOT NULL,                 -- normalized English predicate (snake_case)
    predicate_raw  TEXT        NOT NULL DEFAULT '',      -- first-seen original-language predicate
    display_names  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    aliases        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    predicate_desc TEXT        NOT NULL DEFAULT '',
    status         TEXT        NOT NULL DEFAULT 'pending_review',
    seen_count     BIGINT      NOT NULL DEFAULT 0,
    first_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    create_time    TIMESTAMPTZ NOT NULL DEFAULT now(),
    modify_time    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT relation_predicates_key_uniq UNIQUE (predicate_key)
);
CREATE INDEX IF NOT EXISTS idx_kb_relation_predicates_seen_count
    ON kb.relation_predicates USING btree (seen_count DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.relation_predicates;
