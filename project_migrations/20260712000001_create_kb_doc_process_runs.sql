-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.doc_process_runs (
    id              BIGSERIAL    PRIMARY KEY,
    record_id       BIGINT       NOT NULL,
    event_id        VARCHAR(128),
    mode            TEXT         NOT NULL CHECK (mode IN ('auto', 'dev')),
    run_number      INT          NOT NULL,
    processors      JSONB        NOT NULL,
    parameters      JSONB        NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT         NOT NULL DEFAULT 'running'
                                  CHECK (status IN ('running', 'success', 'failed', 'stopped')),
    error_message   TEXT,
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    start_time      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    end_time        TIMESTAMPTZ,
    CONSTRAINT fk_doc_process_runs_event
        FOREIGN KEY (event_id) REFERENCES kb.events (event_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_doc_process_runs_record
    ON kb.doc_process_runs (record_id);
CREATE INDEX IF NOT EXISTS idx_kb_doc_process_runs_event
    ON kb.doc_process_runs (event_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_doc_process_runs_record_run_number
    ON kb.doc_process_runs (record_id, run_number);

ALTER TABLE kb.doc_proc_logs ADD COLUMN IF NOT EXISTS run_id BIGINT REFERENCES kb.doc_process_runs (id);
CREATE INDEX IF NOT EXISTS idx_kb_doc_proc_logs_run_id ON kb.doc_proc_logs (run_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_doc_proc_logs_run_id;
ALTER TABLE kb.doc_proc_logs DROP COLUMN IF EXISTS run_id;

DROP INDEX IF EXISTS idx_kb_doc_process_runs_record_run_number;
DROP INDEX IF EXISTS idx_kb_doc_process_runs_event;
DROP INDEX IF EXISTS idx_kb_doc_process_runs_record;
DROP TABLE IF EXISTS kb.doc_process_runs;
