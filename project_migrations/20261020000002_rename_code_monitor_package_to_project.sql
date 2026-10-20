-- +goose Up
ALTER TABLE kb.code_monitor RENAME COLUMN package_path TO project_path;

-- +goose Down
ALTER TABLE kb.code_monitor RENAME COLUMN project_path TO package_path;
