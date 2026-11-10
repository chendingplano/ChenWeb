-- +goose Up
-- The hard-coded Resources > Documents entries (docs-users-manual, docs-development) were replaced by
-- document groups (kb.doc_groups), whose menu items are docgroup-<key>. Their page_config rows now match
-- no menu entry and only trigger an "unrecognized nav entry" warning. Visibility and labels of the groups
-- come from kb.doc_groups (access_role, label), so the rows are dropped.
DELETE FROM kb.page_config
WHERE page_key = 'resources'
  AND entry_key IN ('docs-users-manual', 'docs-development');

-- +goose Down
-- The rows were obsolete seed data; nothing to restore.
SELECT 1;
