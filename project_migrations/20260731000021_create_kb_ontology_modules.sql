-- +goose Up
-- Ontology module identity and metadata (DB-native storage, 2026-07-31:
-- content lives in the database, not a data-only repository). One row per
-- module_id; released snapshots and their versions live in
-- kb.ontology_module_releases, so a module row tracks current metadata only.
CREATE TABLE IF NOT EXISTS kb.ontology_modules (
    id            BIGSERIAL PRIMARY KEY,
    module_id     TEXT NOT NULL UNIQUE,
    title         TEXT,
    owner         TEXT,
    description   TEXT,
    depends_on    TEXT[] NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    create_time   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by     TEXT,
    modify_time   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by     TEXT
);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_modules;
