-- +goose Up
-- ADR 2026081701 DR3/DR7: class resolution is a source-backed, append-only
-- decision history. Ambiguity is retained as alternatives rather than being
-- silently reduced to an authoritative class assignment.

CREATE TABLE IF NOT EXISTS kb.ontology_class_resolution_decisions (
    id                      BIGSERIAL PRIMARY KEY,
    source_artifact_type    TEXT NOT NULL,
    source_artifact_id      TEXT NOT NULL,
    source_input_record_id  BIGINT,
    source_assertion_id     BIGINT REFERENCES kb.semantic_assertions (id),
    selected_class_term_id  TEXT REFERENCES kb.ontology_term_headers (term_id),
    identity_state          TEXT NOT NULL CHECK (identity_state IN (
                                'resolved_existing', 'provisional_new',
                                'ambiguous_candidates', 'candidate_evidence_conflict',
                                'rejected'
                            )),
    method                  TEXT NOT NULL,
    confidence              DOUBLE PRECISION CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    evidence                JSONB NOT NULL DEFAULT '{}'::jsonb,
    supersedes_decision_id  BIGINT REFERENCES kb.ontology_class_resolution_decisions (id),
    create_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by               TEXT
);

CREATE INDEX IF NOT EXISTS idx_kb_class_resolution_decisions_source
    ON kb.ontology_class_resolution_decisions (source_artifact_type, source_artifact_id, source_input_record_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_kb_class_resolution_decisions_selected_class
    ON kb.ontology_class_resolution_decisions (selected_class_term_id) WHERE selected_class_term_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS kb.ontology_class_resolution_alternatives (
    id                      BIGSERIAL PRIMARY KEY,
    decision_id             BIGINT NOT NULL REFERENCES kb.ontology_class_resolution_decisions (id),
    candidate_class_term_id TEXT REFERENCES kb.ontology_term_headers (term_id),
    candidate_key           TEXT NOT NULL DEFAULT '',
    rank                    INT NOT NULL CHECK (rank > 0),
    score                   DOUBLE PRECISION CHECK (score IS NULL OR (score >= 0 AND score <= 1)),
    evidence                JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (decision_id, rank),
    CHECK (candidate_class_term_id IS NOT NULL OR BTRIM(candidate_key) <> '')
);

CREATE INDEX IF NOT EXISTS idx_kb_class_resolution_alternatives_candidate
    ON kb.ontology_class_resolution_alternatives (candidate_class_term_id) WHERE candidate_class_term_id IS NOT NULL;

CREATE OR REPLACE FUNCTION kb.reject_class_resolution_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'class resolution history is append-only';
END;
$$;

CREATE TRIGGER kb_class_resolution_decisions_immutable
BEFORE UPDATE OR DELETE ON kb.ontology_class_resolution_decisions
FOR EACH ROW EXECUTE FUNCTION kb.reject_class_resolution_mutation();

CREATE TRIGGER kb_class_resolution_alternatives_immutable
BEFORE UPDATE OR DELETE ON kb.ontology_class_resolution_alternatives
FOR EACH ROW EXECUTE FUNCTION kb.reject_class_resolution_mutation();

-- +goose Down
DROP TRIGGER IF EXISTS kb_class_resolution_alternatives_immutable ON kb.ontology_class_resolution_alternatives;
DROP TRIGGER IF EXISTS kb_class_resolution_decisions_immutable ON kb.ontology_class_resolution_decisions;
DROP FUNCTION IF EXISTS kb.reject_class_resolution_mutation();
DROP TABLE IF EXISTS kb.ontology_class_resolution_alternatives;
DROP TABLE IF EXISTS kb.ontology_class_resolution_decisions;
