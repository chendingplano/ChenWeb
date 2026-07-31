-- +goose Up
-- The activation pointer for ontology modules (ADR DR2): at most one active
-- release per module (partial unique index over rows with deactivated_at
-- NULL). Activation replaces the current pointer by deactivating it first and
-- inserting a new row -- nothing is deleted, so activation/rollback history is
-- preserved and re-pointing at an older release is just another activation
-- row.
CREATE TABLE IF NOT EXISTS kb.ontology_active_releases (
    id              BIGSERIAL PRIMARY KEY,
    module_id       TEXT NOT NULL,
    release_id      BIGINT NOT NULL REFERENCES kb.ontology_module_releases(id) ON DELETE RESTRICT,
    activated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    activated_by    TEXT,
    deactivated_at  TIMESTAMPTZ,
    deactivated_by  TEXT,
    UNIQUE (module_id, release_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_active_releases_one_per_module
    ON kb.ontology_active_releases (module_id) WHERE deactivated_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_active_releases;
