-- +goose Up
-- Onboard the /resources page (Dashboard app-shell NavRail with pageKey='resources')
-- onto the DB-backed page-config capability (spec 2026072001 §11; ADR 2026072003).
-- entry_key = the id each nav node owns in the `resourcesNav` tree in
-- web/src/lib/components/home3/nav-rail.svelte. The pair (page_key, entry_key) is
-- the durable identity — never display text.
--
-- Content model matches the development seed (20260721000001): "en" rows carry
-- empty content ({}) so English falls back to the NavRail's hardcoded default
-- labels; "zh-cn" rows carry Chinese translations. Seeding all current nodes keeps
-- rendering identical (overlay model) and puts access control + translations in
-- place from the start.

INSERT INTO kb.page_def (page_key, route, title, description) VALUES
    ('resources', '/resources', 'Resources',
        'Configurable NavRail sidebar menu on /resources (the Dashboard app shell).')
ON CONFLICT (page_key) DO NOTHING;

-- ── resources: en rows (empty content = hardcoded English default) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('resources', 'documents', 'en', '{}'::jsonb),
    ('resources', 'docs-users-manual', 'en', '{}'::jsonb),
    ('resources', 'docs-development', 'en', '{}'::jsonb),
    ('resources', 'videos', 'en', '{}'::jsonb),
    ('resources', 'videos-training', 'en', '{}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- ── resources: zh-cn rows (Chinese label overrides) ──
INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('resources', 'documents', 'zh-cn', '{"label":"文档"}'::jsonb),
    ('resources', 'docs-users-manual', 'zh-cn', '{"label":"用户手册"}'::jsonb),
    ('resources', 'docs-development', 'zh-cn', '{"label":"开发"}'::jsonb),
    ('resources', 'videos', 'zh-cn', '{"label":"视频"}'::jsonb),
    ('resources', 'videos-training', 'zh-cn', '{"label":"培训"}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- Grant the current [system].access_roles set to every seeded entry so all
-- existing users retain access. Only fills rows with no access_role yet, so
-- admin edits are preserved on re-run.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'resources'
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'resources';
DELETE FROM kb.page_def    WHERE page_key = 'resources';
