-- +goose Up
-- Immutable module releases (ADR DR2, DB-native): one row per released
-- snapshot of a module's approved content. payload holds the full snapshot
-- (terms/labels/axioms/mappings) for reproducibility; content_checksum is a
-- deterministic hash of that snapshot; dependency_releases pins the release
-- version of each declared dependency. Releases are never edited -- a
-- correction is a new release. superseded_by_release_id records the later
-- release that replaced this one.
CREATE TABLE IF NOT EXISTS kb.ontology_module_releases (
    id                    BIGSERIAL PRIMARY KEY,
    module_id             TEXT NOT NULL,
    version               TEXT NOT NULL,
    title                 TEXT,
    payload               JSONB NOT NULL,
    content_checksum      TEXT NOT NULL,
    dependency_releases   JSONB NOT NULL DEFAULT '{}',
    superseded_by_release_id BIGINT,
    released_by           TEXT,
    released_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_module_releases_module_version
    ON kb.ontology_module_releases (module_id, version);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_module_releases_module
    ON kb.ontology_module_releases (module_id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_module_releases;
