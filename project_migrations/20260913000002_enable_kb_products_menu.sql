-- +goose Up
-- Products is a supported Knowledge System viewer. Restore its page-config
-- visibility so the frontend overlay does not hide the hardcoded menu entry.
UPDATE kb.page_config
   SET accessible = TRUE,
       enabled = TRUE,
       updated_at = NOW()
 WHERE page_key = 'home3-knowledge'
   AND entry_key = 'kb-products';

-- +goose Down
UPDATE kb.page_config
   SET accessible = FALSE,
       updated_at = NOW()
 WHERE page_key = 'home3-knowledge'
   AND entry_key = 'kb-products';
