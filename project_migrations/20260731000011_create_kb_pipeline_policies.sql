-- +goose Up
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

-- At most one policy can be active at a time -- resolution (bindings/rules)
-- always filters against exactly this row, so "the active policy" must be
-- unambiguous.
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_policies_one_active
    ON kb.pipeline_policies ((1)) WHERE status = 'active';

-- Bootstrap policy, seeded active, so kb.pipeline_bindings/kb.pipeline_rules
-- (both getting a NOT NULL policy_id in the next two migrations) have
-- something to backfill against with zero behavior change -- matching P1's
-- "no active policy means legacy behavior" invariant: there is always
-- exactly one active policy, and it starts out owning everything already
-- authored.
INSERT INTO kb.pipeline_policies (version, status, source_ref, activated_at, activated_by)
SELECT 1, 'active', 'bootstrap', NOW(), 'system'
WHERE NOT EXISTS (SELECT 1 FROM kb.pipeline_policies WHERE status = 'active');

-- +goose Down
DROP TABLE IF EXISTS kb.pipeline_policies;
