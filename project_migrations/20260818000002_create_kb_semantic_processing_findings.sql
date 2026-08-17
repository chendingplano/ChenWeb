-- +goose Up
-- ADR 2026081801 DR4: typed, append-only child findings of one outcome
-- envelope. Modelled as a table rather than a JSONB array on the envelope
-- because DR4 requires per-finding active-row uniqueness, per-finding retry
-- state, and per-finding dependency fingerprints -- a mapping approval must be
-- able to retry only the mapping finding, which a JSON array cannot key.
--
-- The point of the split is that one stage can simultaneously report
-- datatype_mismatch, contract_violation, and source_conflict without
-- duplicating its stage outcome, and DR11's reports can count findings by
-- governed term rather than by envelope disposition.
CREATE TABLE IF NOT EXISTS kb.semantic_processing_findings (
    id                       BIGSERIAL PRIMARY KEY,
    outcome_id               BIGINT NOT NULL REFERENCES kb.semantic_processing_outcomes(id),

    -- Deterministic within the outcome's family-declared decision scope (DR4).
    -- Two findings on different axes of the same stage therefore never
    -- collide, and a replay of the same axis reuses this row.
    finding_key              TEXT NOT NULL,

    -- Governed term references, not enums (DR4): mapping/value/class/
    -- conformance/identity/conflict dimensions and the finding vocabulary are
    -- extensible without a migration.
    dimension_term_id        TEXT NOT NULL,
    finding_term_id          TEXT NOT NULL,
    severity_term_id         TEXT NOT NULL,
    retry_state_term_id      TEXT,

    -- DR4: stable machine-readable identifier for programmatic cases. Human
    -- details do not participate in identity, so rewording `details` must not
    -- change error_code or finding_key.
    error_code               TEXT,
    details                  JSONB,

    dependency_fingerprint   TEXT NOT NULL,
    supersedes_finding_id    BIGINT REFERENCES kb.semantic_processing_findings(id),
    active                   BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_processing_findings_revision
    ON kb.semantic_processing_findings (outcome_id, finding_key, dependency_fingerprint);

-- DR4's named active-finding invariant: "enforced by the partial unique index
-- uq_semantic_processing_findings_active ...; it is not left to
-- application-only checking."
CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_processing_findings_active
    ON kb.semantic_processing_findings (outcome_id, finding_key) WHERE active = true;

CREATE INDEX IF NOT EXISTS idx_semantic_processing_findings_term_active
    ON kb.semantic_processing_findings (finding_term_id) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_semantic_processing_findings_retry_active
    ON kb.semantic_processing_findings (retry_state_term_id) WHERE active = true;

-- +goose StatementBegin
-- DR4: "A deferred constraint trigger rejects commit if an active finding
-- references an inactive outcome."
--
-- This is a constraint trigger rather than an application check because
-- superseding an envelope and deactivating its children happen in one
-- transaction: a crash or a bug between the two statements must not be able to
-- commit an orphaned active finding. DEFERRABLE INITIALLY DEFERRED is what
-- lets the writer deactivate parent and children in either order within the
-- transaction and still be checked once at commit.
CREATE OR REPLACE FUNCTION kb.check_semantic_finding_parent_active() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM kb.semantic_processing_findings f
        JOIN kb.semantic_processing_outcomes o ON o.id = f.outcome_id
        WHERE f.id = NEW.id AND f.active = true AND o.active = false
    ) THEN
        RAISE EXCEPTION
            'active finding % references inactive outcome %', NEW.id, NEW.outcome_id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER trg_semantic_finding_parent_active
    AFTER INSERT OR UPDATE ON kb.semantic_processing_findings
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION kb.check_semantic_finding_parent_active();

-- +goose StatementBegin
-- The mirror direction: deactivating an outcome while a child is still active
-- must also fail. Without this, the check above is trivially bypassed by
-- superseding the parent last.
CREATE OR REPLACE FUNCTION kb.check_semantic_outcome_children_inactive() RETURNS TRIGGER AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM kb.semantic_processing_outcomes o
        JOIN kb.semantic_processing_findings f ON f.outcome_id = o.id
        WHERE o.id = NEW.id AND o.active = false AND f.active = true
    ) THEN
        RAISE EXCEPTION
            'inactive outcome % still has active findings', NEW.id
            USING ERRCODE = 'integrity_constraint_violation';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE CONSTRAINT TRIGGER trg_semantic_outcome_children_inactive
    AFTER UPDATE ON kb.semantic_processing_outcomes
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION kb.check_semantic_outcome_children_inactive();

-- +goose Down
DROP TRIGGER IF EXISTS trg_semantic_outcome_children_inactive ON kb.semantic_processing_outcomes;
DROP TRIGGER IF EXISTS trg_semantic_finding_parent_active ON kb.semantic_processing_findings;
DROP FUNCTION IF EXISTS kb.check_semantic_outcome_children_inactive();
DROP FUNCTION IF EXISTS kb.check_semantic_finding_parent_active();
DROP TABLE IF EXISTS kb.semantic_processing_findings;
