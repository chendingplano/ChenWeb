-- +goose Up
-- Restrict Dashboard => LLM Activities on /development to the 'admin' role only.
UPDATE kb.page_config
   SET access_role = '["admin"]'::jsonb,
       updated_at  = NOW(),
       updated_by  = 'migration-20260930000005'
 WHERE page_key = 'development'
   AND entry_key = 'llm-activities';

-- +goose Down
-- Restore the role set this row carried before this migration.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development'
   AND entry_key = 'llm-activities';
