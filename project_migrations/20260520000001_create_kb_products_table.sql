-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.products (
    id                  BIGSERIAL PRIMARY KEY,
    input_record_id     BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    product_rel_id      TEXT        NOT NULL,
    product_name        TEXT        NOT NULL DEFAULT '',
    product_name_en     TEXT        NOT NULL DEFAULT '',
    canonical_name      TEXT        NOT NULL DEFAULT '',
    canonical_name_en   TEXT        NOT NULL DEFAULT '',
    product_type        TEXT        NOT NULL DEFAULT '',
    relation_type       TEXT        NOT NULL DEFAULT '',
    relation_summary    TEXT        NOT NULL DEFAULT '',
    relation_summary_en TEXT        NOT NULL DEFAULT '',
    evidence_quote      TEXT        NOT NULL DEFAULT '',
    evidence_lines      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    obligation_level    TEXT        NOT NULL DEFAULT '',
    requirement_text    TEXT        NOT NULL DEFAULT '',
    requirement_text_en TEXT        NOT NULL DEFAULT '',
    conditions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    exceptions          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    parameters          JSONB       NOT NULL DEFAULT '[]'::jsonb,
    related_products    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    responsible_actor   TEXT        NOT NULL DEFAULT '',
    confidence          DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_reason   TEXT        NOT NULL DEFAULT '',
    category_paths      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    category_paths_en   JSONB       NOT NULL DEFAULT '[]'::jsonb,
    status              TEXT        NOT NULL DEFAULT 'active',
    model_name          TEXT        NOT NULL DEFAULT '',
    prompt_name         TEXT        NOT NULL DEFAULT '',
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    public_info         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    private_info        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    notes               TEXT        NOT NULL DEFAULT '',
    error_msg           TEXT        NOT NULL DEFAULT '',
    CONSTRAINT uq_kb_products_input_product_rel UNIQUE (input_record_id, product_rel_id)
);
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_products_input_record_id
    ON kb.products(input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_products_status
    ON kb.products(status);

CREATE INDEX IF NOT EXISTS idx_kb_products_product_type
    ON kb.products(product_type);

CREATE INDEX IF NOT EXISTS idx_kb_products_category_paths_gin
    ON kb.products USING GIN (category_paths);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_products_category_paths_gin;
DROP INDEX IF EXISTS kb.idx_kb_products_product_type;
DROP INDEX IF EXISTS kb.idx_kb_products_status;
DROP INDEX IF EXISTS kb.idx_kb_products_input_record_id;
DROP TABLE IF EXISTS kb.products;
