-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF to_regclass('kb.inventory_categories') IS NOT NULL THEN
        INSERT INTO kb.artifact_categories (
            category_key,
            category_type,
            status,
            canonical_of,
            display_names,
            required_attrs,
            specs,
            plausible_ranges,
            embedding,
            seen_count,
            first_seen_at,
            last_seen_at,
            create_time,
            modify_time,
            search_document
        )
        SELECT
            category_key,
            'inventory',
            status,
            canonical_of,
            display_names,
            required_attrs,
            specs,
            plausible_ranges,
            embedding,
            seen_count,
            first_seen_at,
            last_seen_at,
            create_time,
            modify_time,
            category_key
        FROM kb.inventory_categories
        ON CONFLICT (category_key) DO UPDATE SET
            category_type = 'inventory',
            status = EXCLUDED.status,
            canonical_of = EXCLUDED.canonical_of,
            display_names = EXCLUDED.display_names,
            required_attrs = EXCLUDED.required_attrs,
            specs = EXCLUDED.specs,
            plausible_ranges = EXCLUDED.plausible_ranges,
            embedding = EXCLUDED.embedding,
            seen_count = GREATEST(kb.artifact_categories.seen_count, EXCLUDED.seen_count),
            last_seen_at = GREATEST(kb.artifact_categories.last_seen_at, EXCLUDED.last_seen_at),
            modify_time = NOW(),
            search_document = CASE
                WHEN COALESCE(kb.artifact_categories.search_document, '') = '' THEN EXCLUDED.search_document
                ELSE kb.artifact_categories.search_document
            END;
    END IF;

    UPDATE kb.artifact_categories
    SET category_id = DEFAULT
    WHERE category_id IS NULL;
END $$;

DROP INDEX IF EXISTS idx_kb_inventory_categories_canonical_of;
DROP INDEX IF EXISTS idx_kb_inventory_categories_seen_count;
DROP INDEX IF EXISTS idx_kb_inventory_categories_status;
DROP TABLE IF EXISTS kb.inventory_categories;
DROP FUNCTION IF EXISTS kb.refresh_inventory_category_modify_time();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.inventory_categories (
    category_key      TEXT        PRIMARY KEY,
    status            TEXT        NOT NULL DEFAULT 'pending_review',
    canonical_of      TEXT,
    display_names     JSONB       NOT NULL DEFAULT '[]'::jsonb,
    required_attrs    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    specs             JSONB       NOT NULL DEFAULT '{}'::jsonb,
    plausible_ranges  JSONB       NOT NULL DEFAULT '{}'::jsonb,
    embedding         JSONB       NOT NULL DEFAULT '[]'::jsonb,
    seen_count        BIGINT      NOT NULL DEFAULT 0,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT inventory_categories_status_check
        CHECK (status IN ('pending_review', 'approved', 'rejected', 'merged'))
);

INSERT INTO kb.inventory_categories (
    category_key,
    status,
    canonical_of,
    display_names,
    required_attrs,
    specs,
    plausible_ranges,
    embedding,
    seen_count,
    first_seen_at,
    last_seen_at,
    create_time,
    modify_time
)
SELECT
    category_key,
    status,
    canonical_of,
    display_names,
    required_attrs,
    specs,
    plausible_ranges,
    embedding,
    seen_count,
    first_seen_at,
    last_seen_at,
    create_time,
    modify_time
FROM kb.artifact_categories
WHERE category_type = 'inventory'
ON CONFLICT (category_key) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_kb_inventory_categories_status
    ON kb.inventory_categories (status);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_categories_seen_count
    ON kb.inventory_categories (seen_count DESC);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_categories_canonical_of
    ON kb.inventory_categories (canonical_of);

CREATE OR REPLACE FUNCTION kb.refresh_inventory_category_modify_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_refresh_inventory_category_modify_time ON kb.inventory_categories;
CREATE TRIGGER trg_refresh_inventory_category_modify_time
BEFORE UPDATE ON kb.inventory_categories
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_inventory_category_modify_time();
-- +goose StatementEnd
