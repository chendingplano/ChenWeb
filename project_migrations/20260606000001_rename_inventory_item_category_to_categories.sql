-- +goose Up
-- Rename kb.inventory_items.item_category (single TEXT) to item_categories (JSONB array)
-- to match the LLM output, which now emits an array of categories per item. Also add
-- connected_artifacts (mirrors kb.metrics) for Phase C line-overlap indexing.

-- The search_document trigger/function reference the old scalar column, so drop them
-- first, swap the column, then rebuild against the array form.
DROP TRIGGER IF EXISTS trg_refresh_inventory_item_search_columns ON kb.inventory_items;
DROP FUNCTION IF EXISTS kb.refresh_inventory_item_search_columns();
DROP FUNCTION IF EXISTS kb.inventory_item_search_document(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, TEXT, JSONB, JSONB, TEXT, TEXT);
DROP INDEX IF EXISTS idx_kb_inventory_items_category;

-- kb.inventory_items: scalar item_category -> JSONB array item_categories.
ALTER TABLE kb.inventory_items ADD COLUMN IF NOT EXISTS item_categories JSONB NOT NULL DEFAULT '[]'::jsonb;
-- +goose StatementBegin
UPDATE kb.inventory_items
SET item_categories = CASE
        WHEN item_category IS NULL OR btrim(item_category) = '' THEN '[]'::jsonb
        ELSE jsonb_build_array(item_category)
    END
WHERE item_category IS NOT NULL;
-- +goose StatementEnd
ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS item_category;
ALTER TABLE kb.inventory_items ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

-- kb.inventory_item_duplicates: same column swap (no search trigger on this table).
ALTER TABLE kb.inventory_item_duplicates ADD COLUMN IF NOT EXISTS item_categories JSONB NOT NULL DEFAULT '[]'::jsonb;
-- +goose StatementBegin
UPDATE kb.inventory_item_duplicates
SET item_categories = CASE
        WHEN item_category IS NULL OR btrim(item_category) = '' THEN '[]'::jsonb
        ELSE jsonb_build_array(item_category)
    END
WHERE item_category IS NOT NULL;
-- +goose StatementEnd
ALTER TABLE kb.inventory_item_duplicates DROP COLUMN IF EXISTS item_category;

-- Membership index for the item_categories filter (semantic_payload / handler @>).
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_categories
    ON kb.inventory_items USING GIN (item_categories jsonb_path_ops);

