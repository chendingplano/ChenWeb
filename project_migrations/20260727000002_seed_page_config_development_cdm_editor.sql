-- +goose Up
-- Onboard the new "CDM Editor" nav child (Tools -> CDM Editor,
-- web/src/lib/components/home3/nav-rail.svelte's `tools` item) onto the
-- DB-backed page-config capability for /development
-- (20260721000001_seed_page_config_development_nav.sql), which seeds one row
-- per nav node so every node has its zh-cn translation and an access_role
-- from day one, per that migration's own stated convention.
--
-- entry_key 'cdm-editor' matches the id nav-rail.svelte's `tools.children`
-- array now carries. The "en" row is empty ({}), so English renders the
-- component's hardcoded default label ("CDM Editor"); the "zh-cn" row carries
-- "智能编辑器" -- the same translation already used for the /semos/workspace
-- app tile (20260727000001_seed_page_config_cdm_editor_app.sql), so the
-- feature has one Chinese name across the product rather than two.
--
-- access_role is not optional: unlike a missing row (which nav-rail's
-- isVisible treats as visible by default -- fail open), a row that exists
-- with a null access_role is *suspended* by isAuthorized
-- (pageconfighandler/access.go), which fails closed. Grants the same
-- [system].access_roles set every other /development node carries.
--
-- Idempotent so air re-running it is safe.

INSERT INTO kb.page_config (page_key, entry_key, language, content) VALUES
    ('development', 'cdm-editor', 'en',    '{}'::jsonb),
    ('development', 'cdm-editor', 'zh-cn', '{"label":"智能编辑器"}'::jsonb)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- Same [system].access_roles set the rest of /development's nodes carry.
-- Scoped to this entry and to rows with no access_role yet, so admin edits
-- are preserved on re-run.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development'
   AND entry_key = 'cdm-editor'
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development' AND entry_key = 'cdm-editor';
