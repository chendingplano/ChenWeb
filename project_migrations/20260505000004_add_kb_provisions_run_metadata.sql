-- +goose Up
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS num_blocks INTEGER;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS num_provisions INTEGER;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS time_per_provision DOUBLE PRECISION;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS model_name TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prompt_name TEXT;

ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_subject;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_keywords;

-- +goose Down
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_subject TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_keywords JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE kb.provisions
SET prov_subject = COALESCE(prov_subject, provision_subject);

UPDATE kb.provisions
SET prov_keywords = provision_keywords
WHERE provision_keywords IS NOT NULL
  AND prov_keywords = '[]'::jsonb;

ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prompt_name;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS model_name;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS time_per_provision;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS num_provisions;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS num_blocks;
