-- +goose Up
-- Spec 2026080403 §19 step 7 (K4 + §13.3):
--  - K4/§10.4: kb.keyword_mentions held nothing reconcilable — no column for
--    the observed string, chunk_ref/ks_id always null, context_text always
--    empty. Replaced by kb.keyword_occurrences with the §9.5 shape: the raw
--    name, the derived norm key, consumer-supplied provenance
--    (artifact_type/artifact_id/field_path), the resolution outcome, and a
--    link to the decision-log row from the same call.
--  - §13.3 shapes designed in now, because retrofitting them later means
--    migrating every surface row: multi-source evidence (§13.3.1), external
--    identity mapping (§13.3.2), source/release/license registry (§13.3.3).

DROP TABLE IF EXISTS kb.keyword_mentions;

CREATE TABLE IF NOT EXISTS kb.keyword_occurrences (
    occurrence_id     BIGSERIAL PRIMARY KEY,
    artifact_type     TEXT NOT NULL DEFAULT '',
    artifact_id       TEXT NOT NULL DEFAULT '',
    field_path        TEXT NOT NULL DEFAULT '',
    raw_name          TEXT NOT NULL,
    norm_key          TEXT NOT NULL,
    scope             TEXT NOT NULL DEFAULT '_',
    context_text      TEXT NOT NULL DEFAULT '',
    chunk_ref         TEXT,
    concept_id        TEXT REFERENCES kb.keyword_concepts(concept_id),
    term_id           TEXT,
    resolution_status TEXT NOT NULL
                      CHECK (resolution_status IN
                          ('term_resolved', 'lexical_resolved', 'ambiguous',
                           'unresolved', 'disabled')),
    decision_log_id   BIGINT REFERENCES kb.semid_decision_log(id),
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_keyword_occurrences_norm_scope
    ON kb.keyword_occurrences (norm_key, scope);
CREATE INDEX IF NOT EXISTS idx_keyword_occurrences_concept
    ON kb.keyword_occurrences (concept_id);
CREATE INDEX IF NOT EXISTS idx_keyword_occurrences_artifact
    ON kb.keyword_occurrences (artifact_type, artifact_id);

-- §13.3.3: source/release/license registry — recorded before import, because
-- some sources carry real restrictions that must survive into the data.
CREATE TABLE IF NOT EXISTS kb.keyword_sources (
    source       TEXT NOT NULL,
    release      TEXT NOT NULL DEFAULT '',
    license      TEXT NOT NULL DEFAULT '',
    retrieved_at TIMESTAMPTZ,
    notes        TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (source, release)
);

-- §13.3.2: external identity mapping (source, external_id, release) ->
-- concept_id, so re-import is idempotent.
CREATE TABLE IF NOT EXISTS kb.keyword_external_ids (
    source      TEXT NOT NULL,
    external_id TEXT NOT NULL,
    release     TEXT NOT NULL DEFAULT '',
    concept_id  TEXT NOT NULL REFERENCES kb.keyword_concepts(concept_id),
    PRIMARY KEY (source, external_id, release)
);

CREATE INDEX IF NOT EXISTS idx_keyword_external_ids_concept
    ON kb.keyword_external_ids (concept_id);

-- §13.3.1: multi-source evidence per surface. A single provenance string on
-- the surface cannot represent two sources independently asserting the same
-- alias; here one source's support can be retracted without destroying the
-- other's. CASCADE on surface delete.
CREATE TABLE IF NOT EXISTS kb.keyword_surface_evidence (
    id          BIGSERIAL PRIMARY KEY,
    surface_id  TEXT NOT NULL REFERENCES kb.keyword_surfaces(surface_id)
                ON DELETE CASCADE,
    source      TEXT NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    evidence    TEXT NOT NULL DEFAULT '',
    confidence  DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    UNIQUE (surface_id, source, external_id)
);

-- +goose Down
DROP TABLE IF EXISTS kb.keyword_surface_evidence;
DROP TABLE IF EXISTS kb.keyword_external_ids;
DROP TABLE IF EXISTS kb.keyword_sources;
DROP TABLE IF EXISTS kb.keyword_occurrences;
-- kb.keyword_mentions is deliberately not restored: it held nothing
-- reconcilable (K4), and nothing outside the keywords package read it.
