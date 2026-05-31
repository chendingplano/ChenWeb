-- +goose Up
-- +goose StatementBegin
CREATE SCHEMA IF NOT EXISTS kb;
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
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_inventory_categories_status
    ON kb.inventory_categories (status);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_categories_seen_count
    ON kb.inventory_categories (seen_count DESC);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_categories_canonical_of
    ON kb.inventory_categories (canonical_of);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_inventory_category_modify_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_inventory_category_modify_time ON kb.inventory_categories;
CREATE TRIGGER trg_refresh_inventory_category_modify_time
BEFORE UPDATE ON kb.inventory_categories
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_inventory_category_modify_time();

-- +goose Down
DROP TRIGGER IF EXISTS trg_refresh_inventory_category_modify_time ON kb.inventory_categories;
DROP FUNCTION IF EXISTS kb.refresh_inventory_category_modify_time();
DROP INDEX IF EXISTS idx_kb_inventory_categories_canonical_of;
DROP INDEX IF EXISTS idx_kb_inventory_categories_seen_count;
DROP INDEX IF EXISTS idx_kb_inventory_categories_status;
DROP TABLE IF EXISTS kb.inventory_categories;
