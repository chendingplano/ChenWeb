-- +goose Up
-- ADR 2026081001 DR6: real DAG edges. Names of sibling processors, within the
-- same pipeline version, that must finish successfully before
-- target_processor runs. Orthogonal to predicate/effect (which govern
-- whether a processor runs; this governs when).
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS depends_on_processors TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS depends_on_processors;
