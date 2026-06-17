-- +goose Up
-- Stage 3 of "drop persistence, compute connected_artifacts on demand" (Option A; see
-- KnowledgeStore/Capsules/coding-capsules/llm-wiki/artifact-connections.md §2.2).
--
-- All readers now call kb.connected_artifacts(input_record_id, '<type>', id) and the
-- buildArtifactConnectedArtifacts writer has been removed, so the materialized
-- connected_artifacts columns are no longer used. Drop them from every artifact family.
-- (The on-demand function + kb.chunk_ranges from 20260617000001 remain.)

ALTER TABLE kb.metrics              DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.semantic_projections DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.summaries            DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.topics               DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.scene_objects        DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.provisions           DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.entities             DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.relations            DROP COLUMN IF EXISTS connected_artifacts;
ALTER TABLE kb.inventory_items      DROP COLUMN IF EXISTS connected_artifacts;

-- +goose Down
-- Recreate empty columns (data is not restored; it is recomputable on demand).
ALTER TABLE kb.metrics              ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.semantic_projections ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.summaries            ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.topics               ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.scene_objects        ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.provisions           ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.entities             ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.relations            ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.inventory_items      ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;
