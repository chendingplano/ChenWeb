-- +goose Up
-- P3 Track B: keyword lexicon — governed keyword concepts (DR16 spec §3.2).
-- concept_id is opaque and immutable; label is mutable display.
-- Concepts are ungoverned (no draft/review lifecycle) — status transitions
-- are linear: active -> provisional -> merged -> deprecated.
-- Merges are tombstones (merged_into), never deletes.

CREATE TABLE IF NOT EXISTS kb.keyword_concepts (
    concept_id      TEXT PRIMARY KEY,
    pref_label      TEXT NOT NULL,
    gloss           TEXT,
    scope           TEXT NOT NULL DEFAULT '_',
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'provisional', 'merged', 'deprecated')),
    merged_into     TEXT REFERENCES kb.keyword_concepts(concept_id),
    gloss_source    TEXT NOT NULL DEFAULT 'none',
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_keyword_concepts_scope_status
    ON kb.keyword_concepts (scope, status);
CREATE INDEX IF NOT EXISTS idx_keyword_concepts_pref_label
    ON kb.keyword_concepts (pref_label);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_concepts;
