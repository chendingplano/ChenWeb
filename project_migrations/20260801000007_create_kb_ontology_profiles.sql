-- +goose Up
-- Governed, versioned review profiles. A profile is normative content only
-- after the module compiler includes it in an activated module release.
CREATE TABLE IF NOT EXISTS kb.ontology_profiles (
    id                      BIGSERIAL PRIMARY KEY,
    profile_id              TEXT NOT NULL,
    version                 INT NOT NULL DEFAULT 1 CHECK (version >= 1),
    module_id               TEXT NOT NULL REFERENCES kb.ontology_modules(module_id) ON DELETE RESTRICT,
    status                  TEXT NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'in_review', 'approved', 'included_in_release', 'superseded', 'rejected')),
    title                   TEXT,
    applicability           JSONB NOT NULL DEFAULT '{}',
    closed_dimensions       JSONB NOT NULL DEFAULT '[]',
    released_in_release_id  BIGINT REFERENCES kb.ontology_module_releases(id) ON DELETE RESTRICT,
    create_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by               TEXT,
    modify_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by               TEXT,
    UNIQUE (profile_id, version)
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_profiles_module_status
    ON kb.ontology_profiles (module_id, status, version DESC);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_profiles_release
    ON kb.ontology_profiles (released_in_release_id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_profiles;
