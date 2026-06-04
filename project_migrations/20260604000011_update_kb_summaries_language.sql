-- +goose Up

-- Drop trigger and old function first — trigger has UPDATE OF category_paths/category_paths_en
-- which creates column dependencies that block the DROP COLUMN below
DROP TRIGGER IF EXISTS trg_refresh_summary_search_columns ON kb.summaries;
DROP FUNCTION IF EXISTS kb.summary_search_document(TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, JSONB);

-- Add language column
ALTER TABLE kb.summaries ADD COLUMN IF NOT EXISTS language TEXT NOT NULL DEFAULT '';

-- Drop removed category columns (safe now that the trigger is gone)
ALTER TABLE kb.summaries
    DROP COLUMN IF EXISTS category_paths,
    DROP COLUMN IF EXISTS category_paths_en,
    DROP COLUMN IF EXISTS category_nodes,
    DROP COLUMN IF EXISTS category_path_items,
    DROP COLUMN IF EXISTS category_path_items_en;

-- Create updated 5-arg search document function (no category params)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.summary_search_document(
    summary_text TEXT,
    summary_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    child_summary_ids JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(summary_text, ''),
        COALESCE(summary_text_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en),
        kb.search_jsonb_array_text(child_summary_ids)
    ));
$$;
-- +goose StatementEnd

-- Replace trigger function to use updated search document signature
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_summary_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.summary_search_document(
        NEW.summary_text,
        NEW.summary_text_en,
        NEW.keywords,
        NEW.keywords_en,
        NEW.child_summary_ids
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER trg_refresh_summary_search_columns
BEFORE INSERT OR UPDATE OF
    summary_text, summary_text_en,
    keywords, keywords_en,
    child_summary_ids, language
ON kb.summaries
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_summary_search_columns();

-- +goose Down

DROP TRIGGER IF EXISTS trg_refresh_summary_search_columns ON kb.summaries;

-- Restore original trigger function (with category_paths/category_paths_en)
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_summary_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.summary_search_document(
        NEW.summary_text,
        NEW.summary_text_en,
        NEW.keywords,
        NEW.keywords_en,
        NEW.category_paths,
        NEW.category_paths_en,
        NEW.child_summary_ids
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

-- Drop new 5-arg overload before restoring old 7-arg version
DROP FUNCTION IF EXISTS kb.summary_search_document(TEXT, TEXT, JSONB, JSONB, JSONB);

-- Restore original search document function
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.summary_search_document(
    summary_text TEXT,
    summary_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB,
    child_summary_ids JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(summary_text, ''),
        COALESCE(summary_text_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en),
        kb.search_jsonb_array_text(child_summary_ids)
    ));
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_summary_search_columns ON kb.summaries;
CREATE TRIGGER trg_refresh_summary_search_columns
BEFORE INSERT OR UPDATE OF
    summary_text, summary_text_en,
    keywords, keywords_en,
    category_paths, category_paths_en,
    child_summary_ids
ON kb.summaries
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_summary_search_columns();

-- Restore dropped columns
ALTER TABLE kb.summaries
    ADD COLUMN IF NOT EXISTS category_paths         JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS category_paths_en      JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS category_nodes         JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS category_path_items    JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS category_path_items_en JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Drop language column
ALTER TABLE kb.summaries DROP COLUMN IF EXISTS language;
