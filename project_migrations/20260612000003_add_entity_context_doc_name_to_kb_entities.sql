-- +goose Up

-- source_line_spans was listed in the original spec but never added to kb.entities
-- (the column was named line_spans from the start); this is a no-op safety guard.
ALTER TABLE kb.entities DROP COLUMN IF EXISTS source_line_spans;

ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS entity_context TEXT;
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS doc_name TEXT;

-- +goose Down
ALTER TABLE kb.entities DROP COLUMN IF EXISTS doc_name;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS entity_context;
