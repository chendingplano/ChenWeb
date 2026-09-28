-- +goose Up
-- Preserve existing values: this column stores the submitting user's ID.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'inputs' AND column_name = 'tenant_id'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'inputs' AND column_name = 'user_id'
    ) THEN
        ALTER TABLE kb.inputs RENAME COLUMN tenant_id TO user_id;
    END IF;
END;
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'inputs' AND column_name = 'user_id'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'inputs' AND column_name = 'tenant_id'
    ) THEN
        ALTER TABLE kb.inputs RENAME COLUMN user_id TO tenant_id;
    END IF;
END;
$$;
-- +goose StatementEnd
