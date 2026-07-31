-- +goose Up
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS policy_id BIGINT;

UPDATE kb.pipeline_rules
SET policy_id = (SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1)
WHERE policy_id IS NULL;

ALTER TABLE kb.pipeline_rules ALTER COLUMN policy_id SET NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_pipeline_rules_policy'
          AND conrelid = 'kb.pipeline_rules'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_rules
            ADD CONSTRAINT fk_pipeline_rules_policy
            FOREIGN KEY (policy_id) REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT;
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_policy_id
    ON kb.pipeline_rules (policy_id);

-- Active-rule resolution now needs "active AND belongs to the active
-- policy" -- replace the (active, priority) index with one that also
-- covers policy_id so that filter isn't a sequential scan as authored
-- policy count grows.
DROP INDEX IF EXISTS idx_kb_pipeline_rules_active_priority;
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_policy_active_priority
    ON kb.pipeline_rules (policy_id, active, priority DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_pipeline_rules_policy_active_priority;
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_active_priority
    ON kb.pipeline_rules (active, priority DESC);
DROP INDEX IF EXISTS idx_kb_pipeline_rules_policy_id;
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS fk_pipeline_rules_policy;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS policy_id;
