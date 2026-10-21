-- +goose Up
CREATE TABLE IF NOT EXISTS kb.ex_summaries (
    id                    BIGSERIAL PRIMARY KEY,
    summary_id            TEXT NOT NULL UNIQUE,
    input_record_id       BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    algorithm             TEXT NOT NULL CHECK (btrim(algorithm) <> ''),
    summary_level         INTEGER NOT NULL DEFAULT 0 CHECK (summary_level = 0),
    summary_seq_no        INTEGER NOT NULL CHECK (summary_seq_no > 0),
    source_line_spans     JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_sentence_spans JSONB NOT NULL DEFAULT '[]'::jsonb,
    child_summary_ids     JSONB NOT NULL DEFAULT '[]'::jsonb,
    keywords              JSONB NOT NULL DEFAULT '[]'::jsonb,
    keywords_en           JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary_text          TEXT NOT NULL DEFAULT '',
    summary_text_en       TEXT NOT NULL DEFAULT '',
    language              TEXT NOT NULL DEFAULT '',
    extraction_config     JSONB NOT NULL DEFAULT '{}'::jsonb,
    search_document       TEXT NOT NULL DEFAULT '',
    search_vector         TSVECTOR NOT NULL DEFAULT ''::tsvector,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_ex_summaries_record_algorithm_chunk
        UNIQUE (input_record_id, algorithm, summary_level, summary_seq_no)
);

CREATE INDEX IF NOT EXISTS idx_kb_ex_summaries_search_vector
    ON kb.ex_summaries USING GIN (search_vector);

-- Same current column contract as kb.summaries. Reuse its maintained trigger function.
DROP TRIGGER IF EXISTS trg_refresh_ex_summary_search_columns ON kb.ex_summaries;
CREATE TRIGGER trg_refresh_ex_summary_search_columns
BEFORE INSERT OR UPDATE OF summary_text, summary_text_en, keywords, keywords_en, child_summary_ids, language
ON kb.ex_summaries
FOR EACH ROW EXECUTE FUNCTION kb.refresh_summary_search_columns();

-- +goose Down
DROP TRIGGER IF EXISTS trg_refresh_ex_summary_search_columns ON kb.ex_summaries;
DROP TABLE IF EXISTS kb.ex_summaries;
