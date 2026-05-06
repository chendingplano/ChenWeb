-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.provisions (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    extract_id TEXT NOT NULL DEFAULT '',
    input_filename TEXT NOT NULL DEFAULT ''
);

ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_id INTEGER;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_name TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_subject TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_desc TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_context TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_keywords JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS category_paths JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS location_type TEXT;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS prov_conf DOUBLE PRECISION;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS is_explicit BOOLEAN;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS create_time TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS public_info JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS private_info JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS notes TEXT NOT NULL DEFAULT '';
ALTER TABLE kb.provisions ADD COLUMN IF NOT EXISTS error_msg TEXT NOT NULL DEFAULT '';

UPDATE kb.provisions p
SET prov_id = ranked.prov_id
FROM (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY input_record_id ORDER BY id) AS prov_id
    FROM kb.provisions
    WHERE prov_id IS NULL
) ranked
WHERE p.id = ranked.id;

ALTER TABLE kb.provisions ALTER COLUMN prov_id SET NOT NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'uq_kb_provisions_input_prov_id'
          AND conrelid = 'kb.provisions'::regclass
    ) THEN
        ALTER TABLE kb.provisions
            ADD CONSTRAINT uq_kb_provisions_input_prov_id UNIQUE (input_record_id, prov_id);
    END IF;
END $$;
-- +goose StatementEnd

CREATE INDEX IF NOT EXISTS idx_kb_provisions_input_record_id
    ON kb.provisions(input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_extract_id
    ON kb.provisions(extract_id);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_status
    ON kb.provisions(status);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_keywords_gin
    ON kb.provisions USING GIN (prov_keywords);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_categories_gin
    ON kb.provisions USING GIN (category_paths);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_provisions_categories_gin;
DROP INDEX IF EXISTS kb.idx_kb_provisions_keywords_gin;
DROP INDEX IF EXISTS kb.idx_kb_provisions_status;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'uq_kb_provisions_input_prov_id'
          AND conrelid = 'kb.provisions'::regclass
    ) THEN
        ALTER TABLE kb.provisions
            DROP CONSTRAINT uq_kb_provisions_input_prov_id;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.provisions DROP COLUMN IF EXISTS error_msg;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS notes;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS private_info;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS public_info;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS modify_time;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS create_time;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS status;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS is_explicit;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_conf;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS location_type;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS category_paths;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_keywords;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_context;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_desc;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_subject;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_name;
ALTER TABLE kb.provisions DROP COLUMN IF EXISTS prov_id;
