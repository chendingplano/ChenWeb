-- +goose Up
-- ADR 2026081001 DR3: routing/audit provenance is the resolved pipeline's
-- name+version now, not a kb.pipeline_policies id+version. kb.pipeline_policy_events
-- is confirmed empty (0 rows) as of this migration -- a pure structural change.
ALTER TABLE kb.pipeline_policy_events DROP CONSTRAINT IF EXISTS pipeline_policy_events_policy_id_fkey;
DROP INDEX IF EXISTS idx_pipeline_policy_events_policy;
ALTER TABLE kb.pipeline_policy_events DROP COLUMN IF EXISTS policy_id;
ALTER TABLE kb.pipeline_policy_events DROP COLUMN IF EXISTS policy_version;
ALTER TABLE kb.pipeline_policy_events ADD COLUMN IF NOT EXISTS pipeline_name VARCHAR(128);
ALTER TABLE kb.pipeline_policy_events ADD COLUMN IF NOT EXISTS pipeline_version INT;
CREATE INDEX IF NOT EXISTS idx_pipeline_policy_events_pipeline ON kb.pipeline_policy_events (pipeline_name, pipeline_version);

-- +goose Down
DROP INDEX IF EXISTS idx_pipeline_policy_events_pipeline;
ALTER TABLE kb.pipeline_policy_events DROP COLUMN IF EXISTS pipeline_version;
ALTER TABLE kb.pipeline_policy_events DROP COLUMN IF EXISTS pipeline_name;
ALTER TABLE kb.pipeline_policy_events ADD COLUMN IF NOT EXISTS policy_id BIGINT;
ALTER TABLE kb.pipeline_policy_events ADD COLUMN IF NOT EXISTS policy_version INT;
CREATE INDEX IF NOT EXISTS idx_pipeline_policy_events_policy ON kb.pipeline_policy_events (policy_id, policy_version);
ALTER TABLE kb.pipeline_policy_events
    ADD CONSTRAINT pipeline_policy_events_policy_id_fkey
    FOREIGN KEY (policy_id) REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT;
