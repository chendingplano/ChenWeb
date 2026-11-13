-- +goose Up
-- The sync registry only offers UNIQUE constraints other than the primary key
-- as natural-key candidates. This duplicates the composite primary key so the
-- deployment sync can select the same stable (group_id, doc_id) key.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'doc_group_exclusions_sync_natural_key'
          AND conrelid = 'kb.doc_group_exclusions'::regclass
    ) THEN
        ALTER TABLE kb.doc_group_exclusions
            ADD CONSTRAINT doc_group_exclusions_sync_natural_key UNIQUE (group_id, doc_id);
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.doc_group_exclusions
    DROP CONSTRAINT IF EXISTS doc_group_exclusions_sync_natural_key;
