-- +goose Up
-- P5 storage evolution: kb.pipeline_bindings becomes the authoritative
-- pipeline-selection table for both conditional and store-default rows, while
-- kb.pipeline_rules transitions to processor gates. Existing legacy selector
-- rows remain readable but are copied into bindings with canonical v1 JSON
-- predicates and preserved legacy_rule_id linkage.

ALTER TABLE kb.pipeline_bindings ALTER COLUMN ks_store_id DROP NOT NULL;

ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS name VARCHAR(128);
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0;
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(128) NOT NULL DEFAULT '-';
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS user_id VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS input_record_id BIGINT;
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS binding_kind TEXT NOT NULL DEFAULT 'store_default';
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS predicate JSONB;
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS predicate_checksum TEXT;
ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS legacy_rule_id BIGINT;

UPDATE kb.pipeline_bindings b
SET tenant_id = COALESCE(ks.tenant_id, '-')
FROM kb.knowledge_store ks
WHERE ks.id = b.ks_store_id
  AND (b.tenant_id IS NULL OR b.tenant_id = '');

ALTER TABLE kb.pipeline_rules ALTER COLUMN pipeline_id DROP NOT NULL;

ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS predicate JSONB;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS predicate_checksum TEXT;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS target_processor TEXT;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS effect TEXT;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS required_facets JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS module_id TEXT;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS released_in_release_id BIGINT;
ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'draft';

