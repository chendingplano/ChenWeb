-- +goose Up
-- entry_desc is admin-facing metadata describing what a configurable entry is
-- (e.g. "Wiki sidebar menu item", "Workspace masthead kicker"), shown in the
-- admin editor. It is language-independent; the admin UI writes the same value
-- to every language row, and the default-language row is authoritative.
ALTER TABLE kb.page_config
    ADD COLUMN IF NOT EXISTS entry_desc TEXT;

-- Backfill descriptions for the seeded entries.
UPDATE kb.page_config SET entry_desc = 'Wiki sidebar menu item'
 WHERE page_key = 'home3-knowledge' AND (entry_desc IS NULL OR entry_desc = '');

UPDATE kb.page_config SET entry_desc = 'Workspace masthead: kicker'
 WHERE page_key = 'semos-workspace' AND entry_key = 'ws-kicker';
UPDATE kb.page_config SET entry_desc = 'Workspace masthead: banner title'
 WHERE page_key = 'semos-workspace' AND entry_key = 'ws-banner-title';
UPDATE kb.page_config SET entry_desc = 'Workspace masthead: banner subtitle'
 WHERE page_key = 'semos-workspace' AND entry_key = 'ws-banner-subtitle';
UPDATE kb.page_config SET entry_desc = 'Workspace masthead: announcements section'
 WHERE page_key = 'semos-workspace' AND entry_key = 'ws-announcements';
UPDATE kb.page_config SET entry_desc = 'Workspace app tile'
 WHERE page_key = 'semos-workspace'
   AND entry_key IN ('knowledge_base', 'chat', 'search', 'doc_reviews', 'workflows', 'agents');

-- +goose Down
ALTER TABLE kb.page_config DROP COLUMN IF EXISTS entry_desc;
