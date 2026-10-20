-- +goose Up
CREATE TABLE IF NOT EXISTS kb.code_monitor (
    package_path TEXT NOT NULL,
    collection_date DATE NOT NULL,
    total_lines BIGINT NOT NULL,
    frontend_files BIGINT NOT NULL,
    backend_files BIGINT NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (package_path, collection_date)
);

-- +goose Down
DROP TABLE IF EXISTS kb.code_monitor;
