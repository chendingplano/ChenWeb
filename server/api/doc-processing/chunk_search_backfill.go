package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type ChunkSearchBackfillResult struct {
	Scanned      int                        `json:"scanned"`
	Indexed      int                        `json:"indexed"`
	Failed       int                        `json:"failed"`
	NextRecordID int64                      `json:"next_record_id"`
	HasMore      bool                       `json:"has_more"`
	Errors       []ChunkSearchBackfillError `json:"errors"`
}
type ChunkSearchBackfillError struct {
	RecordID int64  `json:"record_id"`
	Error    string `json:"error"`
}

// BackfillChunkSearch reads IDs before model calls, so no cursor holds a pooled connection.
func BackfillChunkSearch(ctx context.Context, db *sql.DB, afterRecordID, recordID int64, limit int, logger ApiTypes.JimoLogger) (ChunkSearchBackfillResult, error) {
	result := ChunkSearchBackfillResult{NextRecordID: afterRecordID, Errors: []ChunkSearchBackfillError{}}
	if db == nil {
		return result, fmt.Errorf("project database is not initialized")
	}
	if limit < 1 || limit > 100 || afterRecordID < 0 || recordID < 0 {
		return result, fmt.Errorf("limit must be 1..100 and record IDs must be nonnegative")
	}
	records, err := db.QueryContext(ctx, `SELECT id FROM kb.inputs WHERE id > $1 AND ($2::bigint = 0 OR id = $2) AND COALESCE(result_filename, '') <> '' ORDER BY id LIMIT $3`, afterRecordID, recordID, limit+1)
	if err != nil {
		return result, err
	}
	var ids []int64
	for records.Next() {
		var id int64
		if err := records.Scan(&id); err != nil {
			records.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = records.Err()
	records.Close()
	if err != nil {
		return result, err
	}
	result.HasMore = len(ids) > limit
	if result.HasMore {
		ids = ids[:limit]
	}
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		result.Scanned++
		result.NextRecordID = id
		if err := reindexSavedChunkSearch(ctx, db, id, logger); err != nil {
			result.Failed++
			result.Errors = append(result.Errors, ChunkSearchBackfillError{RecordID: id, Error: err.Error()})
			if logger != nil {
				logger.Warn("chunk search backfill record failed", "record_id", id, "error", err)
			}
		} else {
			result.Indexed++
		}
	}
	return result, nil
}

func reindexSavedChunkSearch(ctx context.Context, db *sql.DB, recordID int64, logger ApiTypes.JimoLogger) error {
	ctx, unlock, err := lockChunkSearchSource(ctx, db, recordID)
	if err != nil {
		return err
	}
	defer unlock()
	generation := ctx.Value(chunkSearchLockKey{}).(chunkSearchSnapshot).generation
	rec, err := (SQLStore{DB: db}).GetInputRecord(ctx, recordID)
	if err != nil {
		return err
	}
	path, err := ResolveInputFilePath(LineFileGeneratedEvent{RecordID: recordID}, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines, err := ParseInputLines(body)
	if err != nil {
		return err
	}
	chunks, err := loadSearchChunks(rec, lines)
	if err != nil {
		return err
	}
	return indexChunkSearch(ctx, db, rec, chunks, generation, logger)
}
