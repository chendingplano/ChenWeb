-- +goose Up
-- Onboard the new "CDM Editor" workspace app tile onto the DB-backed
-- page-config capability (KnowledgeStore spec 2026072001 §11), alongside the
-- six tiles 20260720000002_seed_page_config_existing_domains.sql seeded.
--
-- The tile itself is declared in config/site/*.toml as
-- [[workspace.apps]] key = "cdm_editor" -> /home3/cdm. That is its base
-- content (label, description, href); these rows only add the per-language
-- override layer /semos/workspace resolves on top of it, exactly as the other
-- six tiles do:
--   - the "en" row carries empty content ({}), so English renders the
--     site-config default ("CDM Editor" / "Author documents as meaning, …");
--   - the "zh-cn" row carries the translation, mirroring the entry added to
--     config/workspace-content/labels-zh-cn.toml in the same change.
--
-- access_role is NOT optional here. isAuthorized (pageconfighandler/access.go)
-- fails closed: a row whose access_role is null or empty is *suspended*, and
-- the handler reports a suspended entry in `hidden`, which would hide this tile
-- from every user. It is evaluated on the default-language row
-- ([languages].default = "zh-cn"), so both rows are seeded with the same role
-- set 20260720000002's closing UPDATE granted the existing tiles.
--
-- Idempotent so air re-running it is safe.

INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc) VALUES
    ('semos-workspace', 'cdm_editor', 'en',    '{}'::jsonb,
        'Workspace app tile: CDM Editor (/home3/cdm)'),
    ('semos-workspace', 'cdm_editor', 'zh-cn',
        '{"label":"智能编辑器","description":"以语义方式撰写文档，由 Typst 模板负责排版。"}'::jsonb,
        'Workspace app tile: CDM Editor (/home3/cdm)')
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- Same [system].access_roles set the existing workspace tiles carry, so every
-- user who can see the other six can see this one. Scoped to this entry and to
-- rows that have no access_role yet, so admin edits are preserved on re-run.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'semos-workspace'
   AND entry_key = 'cdm_editor'
   AND access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'semos-workspace' AND entry_key = 'cdm_editor';
