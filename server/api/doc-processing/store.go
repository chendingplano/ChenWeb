package docprocessing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type SQLStore struct {
	DB *sql.DB
}

func (s SQLStore) GetInputRecord(ctx context.Context, id int64) (InputRecord, error) {
	if s.DB == nil {
		return InputRecord{}, errors.New("db is nil")
	}
	const stmt = `
SELECT id,
       COALESCE(status::text, '[]')
FROM kb.inputs
WHERE id = $1`

	var rec InputRecord
	err := s.DB.QueryRowContext(ctx, stmt, id).Scan(&rec.ID, &rec.StatusRaw)
	if err != nil {
		return InputRecord{}, err
	}
	return rec, nil
}

func (s SQLStore) InsertChunkRun(ctx context.Context, rec ChunkRunRecord) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if rec.SourceRecordID <= 0 {
		return fmt.Errorf("invalid source_record_id: %d", rec.SourceRecordID)
	}
	method := strings.TrimSpace(rec.ChunkingMethod)
	if method == "" {
		method = ChunkingMethodFixed
	}
	const stmt = `
INSERT INTO kb.chunks (
	source_record_id,
	chunking_method,
	chunking_size,
	overlap_percent,
	notes,
	create_time,
	update_time
) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
	_, err := s.DB.ExecContext(ctx, stmt,
		rec.SourceRecordID,
		method,
		rec.ChunkingSize,
		rec.OverlapPercent,
		strings.TrimSpace(rec.Notes),
	)
	return err
}

func (s SQLStore) UpdateInputStatus(ctx context.Context, id int64, statusJSON string, errorMsg *string) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(statusJSON) == "" {
		statusJSON = "[]"
	}
	const stmt = `
UPDATE kb.inputs
SET status = $2::jsonb,
	error_msg = $3,
	modify_time = NOW()
WHERE id = $1`
	_, err := s.DB.ExecContext(ctx, stmt, id, statusJSON, errorMsg)
	return err
}
