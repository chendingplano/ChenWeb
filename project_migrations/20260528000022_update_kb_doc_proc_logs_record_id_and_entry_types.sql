-- +goose Up
ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS record_id BIGINT;

ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP CONSTRAINT IF EXISTS doc_proc_logs_entry_type_check;

ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD CONSTRAINT doc_proc_logs_entry_type_check
    CHECK (entry_type IN ('llm_call', 'doc_proc_summary', 'generate_summary', 'generate_summary_finish'));

CREATE INDEX IF NOT EXISTS idx_kb_doc_proc_logs_record_id ON kb.doc_proc_logs (record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_doc_proc_logs_record_id;

ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP CONSTRAINT IF EXISTS doc_proc_logs_entry_type_check;

ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD CONSTRAINT doc_proc_logs_entry_type_check
    CHECK (entry_type IN ('llm_call', 'doc_proc_summary'));

ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP COLUMN IF EXISTS record_id;
