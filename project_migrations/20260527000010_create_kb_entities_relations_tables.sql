-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.entities (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    entity_id TEXT,
    language TEXT,
    entity TEXT,
    entity_en TEXT,
    entity_type TEXT,
    entity_type_en TEXT,
    aliases JSONB,
    aliases_en JSONB,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    source_line_spans JSONB,
    confidence DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_entities_input_record_id ON kb.entities (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_entities_entity_id ON kb.entities (entity_id);

CREATE TABLE IF NOT EXISTS kb.relations (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    relation_id TEXT,
    language TEXT,
    subject TEXT,
    subject_en TEXT,
    predicate TEXT,
    predicate_en TEXT,
    object TEXT,
    object_en TEXT,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    source_line_spans JSONB,
    confidence DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_relations_input_record_id ON kb.relations (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_relations_relation_id ON kb.relations (relation_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.entity_search_document(
    entity TEXT,
    entity_en TEXT,
    entity_type TEXT,
    entity_type_en TEXT,
    aliases JSONB,
    aliases_en JSONB,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(entity, ''),
        COALESCE(entity_en, ''),
        COALESCE(entity_type, ''),
        COALESCE(entity_type_en, ''),
        kb.search_jsonb_array_text(aliases),
        kb.search_jsonb_array_text(aliases_en),
        COALESCE(desc_text, ''),
        COALESCE(desc_text_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en)
    ));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_entity_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.entity_search_document(
        NEW.entity, NEW.entity_en,
        NEW.entity_type, NEW.entity_type_en,
        NEW.aliases, NEW.aliases_en,
        NEW.desc_text, NEW.desc_text_en,
        NEW.keywords, NEW.keywords_en
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_entity_search_columns ON kb.entities;
CREATE TRIGGER trg_refresh_entity_search_columns
BEFORE INSERT OR UPDATE OF
    entity, entity_en,
    entity_type, entity_type_en,
    aliases, aliases_en,
    desc_text, desc_text_en,
    keywords, keywords_en
ON kb.entities
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_entity_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_entities_search_vector
    ON kb.entities USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.relation_search_document(
    subject TEXT,
    subject_en TEXT,
    predicate TEXT,
    predicate_en TEXT,
    object TEXT,
    object_en TEXT,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(subject, ''),
        COALESCE(subject_en, ''),
        COALESCE(predicate, ''),
        COALESCE(predicate_en, ''),
        COALESCE(object, ''),
        COALESCE(object_en, ''),
        COALESCE(desc_text, ''),
        COALESCE(desc_text_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en)
    ));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_relation_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.relation_search_document(
        NEW.subject, NEW.subject_en,
        NEW.predicate, NEW.predicate_en,
        NEW.object, NEW.object_en,
        NEW.desc_text, NEW.desc_text_en,
        NEW.keywords, NEW.keywords_en
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_relation_search_columns ON kb.relations;
CREATE TRIGGER trg_refresh_relation_search_columns
BEFORE INSERT OR UPDATE OF
    subject, subject_en,
    predicate, predicate_en,
    object, object_en,
    desc_text, desc_text_en,
    keywords, keywords_en
ON kb.relations
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_relation_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_relations_search_vector
    ON kb.relations USING GIN (search_vector);

CREATE TABLE IF NOT EXISTS kb.search_artifacts_entity PARTITION OF kb.search_artifacts
    FOR VALUES IN ('entity');

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_entity_search_vector
    ON kb.search_artifacts_entity USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_entity_record
    ON kb.search_artifacts_entity (input_record_id);

CREATE TABLE IF NOT EXISTS kb.search_artifacts_relation PARTITION OF kb.search_artifacts
    FOR VALUES IN ('relation');

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_relation_search_vector
    ON kb.search_artifacts_relation USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_relation_record
    ON kb.search_artifacts_relation (input_record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_search_artifacts_relation_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_relation_search_vector;
DROP TABLE IF EXISTS kb.search_artifacts_relation;

DROP INDEX IF EXISTS idx_kb_search_artifacts_entity_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_entity_search_vector;
DROP TABLE IF EXISTS kb.search_artifacts_entity;

DROP INDEX IF EXISTS idx_kb_relations_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_relation_search_columns ON kb.relations;
DROP FUNCTION IF EXISTS kb.refresh_relation_search_columns();
DROP FUNCTION IF EXISTS kb.relation_search_document(TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB);

DROP INDEX IF EXISTS idx_kb_entities_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_entity_search_columns ON kb.entities;
DROP FUNCTION IF EXISTS kb.refresh_entity_search_columns();
DROP FUNCTION IF EXISTS kb.entity_search_document(TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, TEXT, TEXT, JSONB, JSONB);

DROP INDEX IF EXISTS idx_kb_relations_relation_id;
DROP INDEX IF EXISTS idx_kb_relations_input_record_id;
DROP TABLE IF EXISTS kb.relations;

DROP INDEX IF EXISTS idx_kb_entities_entity_id;
DROP INDEX IF EXISTS idx_kb_entities_input_record_id;
DROP TABLE IF EXISTS kb.entities;