-- Rebuild the search_document builder against the JSONB array form.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.inventory_item_search_document(
    item_name TEXT,
    canonical_name TEXT,
    item_categories JSONB,
    manufacturer TEXT,
    brand TEXT,
    model_number TEXT,
    part_number TEXT,
    normalized_specs JSONB,
    raw_specs JSONB,
    standards JSONB,
    aliases JSONB,
    evidence_quote TEXT,
    validation_flags JSONB,
    missing_required_attrs JSONB,
    dedupe_key TEXT,
    confidence_reason TEXT
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(item_name, ''),
        COALESCE(canonical_name, ''),
        kb.search_jsonb_array_text(item_categories),
        COALESCE(manufacturer, ''),
        COALESCE(brand, ''),
        COALESCE(model_number, ''),
        COALESCE(part_number, ''),
        kb.search_jsonb_array_text(normalized_specs),
        kb.search_jsonb_array_text(raw_specs),
        kb.search_jsonb_array_text(standards),
        kb.search_jsonb_array_text(aliases),
        COALESCE(evidence_quote, ''),
        kb.search_jsonb_array_text(validation_flags),
        kb.search_jsonb_array_text(missing_required_attrs),
        COALESCE(dedupe_key, ''),
        COALESCE(confidence_reason, '')
    ));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_inventory_item_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    NEW.search_document := kb.inventory_item_search_document(
        NEW.item_name,
        NEW.canonical_name,
        NEW.item_categories,
        NEW.manufacturer,
        NEW.brand,
        NEW.model_number,
        NEW.part_number,
        NEW.normalized_specs,
        NEW.raw_specs,
        NEW.standards,
        NEW.aliases,
        NEW.evidence_quote,
        NEW.validation_flags,
        NEW.missing_required_attrs,
        NEW.dedupe_key,
        NEW.confidence_reason
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_refresh_inventory_item_search_columns
BEFORE INSERT OR UPDATE OF
    item_name, canonical_name, item_categories, manufacturer, brand,
    model_number, part_number, normalized_specs, raw_specs, standards,
    aliases, evidence_quote, validation_flags, missing_required_attrs,
    dedupe_key, confidence_reason
ON kb.inventory_items
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_inventory_item_search_columns();

-- +goose Down
DROP TRIGGER IF EXISTS trg_refresh_inventory_item_search_columns ON kb.inventory_items;
DROP FUNCTION IF EXISTS kb.refresh_inventory_item_search_columns();
DROP FUNCTION IF EXISTS kb.inventory_item_search_document(TEXT, TEXT, JSONB, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, TEXT, JSONB, JSONB, TEXT, TEXT);
DROP INDEX IF EXISTS idx_kb_inventory_items_categories;

ALTER TABLE kb.inventory_item_duplicates ADD COLUMN IF NOT EXISTS item_category TEXT NOT NULL DEFAULT '';
-- +goose StatementBegin
UPDATE kb.inventory_item_duplicates
SET item_category = COALESCE(item_categories->>0, '')
WHERE item_categories IS NOT NULL;
-- +goose StatementEnd
ALTER TABLE kb.inventory_item_duplicates DROP COLUMN IF EXISTS item_categories;

ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.inventory_items ADD COLUMN IF NOT EXISTS item_category TEXT NOT NULL DEFAULT '';
-- +goose StatementBegin
UPDATE kb.inventory_items
SET item_category = COALESCE(item_categories->>0, '')
WHERE item_categories IS NOT NULL;
-- +goose StatementEnd
ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS item_categories;

CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_category
    ON kb.inventory_items (item_category);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.inventory_item_search_document(
    item_name TEXT,
    canonical_name TEXT,
    item_category TEXT,
    manufacturer TEXT,
    brand TEXT,
    model_number TEXT,
    part_number TEXT,
    normalized_specs JSONB,
    raw_specs JSONB,
    standards JSONB,
    aliases JSONB,
    evidence_quote TEXT,
    validation_flags JSONB,
    missing_required_attrs JSONB,
    dedupe_key TEXT,
    confidence_reason TEXT
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(item_name, ''),
        COALESCE(canonical_name, ''),
        COALESCE(item_category, ''),
        COALESCE(manufacturer, ''),
        COALESCE(brand, ''),
        COALESCE(model_number, ''),
        COALESCE(part_number, ''),
        kb.search_jsonb_array_text(normalized_specs),
        kb.search_jsonb_array_text(raw_specs),
        kb.search_jsonb_array_text(standards),
        kb.search_jsonb_array_text(aliases),
        COALESCE(evidence_quote, ''),
        kb.search_jsonb_array_text(validation_flags),
        kb.search_jsonb_array_text(missing_required_attrs),
        COALESCE(dedupe_key, ''),
        COALESCE(confidence_reason, '')
    ));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_inventory_item_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    NEW.search_document := kb.inventory_item_search_document(
        NEW.item_name,
        NEW.canonical_name,
        NEW.item_category,
        NEW.manufacturer,
        NEW.brand,
        NEW.model_number,
        NEW.part_number,
        NEW.normalized_specs,
        NEW.raw_specs,
        NEW.standards,
        NEW.aliases,
        NEW.evidence_quote,
        NEW.validation_flags,
        NEW.missing_required_attrs,
        NEW.dedupe_key,
        NEW.confidence_reason
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_refresh_inventory_item_search_columns
BEFORE INSERT OR UPDATE OF
    item_name, canonical_name, item_category, manufacturer, brand,
    model_number, part_number, normalized_specs, raw_specs, standards,
    aliases, evidence_quote, validation_flags, missing_required_attrs,
    dedupe_key, confidence_reason
ON kb.inventory_items
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_inventory_item_search_columns();
