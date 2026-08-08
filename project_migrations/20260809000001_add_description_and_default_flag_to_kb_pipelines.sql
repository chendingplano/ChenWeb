-- +goose Up
-- doc-processing-policy-seed (policy_seed.go) previously had no dedicated
-- column for a policy's description and wrote it into display_name instead,
-- and had no durable column at all for "is this the system default pipeline"
-- (config's is_default was consumed only transiently at seed time to pick
-- the NULL-ks_store_id binding). Both are real, config-authored fields
-- (config.local.toml's [doc-processing-policy-*].description/is_default) --
-- give them real columns instead of overloading display_name / deriving the
-- default status only from a join against kb.pipeline_bindings.
ALTER TABLE kb.pipelines ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE kb.pipelines ADD COLUMN IF NOT EXISTS is_system_default BOOLEAN NOT NULL DEFAULT false;

-- Mirrors idx_kb_pipeline_policies_one_active: at most one pipeline may be
-- flagged as the system default at a time.
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipelines_one_system_default
    ON kb.pipelines ((1))
    WHERE is_system_default;

-- +goose Down
DROP INDEX IF EXISTS idx_kb_pipelines_one_system_default;
ALTER TABLE kb.pipelines DROP COLUMN IF EXISTS is_system_default;
ALTER TABLE kb.pipelines DROP COLUMN IF EXISTS description;
