-- +goose Up
-- 1. Seed a kb.page_config row for every /development NavRail node that has
--    none yet (web/src/lib/components/home3/nav-rail.svelte: mainNav,
--    bottomNav and developmentNav, recursively). These nodes were added to the
--    menu after 20260721000001_seed_page_config_development_nav.sql, so they
--    were not configurable from System Admin -> Page Config.
-- 2. Make the root-level items listed below accessible to the 'admin' role.
--
-- Labels are Paraglide messages (ADR 2026093001), so content stays '{}' in both
-- languages -- a label here would only shadow the message files.
--
-- A new row must carry an access_role: a row with a null access_role is
-- suspended (pageconfighandler/access.go fails closed), while a missing row is
-- visible (nav-rail fails open). Each new entry therefore inherits the
-- access_role of its parent's default-language (zh-cn) row, so seeding changes
-- nobody's menu. The new root 'development' has no parent and gets ["admin"].
--
-- Idempotent: inserts skip existing rows and the access_role fills only touch
-- rows that still have none, so admin edits survive a re-run.

CREATE TEMP TABLE tmp_dev_nav_seed (
    entry_key  VARCHAR(128) PRIMARY KEY,
    parent_key VARCHAR(128),
    depth      INT  NOT NULL,
    entry_desc TEXT NOT NULL
) ON COMMIT DROP;

INSERT INTO tmp_dev_nav_seed (entry_key, parent_key, depth, entry_desc) VALUES
    ('development', NULL, 0, '/development => Development'),
    ('apps-product-review', 'applications', 1, '/development => Applications => Product Review'),
    ('sysadmin-system', 'system-admin', 1, '/development => System Admin => System'),
    ('dev-demos', 'development', 1, '/development => Development => Demos'),
    ('sysadmin-benchmark-setup', 'sysadmin-benchmark', 2, '/development => System Admin => Benchmark => Setup'),
    ('sysadmin-db-clean-artifact-data', 'sysadmin-db', 2, '/development => System Admin => DB Maintenance => Clean Artifact Data'),
    ('sysadmin-db-resolve-metric-range-types', 'sysadmin-db', 2, '/development => System Admin => DB Maintenance => Resolve Metric Range Types'),
    ('sysadmin-db-resolve-orphaned-labels', 'sysadmin-db', 2, '/development => System Admin => DB Maintenance => Resolve Orphaned Labels'),
    ('sysadmin-llm-embedding', 'sysadmin-llm', 2, '/development => System Admin => LLM => Embedding'),
    ('sysadmin-llm-chat-sessions', 'sysadmin-llm', 2, '/development => System Admin => LLM => Chat Sessions'),
    ('sysadmin-llm-pi-sessions', 'sysadmin-llm', 2, '/development => System Admin => LLM => Pi Sessions'),
    ('sysadmin-llm-review-metrics', 'sysadmin-llm', 2, '/development => System Admin => LLM => Review Metrics'),
    ('sysadmin-resources-product-drawings', 'sysadmin-resources', 2, '/development => System Admin => Resources => Product Drawings'),
    ('sysadmin-resources-import-product-names', 'sysadmin-resources', 2, '/development => System Admin => Resources => Import Product Names'),
    ('sysadmin-resources-external-terminology', 'sysadmin-resources', 2, '/development => System Admin => Resources => External Terminology'),
    ('sysadmin-resources-review-external-terminology', 'sysadmin-resources', 2, '/development => System Admin => Resources => Review External Terminology'),
    ('sysadmin-resources-sync-data', 'sysadmin-resources', 2, '/development => System Admin => Resources => Sync Data'),
    ('sysadmin-system-calendar', 'sysadmin-system', 2, '/development => System Admin => System => Calendar'),
    ('sysadmin-system-peak-hours', 'sysadmin-system', 2, '/development => System Admin => System => Peak Hours'),
    ('sysadmin-system-releases', 'sysadmin-system', 2, '/development => System Admin => System => Releases'),
    ('sysadmin-system-prices', 'sysadmin-system', 2, '/development => System Admin => System => Prices'),
    ('dev-demos-main-page', 'dev-demos', 2, '/development => Development => Demos => Main Page'),
    ('dev-demos-jenny-main-page', 'dev-demos', 2, '/development => Development => Demos => Jenny Main Page'),
    ('sysadmin-resources-china-mechanical-product-names', 'sysadmin-resources-import-product-names', 3, '/development => System Admin => Resources => Import Product Names => China Mechanical Product Names');

INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc, created_by, updated_by)
SELECT 'development', s.entry_key, l.language, '{}'::jsonb, s.entry_desc,
       'migration-20260930000002', 'migration-20260930000002'
  FROM tmp_dev_nav_seed s
 CROSS JOIN (VALUES ('en'), ('zh-cn')) AS l(language)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

UPDATE kb.page_config
   SET access_role = '["admin"]'::jsonb
 WHERE page_key = 'development' AND entry_key = 'development' AND access_role IS NULL;

-- Inherit the parent's access_role one level at a time, so a new node whose
-- parent is also new (dev-demos, sysadmin-system, ...) sees the parent's value.
UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM tmp_dev_nav_seed s
  JOIN kb.page_config parent
    ON parent.page_key = 'development' AND parent.entry_key = s.parent_key
   AND parent.language = 'zh-cn'
 WHERE s.depth = 1 AND pc.page_key = 'development' AND pc.entry_key = s.entry_key
   AND pc.access_role IS NULL;

UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM tmp_dev_nav_seed s
  JOIN kb.page_config parent
    ON parent.page_key = 'development' AND parent.entry_key = s.parent_key
   AND parent.language = 'zh-cn'
 WHERE s.depth = 2 AND pc.page_key = 'development' AND pc.entry_key = s.entry_key
   AND pc.access_role IS NULL;

UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM tmp_dev_nav_seed s
  JOIN kb.page_config parent
    ON parent.page_key = 'development' AND parent.entry_key = s.parent_key
   AND parent.language = 'zh-cn'
 WHERE s.depth = 3 AND pc.page_key = 'development' AND pc.entry_key = s.entry_key
   AND pc.access_role IS NULL;

-- 2. Root-level items accessible to 'admin'. Adds the role where missing and
--    keeps every role already granted. 'agent-services' is the id of the
--    "Knowledge Desk" item.
UPDATE kb.page_config
   SET access_role = COALESCE(access_role, '[]'::jsonb) || '["admin"]'::jsonb,
       updated_at  = NOW(),
       updated_by  = 'migration-20260930000002'
 WHERE page_key = 'development'
   AND entry_key IN ('chat', 'agent-services', 'agents', 'skills', 'coding', 'personal',
                     'knowledge', 'knowledge-engineering', 'ontology', 'tools',
                     'agent-platform', 'development', 'settings', 'about')
   AND NOT COALESCE(access_role, '[]'::jsonb) ? 'admin';

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development'
   AND created_by = 'migration-20260930000002';
