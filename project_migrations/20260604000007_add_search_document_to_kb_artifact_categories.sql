-- +goose Up
-- +goose StatementBegin
ALTER TABLE kb.artifact_categories
    ADD COLUMN IF NOT EXISTS search_document TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE kb.artifact_categories
    DROP COLUMN IF EXISTS search_document;
-- +goose StatementEnd
