package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type SQLStore struct {
	DB *sql.DB
}

type EventRecord struct {
	EventName    string
	EventID      string
	EventSubject string
	EventPayload string
	EventStatus  string
	EventSource  string
	EventNotes   string
}

type EventStore interface {
	InsertEvent(ctx context.Context, rec EventRecord) error
	UpsertConsumedStatus(ctx context.Context, eventID string, start time.Time, msUsed int64, procErr error) error
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

func (s SQLStore) InsertEvent(ctx context.Context, rec EventRecord) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	if strings.TrimSpace(rec.EventName) == "" {
		return errors.New("event_name is required")
	}
	if strings.TrimSpace(rec.EventID) == "" {
		return errors.New("event_id is required")
	}
	payload := strings.TrimSpace(rec.EventPayload)
	if payload == "" {
		payload = "{}"
	}
	status := strings.TrimSpace(rec.EventStatus)
	if status == "" {
		status = "[]"
	}
	const stmt = `
INSERT INTO kb.events (
	event_name,
	event_id,
	event_time,
	event_subject,
	event_payload,
	event_status,
	event_source,
	event_notes,
	create_time,
	modify_time
) VALUES ($1, $2, NOW(), $3, $4::jsonb, $5::jsonb, $6, $7, NOW(), NOW())`
	_, err := s.DB.ExecContext(ctx, stmt,
		strings.TrimSpace(rec.EventName),
		strings.TrimSpace(rec.EventID),
		strings.TrimSpace(rec.EventSubject),
		payload,
		status,
		strings.TrimSpace(rec.EventSource),
		strings.TrimSpace(rec.EventNotes),
	)
	return err
}

func (s SQLStore) UpsertConsumedStatus(ctx context.Context, eventID string, start time.Time, msUsed int64, procErr error) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return errors.New("event_id is required")
	}

	const selectStmt = `
SELECT COALESCE(event_status::text, '[]')
FROM kb.events
WHERE event_id = $1`
	var raw string
	if err := s.DB.QueryRowContext(ctx, selectStmt, eventID).Scan(&raw); err != nil {
		return err
	}

	entry := map[string]any{
		"operation":   "consumed",
		"start_time":  start.Format(defaultDocMetaStatusTime),
		"ms_used":     msUsed,
		"proc_status": "success",
	}
	errMsg := (*string)(nil)
	if procErr != nil {
		entry["proc_status"] = "failed"
		msg := strings.TrimSpace(procErr.Error())
		entry["error_msg"] = msg
		errMsg = &msg
	}

	entries := decodeStatus(raw)
	out := make([]map[string]any, 0, len(entries)+1)
	replaced := false
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "consumed" {
			out = append(out, e)
			continue
		}
		if !replaced {
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}
	statusJSON, err := json.Marshal(out)
	if err != nil {
		return err
	}

	const updateStmt = `
UPDATE kb.events
SET event_status = $2::jsonb,
	error_msg = $3,
	modify_time = NOW()
WHERE event_id = $1`
	_, err = s.DB.ExecContext(ctx, updateStmt, eventID, string(statusJSON), errMsg)
	return err
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
