-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.inventory_items (
    id                     BIGSERIAL PRIMARY KEY,
    event_id               TEXT,
    input_record_id        BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    inventory_item_id      TEXT        NOT NULL,
    language               TEXT        NOT NULL DEFAULT '',
    item_name              TEXT        NOT NULL DEFAULT '',
    canonical_name         TEXT        NOT NULL DEFAULT '',
    item_category          TEXT        NOT NULL DEFAULT '',
    manufacturer           TEXT        NOT NULL DEFAULT '',
    brand                  TEXT        NOT NULL DEFAULT '',
    model_number           TEXT        NOT NULL DEFAULT '',
    part_number            TEXT        NOT NULL DEFAULT '',
    normalized_specs       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    raw_specs              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    standards              JSONB       NOT NULL DEFAULT '[]'::jsonb,
    aliases                JSONB       NOT NULL DEFAULT '[]'::jsonb,
    evidence_quote         TEXT        NOT NULL DEFAULT '',
    source_line_spans      JSONB       NOT NULL DEFAULT '[]'::jsonb,
    validation_flags       JSONB       NOT NULL DEFAULT '[]'::jsonb,
    missing_required_attrs JSONB       NOT NULL DEFAULT '[]'::jsonb,
    dedupe_key             TEXT        NOT NULL DEFAULT '',
    schema_version         TEXT        NOT NULL DEFAULT '',
    dictionary_version     TEXT        NOT NULL DEFAULT '',
    confidence             DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence_reason      TEXT        NOT NULL DEFAULT '',
    model_name             TEXT        NOT NULL DEFAULT '',
    prompt_name            TEXT        NOT NULL DEFAULT '',
    search_document        TEXT,
    search_vector          TSVECTOR,
    ext_info               JSONB       NOT NULL DEFAULT '{}'::jsonb,
    create_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_inventory_items_input_item UNIQUE (input_record_id, inventory_item_id)
);
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_input_record_id
    ON kb.inventory_items (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_category
    ON kb.inventory_items (item_category);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_manufacturer
    ON kb.inventory_items (manufacturer);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_brand
    ON kb.inventory_items (brand);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_model_number
    ON kb.inventory_items (model_number);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_part_number
    ON kb.inventory_items (part_number);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_dedupe_key
    ON kb.inventory_items (dedupe_key);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_validation_flags_gin
    ON kb.inventory_items USING GIN (validation_flags);

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

DROP TRIGGER IF EXISTS trg_refresh_inventory_item_search_columns ON kb.inventory_items;
CREATE TRIGGER trg_refresh_inventory_item_search_columns
BEFORE INSERT OR UPDATE OF
    item_name, canonical_name, item_category, manufacturer, brand,
    model_number, part_number, normalized_specs, raw_specs, standards,
    aliases, evidence_quote, validation_flags, missing_required_attrs,
    dedupe_key, confidence_reason
ON kb.inventory_items
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_inventory_item_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_search_vector
    ON kb.inventory_items USING GIN (search_vector);

CREATE TABLE IF NOT EXISTS kb.search_artifacts_inventory_item PARTITION OF kb.search_artifacts
    FOR VALUES IN ('inventory_item');
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_inventory_item_search_vector
    ON kb.search_artifacts_inventory_item USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_inventory_item_record
    ON kb.search_artifacts_inventory_item (input_record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_search_artifacts_inventory_item_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_inventory_item_search_vector;
DROP TABLE IF EXISTS kb.search_artifacts_inventory_item;
DROP INDEX IF EXISTS idx_kb_inventory_items_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_inventory_item_search_columns ON kb.inventory_items;
DROP FUNCTION IF EXISTS kb.refresh_inventory_item_search_columns();
DROP FUNCTION IF EXISTS kb.inventory_item_search_document(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, TEXT, JSONB, JSONB, TEXT, TEXT);
DROP INDEX IF EXISTS idx_kb_inventory_items_validation_flags_gin;
DROP INDEX IF EXISTS idx_kb_inventory_items_dedupe_key;
DROP INDEX IF EXISTS idx_kb_inventory_items_part_number;
DROP INDEX IF EXISTS idx_kb_inventory_items_model_number;
DROP INDEX IF EXISTS idx_kb_inventory_items_brand;
DROP INDEX IF EXISTS idx_kb_inventory_items_manufacturer;
DROP INDEX IF EXISTS idx_kb_inventory_items_category;
DROP INDEX IF EXISTS idx_kb_inventory_items_input_record_id;
DROP TABLE IF EXISTS kb.inventory_items;
