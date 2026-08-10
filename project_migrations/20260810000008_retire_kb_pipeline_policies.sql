-- +goose Up
-- ADR 2026081001 DR3: kb.pipeline_policies is retired entirely. Confirmed
-- live data before writing this: 4 policy rows (1 active id=4, 3 archived),
-- 6 bindings and 14 rules all still active=true regardless of which policy
-- they belong to -- but only policy_id=4 (today's active policy) was ever
-- actually reachable through the WHERE policy_id=(SELECT ... active ...)
-- pattern every read site used, so bindings/rules pointing at an archived
-- policy are already-orphaned, unreachable data (discard, per design D6 --
-- nothing reads archived policy content, and DR1's version/status history
-- is the real replacement audit trail going forward).
DELETE FROM kb.pipeline_bindings WHERE policy_id IS NOT NULL AND policy_id <> (SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1);
DELETE FROM kb.pipeline_rules WHERE policy_id IS NOT NULL AND policy_id <> (SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1);

ALTER TABLE kb.pipeline_bindings DROP CONSTRAINT IF EXISTS fk_pipeline_bindings_policy;
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS fk_pipeline_rules_policy;

DROP INDEX IF EXISTS idx_kb_pipeline_bindings_policy_id;
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_policy_active_priority_scope;
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_store_policy;
DROP INDEX IF EXISTS idx_kb_pipeline_rules_policy_id;
DROP INDEX IF EXISTS idx_kb_pipeline_rules_policy_active_priority;
DROP INDEX IF EXISTS idx_kb_pipeline_rules_policy_target_processor_priority;

ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS policy_id;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS policy_id;

-- Replacement indexes, active-only (no policy_id to scope by anymore).
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_active_priority_scope
    ON kb.pipeline_bindings (active, priority DESC, input_record_id, user_id, ks_store_id, tenant_id);
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_target_processor_priority
    ON kb.pipeline_rules (target_processor, active, priority DESC);

-- ADR 2026081001 DR3: "at most one active unconditional binding per
-- context" -- the per-binding-target constraint that replaces
-- kb.pipeline_policies' "at most one system-wide active row" guarantee.
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_one_active_default_per_context
    ON kb.pipeline_bindings (
        COALESCE(ks_store_id, -1), COALESCE(user_id, ''),
        COALESCE(tenant_id, ''), COALESCE(input_record_id, -1)
    )
    WHERE active AND binding_kind = 'store_default';

DROP TABLE IF EXISTS kb.pipeline_policies;

-- +goose Down
CREATE TABLE IF NOT EXISTS kb.pipeline_policies (
    id BIGSERIAL PRIMARY KEY,
    version INT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    source_ref TEXT,
    checksum TEXT,
    activated_at TIMESTAMPTZ,
    activated_by TEXT,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_policies_one_active ON kb.pipeline_policies ((1)) WHERE status = 'active';
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_policies_unique_version ON kb.pipeline_policies (version);

DROP INDEX IF EXISTS idx_kb_pipeline_bindings_one_active_default_per_context;
DROP INDEX IF EXISTS idx_kb_pipeline_rules_target_processor_priority;
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_active_priority_scope;

ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS policy_id BIGINT;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS policy_id BIGINT;
ALTER TABLE kb.pipeline_bindings
    ADD CONSTRAINT fk_pipeline_bindings_policy FOREIGN KEY (policy_id) REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT;
ALTER TABLE kb.pipeline_rules
    ADD CONSTRAINT fk_pipeline_rules_policy FOREIGN KEY (policy_id) REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_policy_id ON kb.pipeline_bindings (policy_id);
CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_policy_id ON kb.pipeline_rules (policy_id);
