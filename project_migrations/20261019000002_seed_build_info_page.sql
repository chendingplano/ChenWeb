-- +goose Up
INSERT INTO kb.page_config (
    page_key, entry_key, language, content, entry_desc, access_role, created_by, updated_by
)
SELECT 'development', 'sysadmin-system-build-info', locale.language, '{}'::jsonb,
       '/development => System Admin => System => Build Info', parent.access_role,
       'migration-20261019000002', 'migration-20261019000002'
  FROM (VALUES ('en'), ('zh-cn')) AS locale(language)
  JOIN kb.page_config parent
    ON parent.page_key = 'development'
   AND parent.entry_key = 'sysadmin-system'
   AND parent.language = locale.language
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development'
   AND entry_key = 'sysadmin-system-build-info'
   AND created_by = 'migration-20261019000002';
