-- +goose Up
-- Existing rows contain frontend/backend file counts and cannot be converted
-- to line totals. Reset history so collection starts with a valid line baseline.
TRUNCATE TABLE kb.code_monitor;

ALTER TABLE kb.code_monitor
    DROP COLUMN frontend_files,
    DROP COLUMN backend_files,
    ADD COLUMN frontend_lines BIGINT NOT NULL,
    ADD COLUMN backend_lines BIGINT NOT NULL;

-- +goose Down
TRUNCATE TABLE kb.code_monitor;

ALTER TABLE kb.code_monitor
    DROP COLUMN frontend_lines,
    DROP COLUMN backend_lines,
    ADD COLUMN frontend_files BIGINT NOT NULL,
    ADD COLUMN backend_files BIGINT NOT NULL;
