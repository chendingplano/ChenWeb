-- +goose Up
-- Add the System Admin → Keyword Normalization → Rewrite Rules branch.
INSERT INTO kb.page_config
    (page_key, entry_key, language, content, access_role, accessible, enabled, entry_desc)
VALUES
    ('development', 'sysadmin-keyword-normalization', 'en', '{"label":"Keyword Normalization"}'::jsonb, '["admin","root"]'::jsonb, true, true, 'Keyword normalization administration'),
    ('development', 'sysadmin-keyword-rewrite-rules', 'en', '{"label":"Rewrite Rules"}'::jsonb, '["admin","root"]'::jsonb, true, true, 'Keyword rewrite rule administration'),
    ('development', 'sysadmin-keyword-normalization', 'zh-cn', '{"label":"关键词规范化"}'::jsonb, '["admin","root"]'::jsonb, true, true, '关键词规范化管理'),
    ('development', 'sysadmin-keyword-rewrite-rules', 'zh-cn', '{"label":"重写规则"}'::jsonb, '["admin","root"]'::jsonb, true, true, '关键词重写规则管理')
ON CONFLICT (page_key, entry_key, language) DO UPDATE SET
    content = EXCLUDED.content, access_role = EXCLUDED.access_role,
    accessible = EXCLUDED.accessible, enabled = EXCLUDED.enabled,
    entry_desc = EXCLUDED.entry_desc;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development'
   AND entry_key IN ('sysadmin-keyword-normalization', 'sysadmin-keyword-rewrite-rules');
