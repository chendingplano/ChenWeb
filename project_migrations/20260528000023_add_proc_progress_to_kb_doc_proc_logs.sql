-- +goose Up
ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS proc_progress TEXT;

-- +goose Down
ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP COLUMN IF EXISTS proc_progress;
