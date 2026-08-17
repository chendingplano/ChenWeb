-- +goose Up
-- ADR 2026081801 DR13: the runtime compliance registry. "Activation is refused
-- when the registered adapter has not passed the current suite."
--
-- This is a table rather than a deploy-time checklist because the ADR makes
-- activation-refusal normative: the writer gate reads this row at startup, so
-- an adapter that has not passed the CURRENT suite version cannot be switched
-- on by config alone.
CREATE TABLE IF NOT EXISTS kb.semantic_adapter_compliance (
    id                        BIGSERIAL PRIMARY KEY,
    artifact_type             TEXT NOT NULL,
    adapter_name              TEXT NOT NULL,
    adapter_version           TEXT NOT NULL,

    -- 'legacy'  -- the pre-lossless writer is in charge.
    -- 'shadow'  -- the adapter computes intended writes but persists nothing
    --              consumer-visible (Phase 1).
    -- 'fallback'-- the family uses the generic unresolved-occurrence path.
    -- 'lossless'-- the family's lossless writer is active.
    writer_mode               TEXT NOT NULL DEFAULT 'legacy'
                                  CHECK (writer_mode IN ('legacy', 'shadow', 'fallback', 'lossless')),

    conformance_suite_version TEXT,
    last_verified_result      TEXT CHECK (last_verified_result IS NULL
                                  OR last_verified_result IN ('passed', 'failed', 'not_run')),
    last_verified_time        TIMESTAMPTZ,
    last_verified_details     JSONB,

    create_time               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                 TEXT,
    modify_time               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by                 TEXT,

    -- A 'lossless' writer mode without a recorded pass is exactly the state
    -- DR13 forbids, so the database refuses to store it rather than relying on
    -- the activation check alone.
    CONSTRAINT chk_semantic_adapter_compliance_lossless_requires_pass
        CHECK (writer_mode <> 'lossless'
               OR (last_verified_result = 'passed' AND conformance_suite_version IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_adapter_compliance_family
    ON kb.semantic_adapter_compliance (artifact_type);

-- +goose Down
DROP TABLE IF EXISTS kb.semantic_adapter_compliance;
