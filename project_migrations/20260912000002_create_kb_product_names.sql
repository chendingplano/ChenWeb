-- +goose Up
CREATE TABLE IF NOT EXISTS kb.product_names (
    id               BIGSERIAL PRIMARY KEY,
    seq_no           INTEGER NOT NULL,
    sub_catalog      TEXT NOT NULL DEFAULT '',
    category_l1      TEXT NOT NULL DEFAULT '',
    category_l2      TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    intended_use     TEXT NOT NULL DEFAULT '',
    product_name     TEXT NOT NULL,
    product_name_en  TEXT NOT NULL DEFAULT '',
    regulatory_class TEXT NOT NULL DEFAULT '',
    aliases          JSONB NOT NULL DEFAULT '[]'::jsonb,
    keywords         JSONB NOT NULL DEFAULT '{}'::jsonb,
    source           TEXT NOT NULL,
    notes            TEXT NOT NULL DEFAULT '',
    extra_info       JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, seq_no, product_name)
);

CREATE INDEX IF NOT EXISTS idx_kb_product_names_source ON kb.product_names (source);
CREATE INDEX IF NOT EXISTS idx_kb_product_names_product_name ON kb.product_names (product_name);
CREATE INDEX IF NOT EXISTS idx_kb_product_names_category_l2 ON kb.product_names (category_l2);

-- +goose Down
DROP TABLE IF EXISTS kb.product_names;
