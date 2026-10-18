-- +goose Up
-- Structured metric frame: uses (resources with role and spec), method and procedure
-- (openspec change metric-frame). condition/condition_en stay the condition facet.
ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS frame JSONB;

CREATE INDEX IF NOT EXISTS idx_kb_metrics_frame ON kb.metrics USING GIN (frame jsonb_path_ops);

ALTER TABLE testbed.metrics
    ADD COLUMN IF NOT EXISTS frame JSONB;

-- +goose Down
ALTER TABLE testbed.metrics
    DROP COLUMN IF EXISTS frame;

DROP INDEX IF EXISTS kb.idx_kb_metrics_frame;

ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS frame;
