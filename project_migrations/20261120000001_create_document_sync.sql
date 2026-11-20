-- +goose Up
ALTER TABLE kb.inputs ADD COLUMN IF NOT EXISTS doc_origin TEXT NOT NULL DEFAULT 'local'
    CHECK (doc_origin IN ('local', 'remote'));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.keep_remote_doc_origin() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.doc_origin = 'remote' AND NEW.doc_origin <> 'remote' THEN
        RAISE EXCEPTION 'A remote document cannot become local';
    END IF;
    RETURN NEW;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER keep_remote_doc_origin BEFORE UPDATE OF doc_origin ON kb.inputs
FOR EACH ROW EXECUTE FUNCTION kb.keep_remote_doc_origin();

CREATE INDEX IF NOT EXISTS kb_inputs_sync_window ON kb.inputs (modify_time, id);
CREATE INDEX IF NOT EXISTS kb_inputs_sync_md5 ON kb.inputs (lower(btrim(md5))) WHERE md5 IS NOT NULL;

CREATE TABLE IF NOT EXISTS kb.doc_sync_marks (
    input_id BIGINT PRIMARY KEY REFERENCES kb.inputs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS kb.doc_sync_inputs (
    site TEXT NOT NULL,
    source_id BIGINT NOT NULL,
    input_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    source_owner TEXT,
    md5 TEXT NOT NULL,
    PRIMARY KEY (site, source_id),
    UNIQUE (site, input_id)
);

CREATE TABLE IF NOT EXISTS kb.doc_sync_applied (
    site TEXT NOT NULL,
    source_id BIGINT NOT NULL,
    processor TEXT NOT NULL,
    digest TEXT NOT NULL,
    source_time TIMESTAMPTZ NOT NULL,
    source_status JSONB,
    files JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (site, source_id, processor)
);

CREATE TABLE IF NOT EXISTS kb.doc_sync_row_map (
    site TEXT NOT NULL,
    table_name TEXT NOT NULL,
    source_id BIGINT NOT NULL,
    local_id BIGINT NOT NULL,
    local_input_id BIGINT REFERENCES kb.inputs(id) ON DELETE CASCADE,
    PRIMARY KEY (site, table_name, source_id)
);
CREATE INDEX IF NOT EXISTS doc_sync_row_map_input_idx
    ON kb.doc_sync_row_map (site, table_name, local_input_id, local_id);

CREATE TABLE IF NOT EXISTS kb.doc_sync_queue (
    id BIGSERIAL PRIMARY KEY,
    site TEXT NOT NULL,
    source_id BIGINT NOT NULL,
    processor TEXT NOT NULL,
    item JSONB NOT NULL,
    last_error TEXT,
    UNIQUE (site, source_id, processor)
);

CREATE TABLE IF NOT EXISTS kb.doc_sync_transfers (
    site TEXT PRIMARY KEY,
    run_id BIGINT NOT NULL,
    tar_number INTEGER NOT NULL,
    request JSONB NOT NULL,
    imported BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE IF NOT EXISTS kb.doc_sync_events (
    id BIGSERIAL PRIMARY KEY,
    site TEXT NOT NULL,
    run_id BIGINT NOT NULL,
    event TEXT NOT NULL,
    source_id BIGINT,
    input_id BIGINT,
    processor TEXT,
    detail JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS doc_sync_events_run_idx ON kb.doc_sync_events (run_id, id);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_sync_events;
DROP TABLE IF EXISTS kb.doc_sync_transfers;
DROP TABLE IF EXISTS kb.doc_sync_queue;
DROP TABLE IF EXISTS kb.doc_sync_row_map;
DROP TABLE IF EXISTS kb.doc_sync_applied;
DROP TABLE IF EXISTS kb.doc_sync_inputs;
DROP TABLE IF EXISTS kb.doc_sync_marks;
DROP INDEX IF EXISTS kb.kb_inputs_sync_md5;
DROP INDEX IF EXISTS kb.kb_inputs_sync_window;
DROP TRIGGER IF EXISTS keep_remote_doc_origin ON kb.inputs;
DROP FUNCTION IF EXISTS kb.keep_remote_doc_origin();
ALTER TABLE kb.inputs DROP COLUMN IF EXISTS doc_origin;
