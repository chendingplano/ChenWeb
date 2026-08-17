-- +goose Up
-- ADR 2026081801 DR10: semantic findings leave the failed-processor queue.
-- The failed-processor queue re-runs a processor; that is exactly the wrong
-- response to "no approved mapping exists yet", because re-running an
-- unchanged dependency cannot change the result and only repeats the alert
-- (ADR section 4.6). This queue is keyed on the DEPENDENCY the outcome is
-- waiting for instead.
--
-- finding_id is nullable: a null means "retry the whole stage" rather than one
-- finding. That is why the unique index below, not a table constraint, is what
-- makes concurrent enqueue idempotent -- NULLS NOT DISTINCT makes two
-- whole-stage enqueues for the same target collide as intended.
CREATE TABLE IF NOT EXISTS kb.semantic_retry_queue (
    id                           BIGSERIAL PRIMARY KEY,
    outcome_id                   BIGINT NOT NULL REFERENCES kb.semantic_processing_outcomes(id),
    finding_id                   BIGINT REFERENCES kb.semantic_processing_findings(id),

    -- The fingerprint the job is waiting to see. A job whose target no longer
    -- matches current state records 'stale' and performs no semantic writes.
    target_dependency_fingerprint TEXT NOT NULL,
    -- The source revision captured at enqueue, so staleness can be detected on
    -- the input axis as well as the dependency axis.
    source_input_fingerprint      TEXT,

    state                        TEXT NOT NULL DEFAULT 'pending'
                                     CHECK (state IN ('pending', 'claimed', 'done', 'stale', 'failed')),
    attempts                     INT NOT NULL DEFAULT 0,
    lease_token                  TEXT,
    lease_expires_at             TIMESTAMPTZ,
    last_error                   TEXT,

    create_time                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                    TEXT,
    modify_time                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_semantic_retry_queue_lease_pair
        CHECK ((lease_token IS NULL AND lease_expires_at IS NULL)
               OR (lease_token IS NOT NULL AND lease_expires_at IS NOT NULL))
);

-- DR10: "The retry queue is unique on (outcome_id, finding_id,
-- target_dependency_fingerprint)". NULLS NOT DISTINCT (PostgreSQL 15+) is
-- required for the whole-stage case: without it, every whole-stage enqueue
-- would insert a new row because NULL <> NULL.
CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_retry_queue_target
    ON kb.semantic_retry_queue (outcome_id, finding_id, target_dependency_fingerprint)
    NULLS NOT DISTINCT;

-- Claim scan for FOR UPDATE SKIP LOCKED workers.
CREATE INDEX IF NOT EXISTS idx_semantic_retry_queue_claimable
    ON kb.semantic_retry_queue (state, lease_expires_at, id);
CREATE INDEX IF NOT EXISTS idx_semantic_retry_queue_outcome
    ON kb.semantic_retry_queue (outcome_id);

-- +goose Down
DROP TABLE IF EXISTS kb.semantic_retry_queue;
