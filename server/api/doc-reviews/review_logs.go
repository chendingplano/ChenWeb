package docreviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ReviewLogEntry is one reviewer-execution audit row persisted to
// kb.doc_review_logs. Unlike ReviewFinding, it records both finding-emitting
// and no-issue/error review attempts for one review unit.
type ReviewLogEntry struct {
	InputRecordID int64
	RunID         int64
	Pass          string
	Aspect        string
	UnitType      string
	UnitKey       string
	UnitLocation  map[string]any
	MatchedUnits  []map[string]any
	Findings      []ReviewFinding
	Outcome       string
	Detail        map[string]any
}

// ReviewLogsStore persists reviewer execution logs.
type ReviewLogsStore interface {
	SaveLogs(ctx context.Context, logs []ReviewLogEntry) (int64, error)
	DeleteLogs(ctx context.Context, runID int64) (int64, error)
}

// ReviewLogsSQLStore implements ReviewLogsStore.
type ReviewLogsSQLStore struct {
	DB *sql.DB
}

func (s ReviewLogsSQLStore) SaveLogs(ctx context.Context, logs []ReviewLogEntry) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	if len(logs) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.doc_review_logs
    (input_record_id, run_id, pass, aspect, unit_type, unit_key,
     unit_location, matched_units, findings, outcome, detail)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	var inserted int64
	for _, log := range logs {
		unitLocation, err := json.Marshal(log.UnitLocation)
		if err != nil {
			return inserted, fmt.Errorf("marshal review log unit_location: %w", err)
		}
		matchedUnits, err := json.Marshal(log.MatchedUnits)
		if err != nil {
			return inserted, fmt.Errorf("marshal review log matched_units: %w", err)
		}
		findings, err := json.Marshal(log.Findings)
		if err != nil {
			return inserted, fmt.Errorf("marshal review log findings: %w", err)
		}
		detail, err := json.Marshal(log.Detail)
		if err != nil {
			return inserted, fmt.Errorf("marshal review log detail: %w", err)
		}

		_, err = s.DB.ExecContext(ctx, stmt,
			log.InputRecordID,
			log.RunID,
			log.Pass,
			log.Aspect,
			log.UnitType,
			log.UnitKey,
			unitLocation,
			matchedUnits,
			findings,
			log.Outcome,
			detail,
		)
		if err != nil {
			return inserted, fmt.Errorf("insert review log: %w", err)
		}
		inserted++
	}
	return inserted, nil
}

func (s ReviewLogsSQLStore) DeleteLogs(ctx context.Context, runID int64) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.doc_review_logs WHERE run_id = $1`, runID)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return n, nil
}
