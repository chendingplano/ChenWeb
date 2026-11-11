package kbsearch

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

var chunkSourceLockBudgets sync.Map // *sql.DB -> admission channel, for fixed pool limits

func reserveChunkSourceConnection(ctx context.Context, db *sql.DB) (func(), error) {
	if db == nil {
		return nil, fmt.Errorf("project database is not initialized")
	}
	maxOpen := db.Stats().MaxOpenConnections
	if maxOpen == 1 {
		return nil, fmt.Errorf("chunk source operations require a database pool with at least two connections")
	}
	if maxOpen == 0 {
		return func() {}, nil
	}
	// Leave at least half the centralized pool available for store calls and publication.
	value, _ := chunkSourceLockBudgets.LoadOrStore(db, make(chan struct{}, maxOpen/2))
	slots := value.(chan struct{})
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// LockChunkSearchSource serializes source reads, file mutations and publication
// across processes. It holds one pooled connection, without a long transaction.
func LockChunkSearchSource(ctx context.Context, db *sql.DB, recordID int64) (func(), error) {
	freeSlot, err := reserveChunkSourceConnection(ctx, db)
	if err != nil {
		return nil, err
	}
	var conn *sql.Conn
	for {
		conn, err = db.Conn(ctx)
		if err != nil {
			freeSlot()
			return nil, err
		}
		var acquired bool
		err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(hashtextextended('chunk-search:' || $1::text, 0))`, recordID).Scan(&acquired)
		if err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			conn.Close()
			freeSlot()
			return nil, err
		}
		if acquired {
			break
		}
		conn.Close()
		// Waiting for another record operation never reserves a pooled connection.
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			freeSlot()
			return nil, ctx.Err()
		}
	}
	return func() {
		defer freeSlot()
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.ExecContext(cleanup, `SELECT pg_advisory_unlock(hashtextextended('chunk-search:' || $1::text, 0))`, recordID); err != nil {
			loggerutil.CreateDefaultLogger("20261111-734").Error("release chunk source lock failed", "record_id", recordID, "error", err)
			// Discard the physical connection so a session lock cannot return to the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}
