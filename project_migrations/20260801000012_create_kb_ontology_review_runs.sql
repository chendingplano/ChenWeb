-- +goose Up
-- A review run records one concrete evaluation of a frozen review scope,
-- pinning the assertion watermark at that moment (spec §12; ADR DR9/DR21
-- pattern reused from kb.ontology_comparison_runs). The scope stays
-- reusable across many runs; each run is a separate, reproducible snapshot
-- of the assertion state it evaluated against.
CREATE TABLE IF NOT EXISTS kb.ontology_review_runs (
    id                   BIGSERIAL PRIMARY KEY,
    review_scope_id      TEXT NOT NULL REFERENCES kb.ontology_review_scopes(review_scope_id) ON DELETE RESTRICT,
    input_record_id      BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    assertion_watermark  TEXT NOT NULL,
    create_time          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ontology_review_runs_scope
    ON kb.ontology_review_runs (review_scope_id, id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_review_runs;
