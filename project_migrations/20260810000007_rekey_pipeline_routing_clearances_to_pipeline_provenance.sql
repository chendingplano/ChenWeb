-- +goose Up
-- ADR 2026081001 DR3: D2 routing-clearance subjects are keyed by the
-- resolved pipeline's name+version now, not a kb.pipeline_policies id+
-- version. Both tables confirmed empty (0 rows) as of this migration -- a
-- pure structural change, no backfill needed.
ALTER TABLE kb.pipeline_routing_clearances DROP CONSTRAINT IF EXISTS pipeline_routing_clearances_policy_id_fkey;
ALTER TABLE kb.pipeline_routing_clearances DROP COLUMN IF EXISTS policy_id;
ALTER TABLE kb.pipeline_routing_clearances DROP COLUMN IF EXISTS policy_version;
ALTER TABLE kb.pipeline_routing_clearances ADD COLUMN IF NOT EXISTS pipeline_name VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE kb.pipeline_routing_clearances ADD COLUMN IF NOT EXISTS pipeline_version INT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_routing_clearances ALTER COLUMN pipeline_name DROP DEFAULT;
ALTER TABLE kb.pipeline_routing_clearances ALTER COLUMN pipeline_version DROP DEFAULT;

DROP INDEX IF EXISTS idx_pipeline_routing_clearance_coverage_subject;
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP CONSTRAINT IF EXISTS pipeline_routing_clearance_co_policy_id_policy_version_subj_key;
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP CONSTRAINT IF EXISTS pipeline_routing_clearance_coverage_policy_id_fkey;
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP COLUMN IF EXISTS policy_id;
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP COLUMN IF EXISTS policy_version;
ALTER TABLE kb.pipeline_routing_clearance_coverage ADD COLUMN IF NOT EXISTS pipeline_name VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE kb.pipeline_routing_clearance_coverage ADD COLUMN IF NOT EXISTS pipeline_version INT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_routing_clearance_coverage ALTER COLUMN pipeline_name DROP DEFAULT;
ALTER TABLE kb.pipeline_routing_clearance_coverage ALTER COLUMN pipeline_version DROP DEFAULT;
CREATE INDEX IF NOT EXISTS idx_pipeline_routing_clearance_coverage_subject
    ON kb.pipeline_routing_clearance_coverage (pipeline_name, pipeline_version, subject_kind, subject_id, document_kind, subject_checksum, net_plan_delta_checksum);
ALTER TABLE kb.pipeline_routing_clearance_coverage
    ADD CONSTRAINT pipeline_routing_clearance_co_pipeline_subj_key
    UNIQUE (pipeline_name, pipeline_version, subject_kind, subject_id, document_kind, clearance_id);

-- +goose Down
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP CONSTRAINT IF EXISTS pipeline_routing_clearance_co_pipeline_subj_key;
DROP INDEX IF EXISTS idx_pipeline_routing_clearance_coverage_subject;
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP COLUMN IF EXISTS pipeline_version;
ALTER TABLE kb.pipeline_routing_clearance_coverage DROP COLUMN IF EXISTS pipeline_name;
ALTER TABLE kb.pipeline_routing_clearance_coverage ADD COLUMN IF NOT EXISTS policy_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_routing_clearance_coverage ADD COLUMN IF NOT EXISTS policy_version INT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_routing_clearance_coverage ALTER COLUMN policy_id DROP DEFAULT;
ALTER TABLE kb.pipeline_routing_clearance_coverage ALTER COLUMN policy_version DROP DEFAULT;
CREATE INDEX IF NOT EXISTS idx_pipeline_routing_clearance_coverage_subject
    ON kb.pipeline_routing_clearance_coverage (policy_id, policy_version, subject_kind, subject_id, document_kind, subject_checksum, net_plan_delta_checksum);
ALTER TABLE kb.pipeline_routing_clearance_coverage
    ADD CONSTRAINT pipeline_routing_clearance_co_policy_id_policy_version_subj_key
    UNIQUE (policy_id, policy_version, subject_kind, subject_id, document_kind, clearance_id);

ALTER TABLE kb.pipeline_routing_clearances DROP COLUMN IF EXISTS pipeline_version;
ALTER TABLE kb.pipeline_routing_clearances DROP COLUMN IF EXISTS pipeline_name;
ALTER TABLE kb.pipeline_routing_clearances ADD COLUMN IF NOT EXISTS policy_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_routing_clearances ADD COLUMN IF NOT EXISTS policy_version INT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_routing_clearances ALTER COLUMN policy_id DROP DEFAULT;
ALTER TABLE kb.pipeline_routing_clearances ALTER COLUMN policy_version DROP DEFAULT;
ALTER TABLE kb.pipeline_routing_clearances
    ADD CONSTRAINT pipeline_routing_clearances_policy_id_fkey
    FOREIGN KEY (policy_id) REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT;
