-- +goose Up
-- Qualified semantic assertions (ADR 2026072901 DR9; spec 2026072702 §8.3).
-- A typed-reference pair, not a polymorphic FK and not opaque text:
-- subject_ref_kind/subject_ref_id is the generic pair, subject_object_id is a
-- fast-path column populated only when subject_ref_kind='object_node', giving
-- referential integrity and an index for the dominant "all assertions about
-- this referent" query shape. object_ref_* mirrors this; object_literal
-- covers the typed-literal alternative to a reference (spec §8.3).
--
-- logical_identity_key groups revisions of the same claim (spec §10.7); the
-- unique (logical_identity_key, revision) pair mirrors the kb.ontology_terms
-- (term_id, version) pattern from P2 -- the latest revision is current state,
-- and a decision-relevant change creates a new revision rather than mutating
-- the prior one (spec §16.3 item 2).
--
-- status follows the spec §9.3 OPERATIONAL semantic-decision state machine
-- (candidate -> in_review -> accepted/rejected/deferred; accepted ->
-- superseded | unsupported; unsupported -> accepted) -- a distinct machine
-- from the P2 governed-content lifecycle on kb.ontology_terms/candidates.
--
-- raw_text is never dropped even when a value fails to parse into the
-- structured columns (DR8: normalize_assertions must not fabricate a value).
CREATE TABLE IF NOT EXISTS kb.semantic_assertions (
    id                      BIGSERIAL PRIMARY KEY,
    logical_identity_key    TEXT NOT NULL,
    revision                INT NOT NULL DEFAULT 1,

    subject_ref_kind        TEXT NOT NULL CHECK (subject_ref_kind IN
                                ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal')),
    subject_ref_id          TEXT NOT NULL,
    subject_object_id       TEXT REFERENCES kb.object_nodes(object_id),

    predicate_term_id       TEXT NOT NULL,

    object_ref_kind          TEXT CHECK (object_ref_kind IS NULL OR object_ref_kind IN
                                ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal')),
    object_ref_id            TEXT,
    object_object_id         TEXT REFERENCES kb.object_nodes(object_id),
    object_literal           JSONB,

    assertion_kind_term_id   TEXT,
    polarity                 TEXT NOT NULL DEFAULT 'positive' CHECK (polarity IN ('positive', 'negative')),
    modality                 TEXT,
    qualifiers               JSONB,
    confidence                DOUBLE PRECISION CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),

    value_form                TEXT,
    numeric_value              DOUBLE PRECISION,
    lower_value                 DOUBLE PRECISION,
    upper_value                  DOUBLE PRECISION,
    lower_inclusive              BOOLEAN,
    upper_inclusive              BOOLEAN,
    comparator                   TEXT,
    unit_term_id                 TEXT,
    quantity_kind_term_id        TEXT,
    raw_text                     TEXT,

    status                   TEXT NOT NULL DEFAULT 'candidate' CHECK (status IN (
                                'candidate', 'in_review', 'accepted', 'rejected',
                                'deferred', 'superseded', 'unsupported'
                              )),
    decision_reason           TEXT,
    dependency_fingerprint    TEXT,
    superseded_by              BIGINT REFERENCES kb.semantic_assertions(id),

    valid_time_start           TIMESTAMPTZ,
    valid_time_end              TIMESTAMPTZ,
    transaction_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    create_time               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by                 TEXT,
    modify_time                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by                  TEXT,

    CONSTRAINT chk_semantic_assertions_object_ref_or_literal
        CHECK (object_ref_id IS NOT NULL OR object_literal IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_semantic_assertions_identity_revision
    ON kb.semantic_assertions (logical_identity_key, revision);
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_status
    ON kb.semantic_assertions (status);
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_subject_object
    ON kb.semantic_assertions (subject_object_id) WHERE subject_object_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_predicate
    ON kb.semantic_assertions (predicate_term_id);
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_identity_key
    ON kb.semantic_assertions (logical_identity_key);

-- +goose Down
DROP TABLE IF EXISTS kb.semantic_assertions;
