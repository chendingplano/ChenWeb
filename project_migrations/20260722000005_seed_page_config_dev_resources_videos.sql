-- +goose Up
-- Register the new workspace nav nodes (SYSTEM ADMIN → Resources → Videos) for
-- the DB-backed page-config on the /development page, so the resolver recognizes
-- them (no "unrecognized nav entry id" warning) and provides zh-cn labels. The
-- ids match nav-rail.svelte: `sysadmin-resources` (sub-group) and
-- `sysadmin-resources-videos` (grandchild). /home3 has no page_key and shows
-- them by fail-open default.

INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('development', 'sysadmin-resources', 'en', '{}'::jsonb),
    ('development', 'sysadmin-resources-videos', 'en', '{}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('development', 'sysadmin-resources', 'zh-cn', '{"label":"资源"}'::jsonb),
    ('development', 'sysadmin-resources-videos', 'zh-cn', '{"label":"视频"}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development'
   AND entry_key IN ('sysadmin-resources', 'sysadmin-resources-videos')
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development'
   AND entry_key IN ('sysadmin-resources', 'sysadmin-resources-videos');
