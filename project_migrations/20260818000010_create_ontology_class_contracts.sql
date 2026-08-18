-- +goose Up
-- ADR 2026081701 DR2/DR3: a stable class identity may accumulate immutable
-- semantic contracts and independently evaluated capabilities. These tables
-- are additive and do not activate any metric writer or governance reader.

ALTER TABLE kb.ontology_term_headers
    ADD COLUMN IF NOT EXISTS current_contract_revision_id BIGINT;

CREATE TABLE IF NOT EXISTS kb.ontology_class_contract_revisions (
    id                      BIGSERIAL PRIMARY KEY,
    term_id                 TEXT NOT NULL REFERENCES kb.ontology_term_headers (term_id),
    revision                INT NOT NULL,
    contract_schema_version TEXT NOT NULL,
    identity_schema_version TEXT NOT NULL,
    definition_state        TEXT NOT NULL CHECK (definition_state IN (
                                'identity_only', 'partially_defined', 'validated'
                            )),
    contract_payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
    synthesis_method        TEXT NOT NULL,
    confidence              DOUBLE PRECISION,
    policy_version          TEXT,
    provenance              JSONB NOT NULL DEFAULT '{}'::jsonb,
    supersedes_revision_id  BIGINT REFERENCES kb.ontology_class_contract_revisions (id),
    create_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by               TEXT,
    UNIQUE (term_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_class_contract_revisions_term_current
    ON kb.ontology_class_contract_revisions (term_id, revision DESC);

ALTER TABLE kb.ontology_term_headers
    ADD CONSTRAINT fk_kb_ontology_term_headers_current_contract_revision
    FOREIGN KEY (current_contract_revision_id)
    REFERENCES kb.ontology_class_contract_revisions (id);

CREATE TABLE IF NOT EXISTS kb.ontology_class_contract_capabilities (
    contract_revision_id    BIGINT NOT NULL REFERENCES kb.ontology_class_contract_revisions (id),
    capability_term_id      TEXT NOT NULL REFERENCES kb.ontology_term_headers (term_id),
    result_state            TEXT NOT NULL CHECK (result_state IN (
                                'enabled', 'disabled', 'indeterminate'
                            )),
    declared_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    declared_by             TEXT,
    PRIMARY KEY (contract_revision_id, capability_term_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_class_contract_capabilities_term_state
    ON kb.ontology_class_contract_capabilities (capability_term_id, result_state);

CREATE TABLE IF NOT EXISTS kb.ontology_class_capability_validation_results (
    id                      BIGSERIAL PRIMARY KEY,
    contract_revision_id    BIGINT NOT NULL,
    capability_term_id      TEXT NOT NULL,
    validator_id            TEXT NOT NULL CHECK (BTRIM(validator_id) <> ''),
    validator_version       TEXT NOT NULL CHECK (BTRIM(validator_version) <> ''),
    validation_result       TEXT NOT NULL CHECK (validation_result IN (
                                'pass', 'fail', 'indeterminate', 'error'
                            )),
    evidence                JSONB NOT NULL DEFAULT '{}'::jsonb,
    failure_details         JSONB,
    evaluated_time          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    evaluated_by            TEXT,
    FOREIGN KEY (contract_revision_id, capability_term_id)
        REFERENCES kb.ontology_class_contract_capabilities (contract_revision_id, capability_term_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_class_capability_validation_results_capability
    ON kb.ontology_class_capability_validation_results
        (contract_revision_id, capability_term_id, validation_result, evaluated_time DESC);

-- +goose Down
ALTER TABLE kb.ontology_term_headers
    DROP CONSTRAINT IF EXISTS fk_kb_ontology_term_headers_current_contract_revision;
ALTER TABLE kb.ontology_term_headers
    DROP COLUMN IF EXISTS current_contract_revision_id;
DROP TABLE IF EXISTS kb.ontology_class_capability_validation_results;
DROP TABLE IF EXISTS kb.ontology_class_contract_capabilities;
DROP TABLE IF EXISTS kb.ontology_class_contract_revisions;
