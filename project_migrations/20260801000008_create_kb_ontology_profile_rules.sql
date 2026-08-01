-- +goose Up
-- Rules are versioned profile content. The concrete rule configuration stays
-- JSONB so each registered rule kind owns its validated shape.
CREATE TABLE IF NOT EXISTS kb.ontology_profile_rules (
    id                      BIGSERIAL PRIMARY KEY,
    rule_id                 TEXT NOT NULL,
    version                 INT NOT NULL DEFAULT 1 CHECK (version >= 1),
    profile_id              TEXT NOT NULL,
    profile_version         INT NOT NULL,
    rule_kind               TEXT NOT NULL,
    status                  TEXT NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft', 'in_review', 'approved', 'included_in_release', 'superseded', 'rejected')),
    severity                TEXT NOT NULL DEFAULT 'error',
    rule_config             JSONB NOT NULL DEFAULT '{}',
    applicability           JSONB NOT NULL DEFAULT '{}',
    released_in_release_id  BIGINT REFERENCES kb.ontology_module_releases(id) ON DELETE RESTRICT,
    create_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by               TEXT,
    modify_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by               TEXT,
    UNIQUE (rule_id, version),
    FOREIGN KEY (profile_id, profile_version)
        REFERENCES kb.ontology_profiles(profile_id, version) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_profile_rules_profile
    ON kb.ontology_profile_rules (profile_id, profile_version, status);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_profile_rules_release
    ON kb.ontology_profile_rules (released_in_release_id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_profile_rules;
