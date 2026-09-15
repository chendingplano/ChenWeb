-- +goose Up
-- Per-sync-item cursor/run state for a sync *target* (see
-- openspec/changes/production-data-sync). One row per registered sync item;
-- last_cursor is the source-reported cursor through which changes have been
-- successfully applied -- it is only advanced after a fully successful apply,
-- so a failed run is always safe to retry from the same starting point.
CREATE TABLE IF NOT EXISTS kb.data_sync_state (
    sync_item_id   TEXT PRIMARY KEY,
    last_cursor    TIMESTAMPTZ,
    last_synced_at TIMESTAMPTZ,
    last_row_count INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT
);

-- +goose Down
DROP TABLE IF EXISTS kb.data_sync_state;
