package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// DocProcessRunRecord describes one kb.doc_process_runs row to be created —
// one row per handleEvent invocation (one pipeline execution for one record).
// See ADR 2026071201.
type DocProcessRunRecord struct {
	RecordID   int64
	EventID    string // empty -> NULL (not every invocation traces back to a kb.events row)
	Mode       string // "auto" | "dev"
	Processors []string
	Parameters map[string]any
}

// DocProcessRunStore creates and closes kb.doc_process_runs rows.
type DocProcessRunStore interface {
	CreateDocProcessRun(ctx context.Context, rec DocProcessRunRecord) (int64, error)
	CloseDocProcessRun(ctx context.Context, runID int64, status string, errMsg *string) error
}

// CreateDocProcessRun inserts a new run row, assigning run_number as the next
// sequential value for the record (1, 2, 3...), and returns the new row's id.
func (s SQLStore) CreateDocProcessRun(ctx context.Context, rec DocProcessRunRecord) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("db is nil")
	}
	if rec.RecordID <= 0 {
		return 0, errors.New("record_id is required")
	}
	if strings.TrimSpace(rec.Mode) == "" {
		return 0, errors.New("mode is required")
	}

	processors := rec.Processors
	if processors == nil {
		processors = []string{}
	}
	processorsJSON, err := json.Marshal(processors)
	if err != nil {
		return 0, err
	}

	parameters := rec.Parameters
	if parameters == nil {
		parameters = map[string]any{}
	}
	parametersJSON, err := json.Marshal(parameters)
	if err != nil {
		return 0, err
	}

	var eventID *string
	if trimmed := strings.TrimSpace(rec.EventID); trimmed != "" {
		eventID = &trimmed
	}

	const stmt = `
INSERT INTO kb.doc_process_runs (record_id, event_id, mode, run_number, processors, parameters)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(run_number), 0) + 1 FROM kb.doc_process_runs WHERE record_id = $1), $4::jsonb, $5::jsonb)
RETURNING id`

	var id int64
	err = s.DB.QueryRowContext(ctx, stmt,
		rec.RecordID,
		eventID,
		rec.Mode,
		string(processorsJSON),
		string(parametersJSON),
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// CloseDocProcessRun sets the run's terminal status, error message, and end_time.
func (s SQLStore) CloseDocProcessRun(ctx context.Context, runID int64, status string, errMsg *string) error {
	if s.DB == nil {
		return errors.New("db is nil")
	}
	const stmt = `
UPDATE kb.doc_process_runs
SET status = $2, error_message = $3, end_time = NOW()
WHERE id = $1`
	_, err := s.DB.ExecContext(ctx, stmt, runID, status, errMsg)
	return err
}
