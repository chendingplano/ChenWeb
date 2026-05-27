-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.doc_proc_logs (
    id             BIGSERIAL PRIMARY KEY,
    call_reason    TEXT,
    doc_proc_name  TEXT NOT NULL,
    model_names    TEXT[],
    prompt_name    TEXT,
    entry_type     TEXT NOT NULL CHECK (entry_type IN ('llm_call', 'doc_proc_summary')),
    pass           INTEGER,
    llm_call_id    TEXT,
    activity_name  TEXT,
    artifact       JSONB,
    errors         TEXT,
    extra_info     JSONB,
    ms_used        BIGINT,
    create_time    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_doc_proc_logs_doc_proc_name ON kb.doc_proc_logs (doc_proc_name);
CREATE INDEX IF NOT EXISTS idx_kb_doc_proc_logs_entry_type    ON kb.doc_proc_logs (entry_type);
CREATE INDEX IF NOT EXISTS idx_kb_doc_proc_logs_create_time   ON kb.doc_proc_logs (create_time);
CREATE INDEX IF NOT EXISTS idx_kb_doc_proc_logs_llm_call_id   ON kb.doc_proc_logs (llm_call_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_doc_proc_logs_llm_call_id;
DROP INDEX IF EXISTS idx_kb_doc_proc_logs_create_time;
DROP INDEX IF EXISTS idx_kb_doc_proc_logs_entry_type;
DROP INDEX IF EXISTS idx_kb_doc_proc_logs_doc_proc_name;
DROP TABLE IF EXISTS kb.doc_proc_logs;
