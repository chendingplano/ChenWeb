-- +goose Up
-- ADR 2026081801 DR6/DR9: the cross-family minimum field and state contract.
--
-- The existing table compresses every kind of uncertainty into `status`. DR9 is
-- explicit that these axes are independent and can coexist -- an assertion can
-- be `represented` (lifecycle), `resolved_existing` (class identity),
-- `datatype_mismatch` (value), and `contract_violation` (conformance) at the
-- same time. Storing them as separate governed-term columns is what lets a
-- consumer ask "is this parsed?" without inferring it from the governance
-- status.
--
-- All four state columns are governed term IDs, not CHECK-constrained enums,
-- for the same reason as the disposition/finding vocabularies: DR9's list is
-- the initial set, not a closed one.
ALTER TABLE kb.semantic_assertions
    ADD COLUMN IF NOT EXISTS class_identity_state_term_id      TEXT,
    ADD COLUMN IF NOT EXISTS mapping_resolution_state_term_id  TEXT,
    ADD COLUMN IF NOT EXISTS value_state_term_id               TEXT,
    ADD COLUMN IF NOT EXISTS conformance_state_term_id         TEXT,
    -- DR2: the immutable normalization-time snapshot of the raw occurrence,
    -- plus the fingerprint of the occurrence revision it was copied from. The
    -- artifact-family row stays authoritative for the CURRENT raw occurrence;
    -- this is history, and reprocessing must not mutate it to resemble a newer
    -- extraction.
    ADD COLUMN IF NOT EXISTS raw_payload                       JSONB,
    ADD COLUMN IF NOT EXISTS raw_snapshot_fingerprint          TEXT,
    -- DR6: pointer to the outcome/validation records carrying the detail, so
    -- the assertion row does not have to inline every error.
    ADD COLUMN IF NOT EXISTS processing_error_details          JSONB,
    -- ADR 2026081701 DR2's optional immutable audit reference; owned there,
    -- declared here because DR6 lists it in the cross-family minimum contract.
    ADD COLUMN IF NOT EXISTS normalized_against_contract_revision_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_value_state
    ON kb.semantic_assertions (value_state_term_id) WHERE value_state_term_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_class_identity_state
    ON kb.semantic_assertions (class_identity_state_term_id) WHERE class_identity_state_term_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_mapping_state
    ON kb.semantic_assertions (mapping_resolution_state_term_id) WHERE mapping_resolution_state_term_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_conformance_state
    ON kb.semantic_assertions (conformance_state_term_id) WHERE conformance_state_term_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_conformance_state;
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_mapping_state;
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_class_identity_state;
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_value_state;
ALTER TABLE kb.semantic_assertions
    DROP COLUMN IF EXISTS normalized_against_contract_revision_id,
    DROP COLUMN IF EXISTS processing_error_details,
    DROP COLUMN IF EXISTS raw_snapshot_fingerprint,
    DROP COLUMN IF EXISTS raw_payload,
    DROP COLUMN IF EXISTS conformance_state_term_id,
    DROP COLUMN IF EXISTS value_state_term_id,
    DROP COLUMN IF EXISTS mapping_resolution_state_term_id,
    DROP COLUMN IF EXISTS class_identity_state_term_id;
