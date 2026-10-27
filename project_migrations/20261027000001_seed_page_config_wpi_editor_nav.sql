-- +goose Up
-- Seed kb.page_config rows for the new /development NavRail node
-- Tools => WPI Editor (a copy of Tools => Flow), so it is configurable from
-- System Admin -> Page Config.
--
-- Same rules as 20260930000002_seed_page_config_development_missing_nav.sql:
-- labels are Paraglide messages, so content stays '{}'; the row inherits its
-- parent's zh-cn access_role (a null access_role would suspend the entry).
-- Idempotent: inserts skip existing rows and the access_role fill only touches
-- rows that still have none.

INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc, created_by, updated_by)
SELECT 'development', 'wpi-editor', l.language, '{}'::jsonb,
       '/development => Tools => WPI Editor',
       'migration-20261027000001', 'migration-20261027000001'
  FROM (VALUES ('en'), ('zh-cn')) AS l(language)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM kb.page_config parent
 WHERE pc.page_key = 'development' AND pc.entry_key = 'wpi-editor'
   AND parent.page_key = 'development' AND parent.entry_key = 'tools'
   AND parent.language = 'zh-cn'
   AND pc.access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development'
   AND created_by = 'migration-20261027000001';
