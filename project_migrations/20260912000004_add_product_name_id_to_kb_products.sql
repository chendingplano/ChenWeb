-- +goose Up
ALTER TABLE kb.products
    ADD COLUMN IF NOT EXISTS product_name_id BIGINT REFERENCES kb.product_names(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_kb_products_product_name_id
    ON kb.products (product_name_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_products_product_name_id;
ALTER TABLE kb.products DROP COLUMN IF EXISTS product_name_id;
