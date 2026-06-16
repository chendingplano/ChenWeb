-- +goose Up
-- Roll out the line_range generated-column pattern (proven for kb.metrics in
-- 20260616000001) to the remaining artifact-family tables. See
-- KnowledgeStore/Capsules/coding-capsules/llm-wiki/artifact-connections.md §2.2.
--
-- Reuses kb.line_spans_to_int8multirange(jsonb), created by 20260616000001 (goose runs
-- migrations in order, so the function exists here). Each table's STORED generated column
-- derives the range from whichever span column that table uses (line_spans or
-- source_line_spans), backfills existing rows, and cannot drift from the spans.

ALTER TABLE kb.entities
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_entities_line_range
    ON kb.entities USING gist (line_range);

ALTER TABLE kb.relations
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_relations_line_range
    ON kb.relations USING gist (line_range);

ALTER TABLE kb.scene_objects
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_scene_objects_line_range
    ON kb.scene_objects USING gist (line_range);

ALTER TABLE kb.semantic_projections
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_semantic_projections_line_range
    ON kb.semantic_projections USING gist (line_range);

ALTER TABLE kb.inventory_items
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_line_range
    ON kb.inventory_items USING gist (line_range);

ALTER TABLE kb.provisions
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_provisions_line_range
    ON kb.provisions USING gist (line_range);

ALTER TABLE kb.summaries
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_summaries_line_range
    ON kb.summaries USING gist (line_range);

ALTER TABLE kb.topics
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED;
CREATE INDEX IF NOT EXISTS idx_kb_topics_line_range
    ON kb.topics USING gist (line_range);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_topics_line_range;
ALTER TABLE kb.topics DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_summaries_line_range;
ALTER TABLE kb.summaries DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_provisions_line_range;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_inventory_items_line_range;
ALTER TABLE kb.inventory_items DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_semantic_projections_line_range;
ALTER TABLE kb.semantic_projections DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_scene_objects_line_range;
ALTER TABLE kb.scene_objects DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_relations_line_range;
ALTER TABLE kb.relations DROP COLUMN IF EXISTS line_range;
DROP INDEX IF EXISTS kb.idx_kb_entities_line_range;
ALTER TABLE kb.entities DROP COLUMN IF EXISTS line_range;
