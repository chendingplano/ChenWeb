-- +goose Up
-- ADR 2026081801 DR4: the generic, append-only record of what each semantic
-- stage managed to determine about one artifact. This table is what makes the
-- lossless invariant auditable -- before it, "normalize_assertions deferred
-- this metric" existed only as log text and a decision-candidate status, so a
-- consumer could not distinguish "not processed", "processed and clean", and
-- "processed with an unresolved mapping".
--
-- One row per (input record, artifact, stage) attempt, INCLUDING success:
-- DR4 requires "every required semantic-stage attempt, including success, has
-- exactly one outcome envelope or reuses an identical existing envelope", so
-- the completeness projection can tell a missing attempt from a clean one.
--
-- Append-only except for two things: last_seen (idempotent replay) and the
-- transactionally maintained active projection. Everything else supersedes.
CREATE TABLE IF NOT EXISTS kb.semantic_processing_outcomes (
    id                       BIGSERIAL PRIMARY KEY,

    -- Deterministic hash/encoding of (input_record_id, artifact_type,
    -- artifact_id, stage_term_id) -- DR4. It is the stable scope key: every
    -- revision of this attempt shares it, and the active projection is unique
    -- on it alone.
    outcome_key              TEXT NOT NULL,

    input_record_id          BIGINT,
    artifact_type            TEXT NOT NULL,
    -- Nullable ONLY for a failed invocation-level outcome with no identifiable
    -- artifact (DR3 source_or_output_unrecoverable). chk_..._artifact_required
    -- below is what actually enforces that; the column is nullable so the
    -- unidentified-invocation row can exist at all.
    artifact_id              TEXT,
    assertion_id             BIGINT REFERENCES kb.semantic_assertions(id),

    stage_term_id            TEXT NOT NULL,
    -- Governed disposition term (semantic:normalized | raw_preserved |
    -- not_applicable | no_result). Deliberately a term reference, not a CHECK
    -- constraint: DR4 makes disposition/dimension/finding vocabularies
    -- extensible ontology terms so a new one needs no migration.
    disposition_term_id      TEXT,
    -- DR3: execution status is canonically binary. A third value is never
    -- persisted; "completed with findings" is derived from finding_count > 0.
    execution_status         TEXT NOT NULL DEFAULT 'completed'
                                 CHECK (execution_status IN ('completed', 'failed')),
    -- DR3 outcome category, kept separate from execution_status so a reader
    -- can tell WHY a run failed without re-deriving it from findings.
    outcome_category         TEXT NOT NULL DEFAULT 'semantic_success'
                                 CHECK (outcome_category IN (
                                     'system_failure', 'source_or_output_unrecoverable',
                                     'semantic_finding', 'semantic_success')),

    -- Denormalized from the current child finding set inside the same
    -- transaction that writes that set (DR4). Reports never scan children.
    finding_count            INT NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
    highest_severity_term_id TEXT,

    -- DR2: populated ONLY when no identified artifact row can preserve the
    -- content. An artifact-family row is always authoritative when one exists.
    raw_fragment             TEXT,

    -- DR10: canonical aggregate of this stage's dependency and its current
    -- children's dependencies, so a child-level change necessarily changes the
    -- envelope fingerprint.
    dependency_fingerprint   TEXT NOT NULL,
    -- DR4: identifies the exact raw occurrence revision this attempt read.
    input_fingerprint        TEXT NOT NULL,

    processor_name           TEXT,
    processor_version        TEXT,
    extraction_run           TEXT,
    model                    TEXT,
    prompt_version           TEXT,

    supersedes_outcome_id    BIGINT REFERENCES kb.semantic_processing_outcomes(id),
    active                   BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                TEXT,

    -- DR4: "A completed semantic-stage outcome always has a non-null
    -- artifact_id. A null artifact_id is permitted only for a failed
    -- invocation-level outcome under DR3's source_or_output_unrecoverable
    -- category; it never creates an unresolved semantic occurrence."
    CONSTRAINT chk_semantic_processing_outcomes_artifact_required
        CHECK (artifact_id IS NOT NULL
               OR (execution_status = 'failed'
                   AND outcome_category = 'source_or_output_unrecoverable')),

    -- DR3: a completed run's finding summary is mandatory, which means the
    -- summary columns must agree with each other. A zero count cannot carry a
    -- severity and a positive count must.
    CONSTRAINT chk_semantic_processing_outcomes_severity_summary
        CHECK ((finding_count = 0 AND highest_severity_term_id IS NULL)
               OR (finding_count > 0 AND highest_severity_term_id IS NOT NULL))
);

-- DR4 base uniqueness: one row per (scope, input revision, dependency set).
-- Two writers racing on identical inputs conflict here rather than both
-- inserting.
CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_processing_outcomes_revision
    ON kb.semantic_processing_outcomes (outcome_key, input_fingerprint, dependency_fingerprint);

-- DR4's named active-row invariant. This enforces AT MOST ONE active outcome
-- per stable source/stage scope -- never existence. Existence is enforced by
-- the atomic attempt transaction and the completeness projection (DR5/DR13).
CREATE UNIQUE INDEX IF NOT EXISTS uq_semantic_processing_outcomes_active
    ON kb.semantic_processing_outcomes (outcome_key) WHERE active = true;

CREATE INDEX IF NOT EXISTS idx_semantic_processing_outcomes_artifact
    ON kb.semantic_processing_outcomes (artifact_type, artifact_id, input_record_id);
CREATE INDEX IF NOT EXISTS idx_semantic_processing_outcomes_stage_active
    ON kb.semantic_processing_outcomes (stage_term_id) WHERE active = true;
CREATE INDEX IF NOT EXISTS idx_semantic_processing_outcomes_assertion
    ON kb.semantic_processing_outcomes (assertion_id) WHERE assertion_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_semantic_processing_outcomes_record
    ON kb.semantic_processing_outcomes (input_record_id) WHERE input_record_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS kb.semantic_processing_outcomes;
