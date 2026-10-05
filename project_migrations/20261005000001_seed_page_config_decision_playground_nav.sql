-- +goose Up
-- Seed kb.page_config rows for the new /development NavRail nodes
-- System Admin => LLM => Decision Models (=> Playground), requirement
-- 2026100502-rqmt, so they are configurable from System Admin -> Page Config.
--
-- Same rules as 20260930000002_seed_page_config_development_missing_nav.sql:
-- labels are Paraglide messages, so content stays '{}'; each row inherits its
-- parent's zh-cn access_role (a null access_role would suspend the entry).
-- Idempotent: inserts skip existing rows and the access_role fill only touches
-- rows that still have none.

INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc, created_by, updated_by)
SELECT 'development', s.entry_key, l.language, '{}'::jsonb, s.entry_desc,
       'migration-20261005000001', 'migration-20261005000001'
  FROM (VALUES
        ('sysadmin-llm-decision-models', '/development => System Admin => LLM => Decision Models'),
        ('sysadmin-llm-decision-models-playground', '/development => System Admin => LLM => Decision Models => Playground')
       ) AS s(entry_key, entry_desc)
 CROSS JOIN (VALUES ('en'), ('zh-cn')) AS l(language)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM kb.page_config parent
 WHERE pc.page_key = 'development' AND pc.entry_key = 'sysadmin-llm-decision-models'
   AND parent.page_key = 'development' AND parent.entry_key = 'sysadmin-llm'
   AND parent.language = 'zh-cn'
   AND pc.access_role IS NULL;

UPDATE kb.page_config pc
   SET access_role = parent.access_role
  FROM kb.page_config parent
 WHERE pc.page_key = 'development' AND pc.entry_key = 'sysadmin-llm-decision-models-playground'
   AND parent.page_key = 'development' AND parent.entry_key = 'sysadmin-llm-decision-models'
   AND parent.language = 'zh-cn'
   AND pc.access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config
 WHERE page_key = 'development'
   AND created_by = 'migration-20261005000001';
