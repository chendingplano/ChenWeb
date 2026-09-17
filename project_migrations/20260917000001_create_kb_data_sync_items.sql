-- +goose Up
-- Runtime-created sync item definitions (see
-- openspec/changes/configurable-data-sync-items). Compiled items in
-- server/api/datasync/registry.go's Registry still work unmigrated; this
-- table only holds items created via the "New Data Syncher" admin UI
-- (origin='local') or write-through-cached from a configured source via
-- discovery (origin='learned', design.md Decision 4/6). NaturalKey/Columns/
-- JSONColumns are stored as JSONB string arrays, decoded into
-- datasync.TableSyncItem at read time.
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.data_sync_items (
    id                     TEXT        PRIMARY KEY,
    kind                   TEXT        NOT NULL DEFAULT 'table'
                                        CHECK (kind IN ('table', 'table_with_files')),
    origin                 TEXT        NOT NULL
                                        CHECK (origin IN ('local', 'learned')),
    table_name             TEXT        NOT NULL,
    cursor_col             TEXT        NOT NULL,
    natural_key            JSONB       NOT NULL,
    columns                JSONB       NOT NULL,
    json_columns           JSONB       NOT NULL DEFAULT '[]',
    filter                 TEXT,
    file_column            TEXT,
    file_dir_env           TEXT,
    file_dir_default_subdir TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by             TEXT,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE kb.data_sync_items IS
    'Runtime-created data-sync item definitions, unioned with the compiled '
    'Registry in server/api/datasync. origin=local: created on this instance '
    'via the admin UI. origin=learned: cached from this instance''s '
    'configured DATA_SYNC_SOURCE_URL via item-definition discovery.';

-- +goose Down
DROP TABLE IF EXISTS kb.data_sync_items;
