-- +goose Up
-- System Admin → Page Content moved under System Admin → System and renamed
-- "Page Config" (nav-rail.svelte). The entry_key (sysadmin-page-config) is
-- unchanged, so only the zh-cn label override needs updating. Only rewrites the
-- seeded default, so an admin-customised label is preserved.
UPDATE kb.page_config
SET content = jsonb_set(content, '{label}', '"页面配置"')
WHERE page_key = 'development'
  AND entry_key = 'sysadmin-page-config'
  AND language = 'zh-cn'
  AND content->>'label' = '页面内容';

-- +goose Down
UPDATE kb.page_config
SET content = jsonb_set(content, '{label}', '"页面内容"')
WHERE page_key = 'development'
  AND entry_key = 'sysadmin-page-config'
  AND language = 'zh-cn'
  AND content->>'label' = '页面配置';
