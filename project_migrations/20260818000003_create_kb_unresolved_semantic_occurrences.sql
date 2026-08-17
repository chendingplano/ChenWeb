-- +goose Up
-- ADR 2026081801 DR13: the generic option-3 fallback. When a family cannot yet
-- create a semantic assertion, the artifact still must not vanish -- it gets a
-- durable, discoverable, materializable occurrence row instead.
--
-- artifact_id is NOT NULL by design (DR13: "required: fallback applies only to
-- an identified artifact"). If no artifact can be identified, DR3 classifies
-- the attempt as source_or_output_unrecoverable and the invocation/raw output
-- is retained -- but no occurrence row is fabricated. That asymmetry is the
-- whole point: an unresolved occurrence is a promise that a specific artifact
-- will be materialized later, and a promise about nothing is not preservable.
CREATE TABLE IF NOT EXISTS kb.unresolved_semantic_occurrences (
    id                       BIGSERIAL PRIMARY KEY,

    -- Deterministic for the source scope and family (DR13). The family-declared
    -- source scope is already part of the key, which is why the active index
    -- below needs only occurrence_key.
    occurrence_key           TEXT NOT NULL,

    input_record_id          BIGINT,
    artifact_type            TEXT NOT NULL,
    artifact_id              TEXT NOT NULL,

    source_revision          TEXT,
    raw_payload              JSONB,
    provenance               JSONB,

    materialization_state    TEXT NOT NULL DEFAULT 'pending'
                                 CHECK (materialization_state IN
                                     ('pending', 'claimed', 'materializing', 'materialized', 'abandoned')),
    resulting_assertion_id   BIGINT REFERENCES kb.semantic_assertions(id),
    current_outcome_id       BIGINT REFERENCES kb.semantic_processing_outcomes(id),
    supersedes_occurrence_id BIGINT REFERENCES kb.unresolved_semantic_occurrences(id),

    input_fingerprint        TEXT NOT NULL,
    dependency_fingerprint   TEXT NOT NULL,
    active                   BOOLEAN NOT NULL DEFAULT TRUE,

    -- DR13: workers claim materialization with row locking and an expiring
    -- lease token, so a crashed worker's occurrence becomes claimable again
    -- without leaving duplicate active state behind.
    lease_token              TEXT,
    lease_expires_at         TIMESTAMPTZ,
    -- Deterministic idempotency key for the recoverable-saga path (DR13), so a
    -- resumed saga recognizes its own partial work.
    saga_idempotency_key     TEXT,
    saga_completed_at        TIMESTAMPTZ,

    last_seen                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                TEXT,

    -- DR13: "A crash cannot leave a materialized assertion paired with an
    -- active unresolved occurrence." Once materialized, the row is superseded
    -- (inactive) and carries its resulting assertion.
    CONSTRAINT chk_unresolved_occurrence_materialized_inactive
        CHECK (materialization_state <> 'materialized'
               OR (active = false AND resulting_assertion_id IS NOT NULL)),
    CONSTRAINT chk_unresolved_occurrence_lease_pair
        CHECK ((lease_token IS NULL AND lease_expires_at IS NULL)
               OR (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_unresolved_semantic_occurrences_revision
    ON kb.unresolved_semantic_occurrences (occurrence_key, input_fingerprint, dependency_fingerprint);

-- DR13's named active-row invariant.
CREATE UNIQUE INDEX IF NOT EXISTS uq_unresolved_semantic_occurrences_active
    ON kb.unresolved_semantic_occurrences (occurrence_key) WHERE active = true;

CREATE INDEX IF NOT EXISTS idx_unresolved_semantic_occurrences_artifact
    ON kb.unresolved_semantic_occurrences (artifact_type, artifact_id, input_record_id);
-- Claim scan: pending/expired-lease rows in family order.
CREATE INDEX IF NOT EXISTS idx_unresolved_semantic_occurrences_claimable
    ON kb.unresolved_semantic_occurrences (artifact_type, materialization_state, lease_expires_at)
    WHERE active = true;

-- +goose Down
DROP TABLE IF EXISTS kb.unresolved_semantic_occurrences;
