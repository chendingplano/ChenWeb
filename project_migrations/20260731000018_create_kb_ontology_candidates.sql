-- +goose Up
-- Ontology candidates: the proposal channel for governed content (spec §9.3).
-- Every proposed term/label/mapping/axiom lands here first -- from LLM
-- extraction, governed import, or discovery -- and nothing reaches the content
-- tables (kb.ontology_terms etc.) except through this lifecycle. This is the
-- code-enforced form of the "no LLM activates ontology content" guarantee
-- (ADR 2026072701 DR6, revised 2026-07-31 to DB-native storage).
--
-- fingerprint is a deterministic identity over the normalized proposal, source,
-- and intended module (spec §9.3). UNIQUE(fingerprint) makes reprocessing an
-- identical proposal reuse the candidate instead of creating duplicate review
-- work (spec §16.3 item 1).
CREATE TABLE IF NOT EXISTS kb.ontology_candidates (
    id                  BIGSERIAL PRIMARY KEY,
    candidate_kind      TEXT NOT NULL CHECK (candidate_kind IN (
                            'term', 'label', 'mapping', 'axiom',
                            'profile', 'profile_rule', 'module_change'
                        )),
    proposed_payload    JSONB NOT NULL,
    proposed_module_id  TEXT,
    source_type         TEXT NOT NULL DEFAULT 'manual',
    source_ref          TEXT,
    source_line_spans   JSONB,
    discovery_method    TEXT,
    confidence          DOUBLE PRECISION CHECK (confidence >= 0 AND confidence <= 1),
    fingerprint         TEXT NOT NULL,
    candidate_matches   JSONB,
    status              TEXT NOT NULL DEFAULT 'discovered' CHECK (status IN (
                            'discovered', 'draft', 'in_review', 'approved',
                            'included_in_release', 'rejected', 'deferred'
                        )),
    decision_reason     TEXT,
    dependency_fingerprint TEXT,
    proposed_by         TEXT,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by           TEXT,
    modify_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by           TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_candidates_fingerprint
    ON kb.ontology_candidates (fingerprint);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_candidates_status
    ON kb.ontology_candidates (status);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_candidates_module
    ON kb.ontology_candidates (proposed_module_id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_candidates;
