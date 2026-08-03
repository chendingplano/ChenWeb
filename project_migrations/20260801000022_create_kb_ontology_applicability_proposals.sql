-- +goose Up
-- P5 H1: governed applicability proposal lifecycle (spec 2026080102 section 8).
-- Each proposal carries a predicate document from an ontology module release.
-- Proposals transition draft -> in_review -> approved -> included_in_release.
-- Only approved proposals are materialized into draft pipeline-policy content
-- when the source module release is imported or activated (H2).

CREATE TABLE IF NOT EXISTS kb.ontology_applicability_proposals (
    id              BIGSERIAL PRIMARY KEY,
    module_id       TEXT NOT NULL,
    release_id      BIGINT NOT NULL,
    proposal_kind   TEXT NOT NULL DEFAULT 'routing',
    predicate       JSONB NOT NULL,
    predicate_checksum TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft',
    source_release_checksum TEXT,
    approved_by     TEXT,
    approved_at     TIMESTAMPTZ,
    included_in_release_id BIGINT,
    created_by      TEXT,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_applicability_proposals_release
    ON kb.ontology_applicability_proposals (release_id);
CREATE INDEX IF NOT EXISTS idx_applicability_proposals_status
    ON kb.ontology_applicability_proposals (status)
    WHERE status IN ('draft', 'in_review', 'approved');

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_applicability_proposals;
