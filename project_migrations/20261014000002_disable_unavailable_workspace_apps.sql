-- +goose Up
-- /semos/workspace resolves tile visibility and label overrides from
-- kb.page_config. Disable the three app rows there so the page's actual
-- configuration source hides them for every user.
UPDATE kb.page_config
   SET enabled = false,
       updated_at = NOW(),
       updated_by = 'migration-20261014000002'
 WHERE page_key = 'semos-workspace'
   AND entry_key IN ('workflows', 'agents', 'cdm_editor');

-- Keep the existing Chinese locale override in step with the site-config
-- default label for the renamed Development app.
UPDATE kb.page_config
   SET content = jsonb_set(content, '{label}', '"开发"'::jsonb, true),
       updated_at = NOW(),
       updated_by = 'migration-20261014000002'
 WHERE page_key = 'semos-workspace'
   AND entry_key = 'chat'
   AND language = 'zh-cn';

-- +goose Down
UPDATE kb.page_config
   SET enabled = true,
       updated_at = NOW(),
       updated_by = 'rollback-20261014000002'
 WHERE page_key = 'semos-workspace'
   AND entry_key IN ('workflows', 'agents', 'cdm_editor');

UPDATE kb.page_config
   SET content = jsonb_set(content, '{label}', '"对话知识库"'::jsonb, true),
       updated_at = NOW(),
       updated_by = 'rollback-20261014000002'
 WHERE page_key = 'semos-workspace'
   AND entry_key = 'chat'
   AND language = 'zh-cn';
