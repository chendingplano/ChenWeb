-- +goose Up
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS policy_id BIGINT;

UPDATE kb.pipeline_bindings
SET policy_id = (SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1)
WHERE policy_id IS NULL;

ALTER TABLE kb.pipeline_bindings ALTER COLUMN policy_id SET NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'fk_pipeline_bindings_policy'
          AND conrelid = 'kb.pipeline_bindings'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_bindings
            ADD CONSTRAINT fk_pipeline_bindings_policy
            FOREIGN KEY (policy_id) REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT;
    END IF;
END $$;
-- +goose StatementEnd

-- A store's binding is now scoped to a policy version, not global: the same
-- store can have one binding under an old (archived) policy and a different
-- one authored under a new (draft, not-yet-activated) policy at the same
-- time. Uniqueness moves from (ks_store_id) alone to (ks_store_id, policy_id).
ALTER TABLE kb.pipeline_bindings DROP CONSTRAINT IF EXISTS pipeline_bindings_ks_store_id_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_store_policy
    ON kb.pipeline_bindings (ks_store_id, policy_id);

CREATE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_policy_id
    ON kb.pipeline_bindings (policy_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_policy_id;
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_store_policy;
ALTER TABLE kb.pipeline_bindings ADD CONSTRAINT pipeline_bindings_ks_store_id_key UNIQUE (ks_store_id);
ALTER TABLE kb.pipeline_bindings DROP CONSTRAINT IF EXISTS fk_pipeline_bindings_policy;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS policy_id;
