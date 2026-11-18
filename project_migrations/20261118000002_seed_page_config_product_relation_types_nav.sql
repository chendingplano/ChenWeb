-- +goose Up
-- Nav entry for System Admin -> Database Maintenance -> Product Relation Types.
-- Labels are Paraglide messages, so content stays '{}'. The new row inherits the parent's
-- access_role so seeding changes nobody's menu (a null access_role would suspend the page).
-- Idempotent.
INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc, created_by, updated_by)
SELECT 'development', 'sysadmin-db-product-relation-types', l.language, '{}'::jsonb,
       '/development => System Admin => DB Maintenance => Product Relation Types',
       'migration-20261118000002', 'migration-20261118000002'
  FROM (VALUES ('en'), ('zh-cn')) AS l(language)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM kb.page_config parent
 WHERE parent.page_key = 'development' AND parent.entry_key = 'sysadmin-db'
   AND parent.language = 'zh-cn'
   AND pc.page_key = 'development' AND pc.entry_key = 'sysadmin-db-product-relation-types'
   AND pc.access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development' AND entry_key = 'sysadmin-db-product-relation-types'
   AND created_by = 'migration-20261118000002';
