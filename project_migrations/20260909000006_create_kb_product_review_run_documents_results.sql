-- +goose Up
-- Product Metric Reviewer (openspec change product-metric-reviewer), capability
-- product-metric-review-runs. Per-run output:
--
--   product_review_run_documents  the scored document scope set, readable
--                                 independently of the results, with the node
--                                 ids and paths that matched each document and
--                                 the doc kind used for the standards boost.
--
--   product_review_results        one row per (artifact_type, artifact_id) --
--                                 the kb.search_artifacts key -- carrying full
--                                 provenance: source row, document, matched
--                                 node, tier, paths, score, inclusion reason,
--                                 line spans. node_id is a loose reference (no
--                                 FK): document_scope results have none, and a
--                                 later profile edit must not disturb a
--                                 completed run's rows.

CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.product_review_run_documents (
    id                 BIGSERIAL   PRIMARY KEY,
    run_id             BIGINT      NOT NULL REFERENCES kb.product_review_runs(id) ON DELETE CASCADE,
    input_record_id    BIGINT      NOT NULL,
    fused_score        DOUBLE PRECISION NOT NULL DEFAULT 0,
    matching_node_ids  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    matching_paths     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    match_reasons      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    doc_kind           TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_product_review_run_documents_run_record UNIQUE (run_id, input_record_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_product_review_run_documents_run
    ON kb.product_review_run_documents (run_id, fused_score DESC);

CREATE TABLE IF NOT EXISTS kb.product_review_results (
    id                 BIGSERIAL   PRIMARY KEY,
    run_id             BIGINT      NOT NULL REFERENCES kb.product_review_runs(id) ON DELETE CASCADE,
    artifact_type      TEXT        NOT NULL,
    artifact_id        TEXT        NOT NULL,
    source_row_id      BIGINT,
    input_record_id    BIGINT      NOT NULL,
    node_id            BIGINT,
    tier               TEXT        NOT NULL
                                   CHECK (tier IN ('direct', 'part', 'aspect', 'document_scope')),
    score              DOUBLE PRECISION NOT NULL DEFAULT 0,
    paths              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    inclusion_reason   TEXT        NOT NULL DEFAULT '',
    source_line_spans  JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_product_review_results_run_artifact UNIQUE (run_id, artifact_type, artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_product_review_results_run_score
    ON kb.product_review_results (run_id, score DESC);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_results_run_node
    ON kb.product_review_results (run_id, node_id);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_results_run_tier
    ON kb.product_review_results (run_id, tier);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_results_run_document
    ON kb.product_review_results (run_id, input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_results_run_type
    ON kb.product_review_results (run_id, artifact_type);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_results_paths
    ON kb.product_review_results USING GIN (paths);

-- +goose Down
DROP TABLE IF EXISTS kb.product_review_results;
DROP TABLE IF EXISTS kb.product_review_run_documents;
