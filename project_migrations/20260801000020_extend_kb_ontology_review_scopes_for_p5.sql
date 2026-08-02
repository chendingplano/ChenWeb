-- +goose Up
-- P5 (spec 2026080102 section 6) extends review scopes with deterministic
-- selection provenance. All five columns are nullable so existing explicit
-- P4 scopes and new explicit-mode inserts remain valid: explicit selection
-- never populates them. A deterministic scope freezes the knowledge store
-- identity, a stable selection attempt id, the selection status, and the fact
-- and selection snapshots that make the decision reproducible.
ALTER TABLE kb.ontology_review_scopes
    ADD COLUMN IF NOT EXISTS knowledge_store_id    BIGINT,
    ADD COLUMN IF NOT EXISTS selection_attempt_id  TEXT,
    ADD COLUMN IF NOT EXISTS selection_status      TEXT,
    ADD COLUMN IF NOT EXISTS fact_snapshot         JSONB,
    ADD COLUMN IF NOT EXISTS selection_snapshot    JSONB;

-- +goose Down
ALTER TABLE kb.ontology_review_scopes
    DROP COLUMN IF EXISTS selection_snapshot,
    DROP COLUMN IF EXISTS fact_snapshot,
    DROP COLUMN IF EXISTS selection_status,
    DROP COLUMN IF EXISTS selection_attempt_id,
    DROP COLUMN IF EXISTS knowledge_store_id;
