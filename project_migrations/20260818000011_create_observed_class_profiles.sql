-- +goose Up
-- ADR 2026081701 DR6: observed class profiles retain inclusive source-backed
-- structure. They are evidence surfaces only and intentionally have no
-- relationship to authoritative class contract storage.

CREATE TABLE IF NOT EXISTS kb.ontology_observed_class_profiles (
    id                  BIGSERIAL PRIMARY KEY,
    class_term_id       TEXT NOT NULL UNIQUE REFERENCES kb.ontology_term_headers (term_id),
    aggregation_method  TEXT NOT NULL,
    method_version      TEXT NOT NULL,
    confidence          DOUBLE PRECISION,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by           TEXT,
    modify_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by           TEXT
);

CREATE TABLE IF NOT EXISTS kb.ontology_observed_class_attribute_observations (
    id                      BIGSERIAL PRIMARY KEY,
    profile_id              BIGINT NOT NULL REFERENCES kb.ontology_observed_class_profiles (id),
    attribute_key           TEXT NOT NULL,
    observed_name           TEXT,
    logical_datatype        TEXT NOT NULL DEFAULT '',
    value_form              TEXT NOT NULL DEFAULT '',
    unit_term_id            TEXT NOT NULL DEFAULT '',
    cardinality_observation TEXT,
    observation_state       TEXT NOT NULL,
    grouping_method         TEXT NOT NULL,
    confidence              DOUBLE PRECISION,
    observed_count          BIGINT NOT NULL DEFAULT 0 CHECK (observed_count >= 0),
    document_count          BIGINT NOT NULL DEFAULT 0 CHECK (document_count >= 0),
    create_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (profile_id, attribute_key, logical_datatype, value_form, unit_term_id, observation_state)
);

CREATE INDEX IF NOT EXISTS idx_kb_observed_class_attribute_profile
    ON kb.ontology_observed_class_attribute_observations (profile_id, attribute_key);

CREATE TABLE IF NOT EXISTS kb.ontology_observed_class_attribute_distributions (
    id                          BIGSERIAL PRIMARY KEY,
    attribute_observation_id    BIGINT NOT NULL REFERENCES kb.ontology_observed_class_attribute_observations (id),
    distribution_kind           TEXT NOT NULL,
    distribution_value          TEXT NOT NULL DEFAULT '',
    observed_count              BIGINT NOT NULL DEFAULT 0 CHECK (observed_count >= 0),
    document_count              BIGINT NOT NULL DEFAULT 0 CHECK (document_count >= 0),
    evidence_summary            JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (attribute_observation_id, distribution_kind, distribution_value)
);

CREATE TABLE IF NOT EXISTS kb.ontology_observed_class_profile_examples (
    id                          BIGSERIAL PRIMARY KEY,
    profile_id                  BIGINT NOT NULL REFERENCES kb.ontology_observed_class_profiles (id),
    attribute_observation_id    BIGINT REFERENCES kb.ontology_observed_class_attribute_observations (id),
    assertion_id                BIGINT REFERENCES kb.semantic_assertions (id),
    evidence_id                 BIGINT REFERENCES kb.assertion_evidence (id),
    observation_state           TEXT NOT NULL,
    raw_value                   TEXT,
    normalized_value            JSONB,
    source_excerpt              TEXT,
    method                      TEXT NOT NULL,
    confidence                  DOUBLE PRECISION,
    create_time                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_observed_class_profile_examples_profile
    ON kb.ontology_observed_class_profile_examples (profile_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_kb_observed_class_profile_examples_assertion
    ON kb.ontology_observed_class_profile_examples (assertion_id) WHERE assertion_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS kb.ontology_observed_class_profile_exceptions (
    id                          BIGSERIAL PRIMARY KEY,
    profile_id                  BIGINT NOT NULL REFERENCES kb.ontology_observed_class_profiles (id),
    attribute_observation_id    BIGINT REFERENCES kb.ontology_observed_class_attribute_observations (id),
    assertion_id                BIGINT REFERENCES kb.semantic_assertions (id),
    evidence_id                 BIGINT REFERENCES kb.assertion_evidence (id),
    exception_kind              TEXT NOT NULL CHECK (exception_kind IN ('outlier', 'contradiction')),
    observation_state           TEXT NOT NULL,
    details                     JSONB NOT NULL DEFAULT '{}'::jsonb,
    method                      TEXT NOT NULL,
    confidence                  DOUBLE PRECISION,
    create_time                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_observed_class_profile_exceptions_profile
    ON kb.ontology_observed_class_profile_exceptions (profile_id, exception_kind, create_time DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_observed_class_profile_exceptions;
DROP TABLE IF EXISTS kb.ontology_observed_class_profile_examples;
DROP TABLE IF EXISTS kb.ontology_observed_class_attribute_distributions;
DROP TABLE IF EXISTS kb.ontology_observed_class_attribute_observations;
DROP TABLE IF EXISTS kb.ontology_observed_class_profiles;
