-- +goose Up
-- Expose the existing per-group, per-document primary key to the sync picker,
-- which offers separately declared UNIQUE constraints as natural keys.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'doc_group_categories_sync_natural_key'
          AND conrelid = 'kb.doc_group_categories'::regclass
    ) THEN
        ALTER TABLE kb.doc_group_categories
            ADD CONSTRAINT doc_group_categories_sync_natural_key UNIQUE (group_id, doc_id);
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.doc_group_categories
    DROP CONSTRAINT IF EXISTS doc_group_categories_sync_natural_key;
