-- +goose Up
-- Restrict to the 'admin' role only:
--   * Applications sub-menu items Installed, Browse, Configure, Generate Doc
--     (Document Review and Product Review keep their current roles);
--   * the System Admin root item, which hides its whole subtree.
UPDATE kb.page_config
   SET access_role = '["admin"]'::jsonb,
       updated_at  = NOW(),
       updated_by  = 'migration-20260930000004'
 WHERE page_key = 'development'
   AND entry_key IN ('apps-installed', 'apps-browse', 'apps-configure', 'apps-generate-doc',
                     'system-admin');

-- +goose Down
-- Restore the role set these rows carried before this migration.
UPDATE kb.page_config
   SET access_role = '["admin","root","guest","dev","k_engineer","trial"]'::jsonb
 WHERE page_key = 'development'
   AND entry_key IN ('apps-installed', 'apps-browse', 'apps-configure', 'apps-generate-doc',
                     'system-admin');
