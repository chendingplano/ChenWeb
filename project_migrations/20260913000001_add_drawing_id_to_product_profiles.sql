-- +goose Up
-- Product Review 3D drawing + resizable layout (openspec change
-- product-review-3d-resizable-layout), capability product-review-results-layout.
-- Caches the kept product-drawings row generated for a review profile so the
-- Results page doesn't regenerate an image on every visit.

ALTER TABLE kb.product_profiles
    ADD COLUMN IF NOT EXISTS drawing_id BIGINT REFERENCES kb.product_drawings(id);

-- +goose Down
ALTER TABLE kb.product_profiles
    DROP COLUMN IF EXISTS drawing_id;
