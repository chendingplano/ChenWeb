-- +goose Up
ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS log_loc TEXT;

-- +goose Down
ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP COLUMN IF EXISTS log_loc;
