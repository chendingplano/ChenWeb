-- +goose Up
-- +goose StatementBegin
ALTER TABLE kb.artifact_categories
    ADD COLUMN IF NOT EXISTS category_id BIGINT GENERATED ALWAYS AS IDENTITY;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE kb.artifact_categories
    DROP COLUMN IF EXISTS category_id;
-- +goose StatementEnd
