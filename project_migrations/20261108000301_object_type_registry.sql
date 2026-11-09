-- +goose Up
CREATE TABLE IF NOT EXISTS kb.object_types (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    legacy_code TEXT UNIQUE,
    status TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('approved', 'proposed')),
    raw_value TEXT NOT NULL DEFAULT '',
    source_record_id BIGINT NOT NULL DEFAULT 0,
    source_run_id BIGINT NOT NULL DEFAULT 0,
    source_processor TEXT NOT NULL DEFAULT '',
    source_model TEXT NOT NULL DEFAULT '',
    source_prompt TEXT NOT NULL DEFAULT '',
    source_spans JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(source_spans) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    approved_at TIMESTAMPTZ,
    approved_by TEXT NOT NULL DEFAULT ''
);

INSERT INTO kb.object_types (name, legacy_code, status, raw_value, approved_at, approved_by)
VALUES ('equipment','eq','approved','equipment',NOW(),'migration'),
       ('material','mat','approved','material',NOW(),'migration'),
       ('system','sys','approved','system',NOW(),'migration'),
       ('process','proc','approved','process',NOW(),'migration'),
       ('organization','org','approved','organization',NOW(),'migration'),
       ('person','per','approved','person',NOW(),'migration'),
       ('place','loc','approved','place',NOW(),'migration'),
       ('document','doc','approved','document',NOW(),'migration'),
       ('concept','con','approved','concept',NOW(),'migration'),
       ('other','oth','approved','other',NOW(),'migration')
ON CONFLICT (name) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_kb_object_types_status ON kb.object_types(status, name);

-- Keep Go normalizeObjectToken and this normalization aligned. Legacy codes
-- are aliases, not additional types. Original values remain available for review.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.normalize_object_type(raw TEXT) RETURNS TEXT
LANGUAGE SQL IMMUTABLE AS $$
    SELECT regexp_replace(btrim(regexp_replace(lower(COALESCE(raw,'')), '[[:space:]._/\-]+', ' ', 'g')), ' +', '_', 'g')
$$;
-- +goose StatementEnd

INSERT INTO kb.object_types (name, raw_value, source_record_id, source_processor, source_spans)
SELECT DISTINCT ON (normalized_name) normalized_name, object_type, source_record_id, artifact_type, COALESCE(source_line_spans,'[]'::jsonb)
FROM (
    SELECT ao.*, COALESCE(t.name,kb.normalize_object_type(ao.object_type)) AS normalized_name
    FROM kb.artifact_objects ao LEFT JOIN kb.object_types t ON t.legacy_code = kb.normalize_object_type(ao.object_type)
) observed
WHERE normalized_name <> ''
ORDER BY normalized_name, id
ON CONFLICT (name) DO NOTHING;

INSERT INTO kb.object_types (name, raw_value, source_processor)
SELECT DISTINCT ON (normalized_name) normalized_name, object_type, 'existing_object_node'
FROM (
    SELECT n.*, COALESCE(t.name,kb.normalize_object_type(n.object_type)) AS normalized_name
    FROM kb.object_nodes n LEFT JOIN kb.object_types t ON t.legacy_code = kb.normalize_object_type(n.object_type)
) observed
WHERE normalized_name <> ''
ORDER BY normalized_name, id
ON CONFLICT (name) DO NOTHING;

-- A database boundary protects all writers, including metrics, inventory,
-- human object creation, and future processors. No registry cache is needed.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.register_object_type() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    raw TEXT;
    canonical TEXT;
    record_id BIGINT := 0;
    run_id BIGINT := 0;
    processor TEXT := 'object_node';
    spans JSONB := '[]'::jsonb;
BEGIN
    raw := COALESCE(NEW.ext_info->>'object_type_raw', NEW.object_type, '');
    -- A type edit usually carries forward ext_info unchanged. Preserve an
    -- explicitly supplied raw value, but never reuse the previous type's raw
    -- spelling when the caller changed only object_type.
    IF TG_OP = 'UPDATE' THEN
        IF NEW.object_type IS DISTINCT FROM OLD.object_type
           AND NEW.ext_info->>'object_type_raw' IS NOT DISTINCT FROM OLD.ext_info->>'object_type_raw' THEN
            raw := COALESCE(NEW.object_type, '');
        END IF;
    END IF;
    canonical := COALESCE(NULLIF(kb.normalize_object_type(NEW.object_type),''),'other');
    SELECT name INTO canonical FROM kb.object_types WHERE name = canonical OR legacy_code = canonical ORDER BY (name = canonical) DESC LIMIT 1;
    IF canonical IS NULL THEN
        canonical := COALESCE(NULLIF(kb.normalize_object_type(NEW.object_type),''),'other');
    END IF;
    IF COALESCE(NEW.ext_info->>'source_run_id','') ~ '^[0-9]+$' THEN
        run_id := (NEW.ext_info->>'source_run_id')::BIGINT;
    END IF;
    IF TG_TABLE_NAME = 'artifact_objects' THEN
        record_id := COALESCE(NULLIF(NEW.source_record_id,0),NEW.input_record_id,0);
        processor := NEW.artifact_type;
        spans := COALESCE(NEW.source_line_spans,'[]'::jsonb);
    ELSE
        IF COALESCE(NEW.ext_info->>'source_record_id','') ~ '^[0-9]+$' THEN
            record_id := (NEW.ext_info->>'source_record_id')::BIGINT;
        END IF;
        processor := COALESCE(NULLIF(NEW.ext_info->>'source_processor',''),'object_node');
        IF jsonb_typeof(NEW.ext_info->'source_line_spans') = 'array' THEN
            spans := NEW.ext_info->'source_line_spans';
        END IF;
    END IF;
    INSERT INTO kb.object_types (name,raw_value,source_record_id,source_run_id,source_processor,source_spans)
    VALUES (canonical,raw,record_id,run_id,processor,spans)
    ON CONFLICT (name) DO NOTHING;
    NEW.object_type := canonical;
    NEW.ext_info := COALESCE(NEW.ext_info,'{}'::jsonb) || jsonb_build_object('object_type_raw',raw);
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS register_object_type ON kb.artifact_objects;
CREATE TRIGGER register_object_type BEFORE INSERT OR UPDATE OF object_type ON kb.artifact_objects
FOR EACH ROW EXECUTE FUNCTION kb.register_object_type();
DROP TRIGGER IF EXISTS register_object_type ON kb.object_nodes;
CREATE TRIGGER register_object_type BEFORE INSERT OR UPDATE OF object_type ON kb.object_nodes
FOR EACH ROW EXECUTE FUNCTION kb.register_object_type();

-- Canonicalize existing spellings without dropping references or provenance.
UPDATE kb.artifact_objects SET object_type = object_type;
UPDATE kb.object_nodes SET object_type = object_type;

-- +goose Down
DROP TRIGGER IF EXISTS register_object_type ON kb.object_nodes;
DROP TRIGGER IF EXISTS register_object_type ON kb.artifact_objects;
DROP FUNCTION IF EXISTS kb.register_object_type();
DROP FUNCTION IF EXISTS kb.normalize_object_type(TEXT);
DROP TABLE IF EXISTS kb.object_types;
