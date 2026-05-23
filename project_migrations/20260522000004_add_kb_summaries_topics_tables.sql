-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.summaries (
    id                     BIGSERIAL    PRIMARY KEY,
    summary_id             TEXT         NOT NULL,
    input_record_id        BIGINT       NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    summary_level          INTEGER      NOT NULL DEFAULT 0,
    summary_seq_no         INTEGER      NOT NULL DEFAULT 0,
    source_line_spans      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    child_summary_ids      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    keywords               JSONB        NOT NULL DEFAULT '[]'::jsonb,
    keywords_en            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_paths         JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_paths_en      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_nodes         JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_path_items    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_path_items_en JSONB        NOT NULL DEFAULT '[]'::jsonb,
    summary_text           TEXT         NOT NULL DEFAULT '',
    summary_text_en        TEXT         NOT NULL DEFAULT '',
    search_document        TEXT         NOT NULL DEFAULT '',
    search_vector          TSVECTOR     NOT NULL DEFAULT ''::tsvector,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_summaries_summary_id UNIQUE (summary_id),
    CONSTRAINT uq_kb_summaries_record_level_seq UNIQUE (input_record_id, summary_level, summary_seq_no)
);

CREATE INDEX IF NOT EXISTS idx_kb_summaries_input_record_id
    ON kb.summaries (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_summaries_search_vector
    ON kb.summaries USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.summary_search_document(
    summary_text TEXT,
    summary_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB,
    child_summary_ids JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(summary_text, ''),
        COALESCE(summary_text_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en),
        kb.search_jsonb_array_text(child_summary_ids)
    ));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_summary_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.summary_search_document(
        NEW.summary_text,
        NEW.summary_text_en,
        NEW.keywords,
        NEW.keywords_en,
        NEW.category_paths,
        NEW.category_paths_en,
        NEW.child_summary_ids
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_summary_search_columns ON kb.summaries;
CREATE TRIGGER trg_refresh_summary_search_columns
BEFORE INSERT OR UPDATE OF
    summary_text, summary_text_en,
    keywords, keywords_en,
    category_paths, category_paths_en,
    child_summary_ids
ON kb.summaries
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_summary_search_columns();

CREATE TABLE IF NOT EXISTS kb.topics (
    id                     BIGSERIAL    PRIMARY KEY,
    topic_id               TEXT         NOT NULL,
    input_record_id        BIGINT       NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    topic_seq_no           INTEGER      NOT NULL DEFAULT 0,
    topic_type             TEXT         NOT NULL DEFAULT '',
    topic_type_en          TEXT         NOT NULL DEFAULT '',
    source_line_spans      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    keywords               JSONB        NOT NULL DEFAULT '[]'::jsonb,
    keywords_en            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_paths         JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_paths_en      JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_path_items    JSONB        NOT NULL DEFAULT '[]'::jsonb,
    category_path_items_en JSONB        NOT NULL DEFAULT '[]'::jsonb,
    topic_desc             TEXT         NOT NULL DEFAULT '',
    topic_desc_en          TEXT         NOT NULL DEFAULT '',
    search_document        TEXT         NOT NULL DEFAULT '',
    search_vector          TSVECTOR     NOT NULL DEFAULT ''::tsvector,
    created_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_topics_record_topic_id UNIQUE (input_record_id, topic_id),
    CONSTRAINT uq_kb_topics_record_seq UNIQUE (input_record_id, topic_seq_no)
);

CREATE INDEX IF NOT EXISTS idx_kb_topics_input_record_id
    ON kb.topics (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_topics_search_vector
    ON kb.topics USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.topic_search_document(
    topic_id TEXT,
    topic_type TEXT,
    topic_type_en TEXT,
    topic_desc TEXT,
    topic_desc_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(topic_id, ''),
        COALESCE(topic_type, ''),
        COALESCE(topic_type_en, ''),
        COALESCE(topic_desc, ''),
        COALESCE(topic_desc_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en)
    ));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_topic_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.topic_search_document(
        NEW.topic_id,
        NEW.topic_type,
        NEW.topic_type_en,
        NEW.topic_desc,
        NEW.topic_desc_en,
        NEW.keywords,
        NEW.keywords_en,
        NEW.category_paths,
        NEW.category_paths_en
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    NEW.updated_at := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_topic_search_columns ON kb.topics;
CREATE TRIGGER trg_refresh_topic_search_columns
BEFORE INSERT OR UPDATE OF
    topic_id, topic_type, topic_type_en,
    topic_desc, topic_desc_en,
    keywords, keywords_en,
    category_paths, category_paths_en
ON kb.topics
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_topic_search_columns();

-- +goose Down
DROP TRIGGER IF EXISTS trg_refresh_topic_search_columns ON kb.topics;
DROP FUNCTION IF EXISTS kb.refresh_topic_search_columns();
DROP FUNCTION IF EXISTS kb.topic_search_document(TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, JSONB, JSONB);
DROP INDEX IF EXISTS idx_kb_topics_search_vector;
DROP INDEX IF EXISTS idx_kb_topics_input_record_id;
DROP TABLE IF EXISTS kb.topics;

DROP TRIGGER IF EXISTS trg_refresh_summary_search_columns ON kb.summaries;
DROP FUNCTION IF EXISTS kb.refresh_summary_search_columns();
DROP FUNCTION IF EXISTS kb.summary_search_document(TEXT, TEXT, JSONB, JSONB, JSONB, JSONB, JSONB);
DROP INDEX IF EXISTS idx_kb_summaries_search_vector;
DROP INDEX IF EXISTS idx_kb_summaries_input_record_id;
DROP TABLE IF EXISTS kb.summaries;
