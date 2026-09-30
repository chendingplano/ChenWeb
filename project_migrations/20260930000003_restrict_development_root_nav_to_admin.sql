-- +goose Up
-- Restrict these /development root-level NavRail items to the 'admin' role only.
-- 20260930000002 only ensured 'admin' was present, which left every other role
-- (dev, guest, trial, ...) with access. Hiding a root item hides its whole
-- subtree in nav-rail.svelte, so child rows are left as they are.
-- 'agent-services' is the id of the "Knowledge Desk" item.
UPDATE kb.page_config
   SET access_role = '["admin"]'::jsonb,
       updated_at  = NOW(),
       updated_by  = 'migration-20260930000003'
 WHERE page_key = 'development'
   AND entry_key IN ('chat', 'agent-services', 'agents', 'skills', 'coding', 'personal',
                     'knowledge', 'knowledge-engineering', 'ontology', 'tools',
                     'agent-platform', 'development', 'settings', 'about');

-- +goose Down
-- Restore the role sets these rows carried before this migration.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development'
   AND entry_key IN ('chat', 'agents', 'skills', 'coding', 'personal', 'knowledge',
                     'tools', 'agent-platform', 'settings', 'about');
UPDATE kb.page_config
   SET access_role = '["admin","root","dev","k_engineer"]'::jsonb
 WHERE page_key = 'development' AND entry_key = 'agent-services';
UPDATE kb.page_config
   SET access_role = '["admin","root","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development' AND entry_key IN ('knowledge-engineering', 'ontology');
