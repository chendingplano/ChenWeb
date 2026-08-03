-- +goose Up
-- P3 Track B: keyword lexicon — surfaces (DR16 spec §3.2).
-- surface_id is opaque and content-derived (kws_<sha256_prefix_12>),
-- not a BIGSERIAL. Stores the verbatim surface text — never only a derived key.
-- (norm_key, concept_id, scope, label_role) is unique: one surface form per
-- concept per scope per role. norm_key is the working-mode index key;
-- norm_version signals when a normalizer change requires re-indexing.

CREATE TABLE IF NOT EXISTS kb.keyword_surfaces (
    surface_id      TEXT PRIMARY KEY,
    concept_id      TEXT NOT NULL REFERENCES kb.keyword_concepts(concept_id),
    surface         TEXT NOT NULL,
    norm_key        TEXT NOT NULL,
    norm_version    INT NOT NULL,
    label_role      TEXT NOT NULL DEFAULT 'pref'
                    CHECK (label_role IN ('pref', 'alt', 'hidden')),
    alias_type      TEXT NOT NULL DEFAULT 'synonym',
    lang            TEXT NOT NULL DEFAULT 'en',
    scope           TEXT NOT NULL DEFAULT '_',
    confidence      DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    provenance      TEXT NOT NULL DEFAULT 'human:',
    locked          BOOLEAN NOT NULL DEFAULT FALSE,
    evidence        TEXT,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_keyword_surfaces_norm_concept_scope_role
    ON kb.keyword_surfaces (norm_key, concept_id, scope, label_role);
CREATE INDEX IF NOT EXISTS idx_keyword_surfaces_norm_key_scope
    ON kb.keyword_surfaces (norm_key, scope);
CREATE INDEX IF NOT EXISTS idx_keyword_surfaces_concept_id
    ON kb.keyword_surfaces (concept_id);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_surfaces;
