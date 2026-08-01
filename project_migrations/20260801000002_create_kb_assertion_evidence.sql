-- +goose Up
-- Evidence for a semantic assertion (spec 2026072702 §8.3). Several artifacts
-- may support one assertion and one artifact may support several assertions.
-- evidence_role distinguishes support from contradiction (spec §16.2 item 5:
-- two conflicting source assertions must remain separately queryable with
-- independent evidence).
--
-- actor_kind='human' marks a qualifying human-authored provenance record
-- (spec §10.12): such a record is never removed by the artifact-deletion
-- cascade, so it alone can keep an assertion 'accepted' after every
-- processor-derived evidence row is gone. deleted is a soft-delete flag --
-- deletion is an application-level decision (existing retention policy,
-- cascade from input-record deletion), not a DB-level ON DELETE CASCADE,
-- because the assertion's unsupported/accepted transition depends on which
-- evidence rows remain active, not merely on row existence.
CREATE TABLE IF NOT EXISTS kb.assertion_evidence (
    id                  BIGSERIAL PRIMARY KEY,
    assertion_id        BIGINT NOT NULL REFERENCES kb.semantic_assertions(id),
    input_record_id     BIGINT,
    artifact_type       TEXT NOT NULL,
    artifact_id         TEXT NOT NULL,
    artifact_object_id  TEXT,
    evidence_quote      TEXT,
    source_line_spans   JSONB,
    extraction_run      TEXT,
    model               TEXT,
    prompt_version      TEXT,
    confidence          DOUBLE PRECISION CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    evidence_role       TEXT NOT NULL DEFAULT 'supports' CHECK (evidence_role IN ('supports', 'contradicts')),
    actor_kind          TEXT NOT NULL DEFAULT 'processor' CHECK (actor_kind IN ('processor', 'human')),
    deleted             BOOLEAN NOT NULL DEFAULT FALSE,
    deleted_reason      TEXT,
    deleted_time        TIMESTAMPTZ,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by           TEXT
);

CREATE INDEX IF NOT EXISTS idx_kb_assertion_evidence_assertion
    ON kb.assertion_evidence (assertion_id, deleted);
CREATE INDEX IF NOT EXISTS idx_kb_assertion_evidence_input_record
    ON kb.assertion_evidence (input_record_id) WHERE input_record_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS kb.assertion_evidence;
