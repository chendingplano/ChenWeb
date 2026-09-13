-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE kb.product_names
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'proposed'
        CHECK (status IN ('proposed', 'approved', 'rejected'));

-- Every row imported before this migration (the NMPA classification catalog)
-- is authoritative reference data, not an LLM proposal.
UPDATE kb.product_names SET status = 'approved' WHERE status = 'proposed';

-- Race-safe identity for auto-created rows (extract_products, D11-style
-- auto-first): scoped to status='proposed' so it does not conflict with the
-- catalog import's legitimate cross-category product_name repeats.
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_product_names_proposed_name
    ON kb.product_names (product_name) WHERE status = 'proposed';

CREATE INDEX IF NOT EXISTS idx_kb_product_names_trgm
    ON kb.product_names USING gin (product_name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_kb_product_names_en_trgm
    ON kb.product_names USING gin (product_name_en gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_product_names_en_trgm;
DROP INDEX IF EXISTS idx_kb_product_names_trgm;
DROP INDEX IF EXISTS idx_kb_product_names_proposed_name;
ALTER TABLE kb.product_names DROP COLUMN IF EXISTS status;
