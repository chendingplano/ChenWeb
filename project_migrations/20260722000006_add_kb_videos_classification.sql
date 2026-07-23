-- +goose Up
-- Enrich kb.videos with classification/triage metadata captured by the upload
-- dialog (System Admin → Resources → Videos). Additive and backward-compatible:
-- existing rows keep working and responses COALESCE missing values to safe
-- defaults ('' for text, 'draft' for status). `category`/`subcategory` are free
-- text (the dialog suggests existing values but does not constrain them);
-- `status` ∈ {draft, published, archived}; `video_type` is auto-detected from the
-- file (e.g. 'mp4') and user-editable.
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS keywords    TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS category    TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS subcategory TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS container   TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS status      VARCHAR(16) NOT NULL DEFAULT 'draft';
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS notes       TEXT;
ALTER TABLE kb.videos ADD COLUMN IF NOT EXISTS video_type  VARCHAR(32);

-- +goose Down
ALTER TABLE kb.videos DROP COLUMN IF EXISTS video_type;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS notes;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS status;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS container;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS subcategory;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS category;
ALTER TABLE kb.videos DROP COLUMN IF EXISTS keywords;