ALTER TABLE kb.pipeline_rules
    ADD CONSTRAINT fk_pipeline_rules_released_in_release
    FOREIGN KEY (released_in_release_id) REFERENCES kb.ontology_module_releases(id) ON DELETE RESTRICT;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipeline_bindings_binding_kind'
          AND conrelid = 'kb.pipeline_bindings'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_bindings
            ADD CONSTRAINT ck_pipeline_bindings_binding_kind
            CHECK (binding_kind IN ('conditional', 'store_default'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipeline_bindings_predicate_checksum_pair'
          AND conrelid = 'kb.pipeline_bindings'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_bindings
            ADD CONSTRAINT ck_pipeline_bindings_predicate_checksum_pair
            CHECK ((predicate IS NULL) = (predicate_checksum IS NULL));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipeline_bindings_no_new_store_default_predicate'
          AND conrelid = 'kb.pipeline_bindings'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_bindings
            ADD CONSTRAINT ck_pipeline_bindings_no_new_store_default_predicate
            CHECK (NOT (legacy_rule_id IS NULL AND predicate IS NOT NULL AND binding_kind = 'store_default'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipeline_rules_predicate_checksum_pair'
          AND conrelid = 'kb.pipeline_rules'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_rules
            ADD CONSTRAINT ck_pipeline_rules_predicate_checksum_pair
            CHECK ((predicate IS NULL) = (predicate_checksum IS NULL));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipeline_rules_effect'
          AND conrelid = 'kb.pipeline_rules'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_rules
            ADD CONSTRAINT ck_pipeline_rules_effect
            CHECK (effect IN ('require', 'enable', 'skip', 'defer'));
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'ck_pipeline_rules_approval_status'
          AND conrelid = 'kb.pipeline_rules'::regclass
    ) THEN
        ALTER TABLE kb.pipeline_rules
            ADD CONSTRAINT ck_pipeline_rules_approval_status
            CHECK (approval_status IN ('draft', 'approved', 'included_in_release', 'rejected'));
    END IF;
END $$;
-- +goose StatementEnd

WITH legacy_rules AS (
    SELECT
        r.id,
        r.name,
        r.priority,
        r.active,
        r.pipeline_id,
        r.policy_id,
        jsonb_build_object(
            'version', 1,
            'expression', jsonb_build_object(
                'kind', 'all',
                'items', to_jsonb(array_remove(ARRAY[
                    CASE
                        WHEN NULLIF(LOWER(BTRIM(COALESCE(r.match_input_doc_type, ''))), '') IS NULL THEN NULL
                        ELSE jsonb_build_object(
                            'kind', 'fact',
                            'path', 'document.input_doc_type',
                            'op', 'eq',
                            'value', LOWER(BTRIM(r.match_input_doc_type))
                        )
                    END,
                    CASE
                        WHEN NULLIF(LOWER(BTRIM(COALESCE(r.match_source_language, ''))), '') IS NULL THEN NULL
                        ELSE jsonb_build_object(
                            'kind', 'fact',
                            'path', 'document.source_language',
                            'op', 'eq',
                            'value', LOWER(BTRIM(r.match_source_language))
                        )
                    END,
                    CASE
                        WHEN NULLIF(LOWER(BTRIM(COALESCE(r.match_knowledge_store_binding, ''))), '') IS NULL THEN NULL
                        ELSE jsonb_build_object(
                            'kind', 'fact',
                            'path', 'document.knowledge_store_binding_state',
                            'op', 'eq',
                            'value', LOWER(BTRIM(r.match_knowledge_store_binding))
                        )
                    END
                ]::jsonb[], NULL))
            )
        ) AS predicate
    FROM kb.pipeline_rules r
)
INSERT INTO kb.pipeline_bindings (
    ks_store_id,
    pipeline_id,
    policy_id,
    name,
    priority,
    active,
    tenant_id,
    user_id,
    input_record_id,
    binding_kind,
    predicate,
    predicate_checksum,
    legacy_rule_id,
    create_time,
    modify_time
)
SELECT
    NULL,
    lr.pipeline_id,
    lr.policy_id,
    lr.name,
    lr.priority,
    lr.active,
    '-',
    '',
    NULL,
    'conditional',
    lr.predicate,
    md5(lr.predicate::text),
    lr.id,
    NOW(),
    NOW()
FROM legacy_rules lr
WHERE NOT EXISTS (
    SELECT 1
    FROM kb.pipeline_bindings b
    WHERE b.legacy_rule_id = lr.id
);

DROP INDEX IF EXISTS idx_kb_pipeline_bindings_store_policy;

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_legacy_rule_id
    ON kb.pipeline_bindings (legacy_rule_id)
    WHERE legacy_rule_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_policy_active_priority_scope
    ON kb.pipeline_bindings (policy_id, active, priority DESC, input_record_id, user_id, ks_store_id, tenant_id);

CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_policy_target_processor_priority
    ON kb.pipeline_rules (policy_id, target_processor, active, priority DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_pipeline_rules_policy_target_processor_priority;
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_policy_active_priority_scope;
DROP INDEX IF EXISTS idx_kb_pipeline_bindings_legacy_rule_id;

DELETE FROM kb.pipeline_bindings WHERE legacy_rule_id IS NOT NULL;

ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS ck_pipeline_rules_approval_status;
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS ck_pipeline_rules_effect;
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS ck_pipeline_rules_predicate_checksum_pair;
ALTER TABLE kb.pipeline_rules DROP CONSTRAINT IF EXISTS fk_pipeline_rules_released_in_release;

ALTER TABLE kb.pipeline_bindings DROP CONSTRAINT IF EXISTS ck_pipeline_bindings_no_new_store_default_predicate;
ALTER TABLE kb.pipeline_bindings DROP CONSTRAINT IF EXISTS ck_pipeline_bindings_predicate_checksum_pair;
ALTER TABLE kb.pipeline_bindings DROP CONSTRAINT IF EXISTS ck_pipeline_bindings_binding_kind;

ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS approval_status;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS released_in_release_id;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS module_id;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS required_facets;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS effect;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS target_processor;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS predicate_checksum;
ALTER TABLE kb.pipeline_rules DROP COLUMN IF EXISTS predicate;
ALTER TABLE kb.pipeline_rules ALTER COLUMN pipeline_id SET NOT NULL;

ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS legacy_rule_id;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS predicate_checksum;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS predicate;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS binding_kind;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS input_record_id;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS user_id;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS active;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS priority;
ALTER TABLE kb.pipeline_bindings DROP COLUMN IF EXISTS name;
ALTER TABLE kb.pipeline_bindings ALTER COLUMN ks_store_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_store_policy
    ON kb.pipeline_bindings (ks_store_id, policy_id);
