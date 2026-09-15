package datasync

import (
	"context"
	"database/sql"
)

// SyncState is one item's row in kb.data_sync_state (or the zero value if the
// item has never been synced from this target).
type SyncState struct {
	SyncItemID   string         `json:"sync_item_id"`
	LastCursor   sql.NullString `json:"-"`
	LastSyncedAt sql.NullString `json:"-"`
	LastRowCount int            `json:"last_row_count"`
	LastError    sql.NullString `json:"-"`
}

func getSyncState(ctx context.Context, db *sql.DB, itemID string) (SyncState, error) {
	state := SyncState{SyncItemID: itemID}
	err := db.QueryRowContext(ctx, `
		SELECT last_cursor::text, last_synced_at::text, last_row_count, last_error
		FROM kb.data_sync_state WHERE sync_item_id = $1`, itemID,
	).Scan(&state.LastCursor, &state.LastSyncedAt, &state.LastRowCount, &state.LastError)
	if err == sql.ErrNoRows {
		return state, nil
	}
	if err != nil {
		return SyncState{}, err
	}
	return state, nil
}

func saveSyncSuccess(ctx context.Context, db *sql.DB, itemID, cursor string, rowCount int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO kb.data_sync_state (sync_item_id, last_cursor, last_synced_at, last_row_count, last_error)
		VALUES ($1, $2, NOW(), $3, NULL)
		ON CONFLICT (sync_item_id) DO UPDATE SET
			last_cursor = EXCLUDED.last_cursor,
			last_synced_at = EXCLUDED.last_synced_at,
			last_row_count = EXCLUDED.last_row_count,
			last_error = NULL`, itemID, nullIfEmpty(cursor), rowCount)
	return err
}

func saveSyncError(ctx context.Context, db *sql.DB, itemID, errMsg string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO kb.data_sync_state (sync_item_id, last_row_count, last_error)
		VALUES ($1, 0, $2)
		ON CONFLICT (sync_item_id) DO UPDATE SET last_error = EXCLUDED.last_error`, itemID, errMsg)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
