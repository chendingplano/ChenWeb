-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.inventory_item_duplicates (
    id                     BIGSERIAL PRIMARY KEY,
    event_id               TEXT,
    input_record_id        BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    inventory_item_id      TEXT        NOT NULL,
    duplicate_of           TEXT        NOT NULL,
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
    ext_info               JSONB       NOT NULL DEFAULT '{}'::jsonb,
    create_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_inventory_item_duplicates_input_item UNIQUE (input_record_id, inventory_item_id)
);
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_duplicates_input_record_id
    ON kb.inventory_item_duplicates (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_duplicates_duplicate_of
    ON kb.inventory_item_duplicates (duplicate_of);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_duplicates_dedupe_key
    ON kb.inventory_item_duplicates (dedupe_key);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_inventory_item_duplicates_dedupe_key;
DROP INDEX IF EXISTS idx_kb_inventory_item_duplicates_duplicate_of;
DROP INDEX IF EXISTS idx_kb_inventory_item_duplicates_input_record_id;
DROP TABLE IF EXISTS kb.inventory_item_duplicates;
