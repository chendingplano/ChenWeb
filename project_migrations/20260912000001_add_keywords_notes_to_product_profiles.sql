-- +goose Up
-- Product Review self-service intake (openspec change product-review-intake),
-- capability product-review-intake. Captures the intake form's `keywords` and
-- `notes` fields on the profile; also indexes the normalized name so the
-- intake flow can look up an existing profile for the same product before
-- creating a duplicate.

ALTER TABLE kb.product_profiles
    ADD COLUMN IF NOT EXISTS keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS notes    TEXT  NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_kb_product_profiles_tenant_name
    ON kb.product_profiles (tenant_id, LOWER(TRIM(name)));

-- +goose Down
DROP INDEX IF EXISTS idx_kb_product_profiles_tenant_name;

ALTER TABLE kb.product_profiles
    DROP COLUMN IF EXISTS notes,
    DROP COLUMN IF EXISTS keywords;
