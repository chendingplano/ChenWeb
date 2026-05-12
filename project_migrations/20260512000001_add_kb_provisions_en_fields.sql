-- +goose Up
ALTER TABLE kb.provisions RENAME COLUMN provision_original TO provision;

ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_name_en          TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_subject_en  TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_desc_en          TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_context_en       TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_keywords_en JSONB;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS category_paths_en     JSONB;

-- +goose Down
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS category_paths_en;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_keywords_en;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_context_en;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_desc_en;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_subject_en;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_name_en;

ALTER TABLE kb.provisions RENAME COLUMN provision TO provision_original;
