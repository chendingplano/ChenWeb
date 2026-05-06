-- +goose Up
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_type TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS source_text TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS source_line_spans JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_original TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_en TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_subject TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS provision_keywords JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS need_verify BOOLEAN;

UPDATE kb.provisions
SET provision_type = COALESCE(provision_type, public_info->>'provision_type')
WHERE public_info ? 'provision_type';

UPDATE kb.provisions
SET source_line_spans = public_info->'source_line_spans'
WHERE public_info ? 'source_line_spans'
  AND source_line_spans = '[]'::jsonb;

UPDATE kb.provisions
SET provision_original = COALESCE(provision_original, public_info->>'provision_original')
WHERE public_info ? 'provision_original';

UPDATE kb.provisions
SET provision_en = COALESCE(provision_en, public_info->>'provision_en')
WHERE public_info ? 'provision_en';

UPDATE kb.provisions
SET provision_subject = COALESCE(provision_subject, prov_subject);

UPDATE kb.provisions
SET provision_keywords = prov_keywords
WHERE prov_keywords IS NOT NULL
  AND provision_keywords = '[]'::jsonb;

UPDATE kb.provisions
SET confidence = COALESCE(confidence, prov_conf);

UPDATE kb.provisions
SET need_verify = COALESCE(need_verify, (public_info->>'need_verify')::boolean)
WHERE public_info ? 'need_verify';

DROP INDEX IF EXISTS kb.idx_kb_provisions_keywords_gin;

CREATE INDEX IF NOT EXISTS idx_kb_provisions_keywords_gin
    ON kb.provisions USING GIN (provision_keywords);

ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_name;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_context;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS categories;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_conf;

-- +goose Down
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_conf DOUBLE PRECISION;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS categories JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE kb.provisions
SET prov_conf = COALESCE(prov_conf, confidence);

DROP INDEX IF EXISTS kb.idx_kb_provisions_keywords_gin;

CREATE INDEX IF NOT EXISTS idx_kb_provisions_keywords_gin
    ON kb.provisions USING GIN (prov_keywords);

ALTER TABLE kb.provisions DROP COLUMN IF EXISTS need_verify;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS confidence;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_keywords;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_subject;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_en;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_original;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS source_line_spans;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS source_text;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS provision_type;
