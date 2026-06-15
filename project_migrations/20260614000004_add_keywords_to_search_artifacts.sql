-- +goose Up
-- Promotes kb.search_artifacts.semantic_payload.keywords to a dedicated TEXT[]
-- column so keywords can be indexed, filtered, and selected independently of the
-- opaque JSONB blob.  The backfill moves the value out of semantic_payload so the
-- two representations stay in sync without duplication.

ALTER TABLE kb.search_artifacts
    ADD COLUMN IF NOT EXISTS keywords TEXT[] NOT NULL DEFAULT '{}';

-- Backfill: extract semantic_payload->keywords into the new column and remove
-- it from the JSONB so it is no longer duplicated.  Only rows that carry the
-- key are touched; artifact types that never had keywords (product,
-- inventory_item) are left with the default empty array.
UPDATE kb.search_artifacts
SET
    keywords        = ARRAY(SELECT jsonb_array_elements_text(semantic_payload->'keywords')),
    semantic_payload = semantic_payload - 'keywords'
WHERE semantic_payload ? 'keywords';

-- +goose Down
-- Restore keywords back into semantic_payload and drop the column.
UPDATE kb.search_artifacts
SET
    semantic_payload = semantic_payload || jsonb_build_object('keywords', to_jsonb(keywords))
WHERE array_length(keywords, 1) IS NOT NULL;

ALTER TABLE kb.search_artifacts
    DROP COLUMN IF EXISTS keywords;
