-- +goose Up
-- ADR 2026081001 DR2/DR3: pipeline versions are now authored atomically,
-- writing kb.pipeline_rules rows without a policy_id (the concept is being
-- retired). Relax NOT NULL now so the new atomic-authoring path can insert
-- rows before the full retirement migration (which drops this column
-- entirely, once every other read/write path has moved off it) lands.
ALTER TABLE kb.pipeline_rules ALTER COLUMN policy_id DROP NOT NULL;

-- +goose Down
ALTER TABLE kb.pipeline_rules ALTER COLUMN policy_id SET NOT NULL;
