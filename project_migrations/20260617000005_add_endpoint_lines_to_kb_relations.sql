-- +goose Up
-- ADR 2026061702 (DR2/DR3) — decoupled free-form relation extraction.
-- The v2 relation prompt (prompt-extract-relations-v2.md) cites each endpoint's
-- source lines (subject_lines / predicate_lines / object_lines). In the decoupled
-- flow the relation pass saves BEFORE endpoint linking (which runs in Phase C,
-- DR4), so these citations must be persisted to survive to the link step.
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS subject_lines   JSONB;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS predicate_lines JSONB;
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS object_lines    JSONB;

-- +goose Down
ALTER TABLE kb.relations DROP COLUMN IF EXISTS object_lines;
ALTER TABLE kb.relations DROP COLUMN IF EXISTS predicate_lines;
ALTER TABLE kb.relations DROP COLUMN IF EXISTS subject_lines;
