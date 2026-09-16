-- +goose Up
ALTER TABLE kb.product_profiles
    ADD COLUMN IF NOT EXISTS name_cn TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS name_en TEXT NOT NULL DEFAULT '';

UPDATE kb.product_profiles p
SET name_cn = COALESCE(NULLIF(TRIM(p.name_cn), ''), p.name),
    name_en = COALESCE(NULLIF(TRIM(p.name_en), ''), (
        SELECT pn.product_name_en
        FROM kb.product_names pn
        WHERE LOWER(TRIM(pn.product_name)) = LOWER(TRIM(p.name))
        ORDER BY pn.status = 'approved' DESC, pn.id
        LIMIT 1
    ), '');

-- +goose Down
ALTER TABLE kb.product_profiles
    DROP COLUMN IF EXISTS name_cn,
    DROP COLUMN IF EXISTS name_en;
