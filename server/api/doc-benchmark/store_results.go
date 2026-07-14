package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrConflict = errors.New("benchmark idempotency conflict")

type AttemptLifecycleError struct{ AttemptID, Lifecycle string }

func (e *AttemptLifecycleError) Error() string {
	return fmt.Sprintf("attempt %s is terminal (%s)", e.AttemptID, e.Lifecycle)
}

type SelectedAttemptError struct{ AttemptID string }

func (e *SelectedAttemptError) Error() string {
	return fmt.Sprintf("attempt %s is already selected", e.AttemptID)
}

type RunLifecycleError struct{ RunID, Lifecycle string }

func (e *RunLifecycleError) Error() string {
	return fmt.Sprintf("run %s is terminal (%s)", e.RunID, e.Lifecycle)
}

type VerifiedArtifactError struct{ ArtifactID string }

func (e *VerifiedArtifactError) Error() string {
	return fmt.Sprintf("verified artifact %s cannot be mutated", e.ArtifactID)
}

// ConflictError indicates that an idempotency key already exists with a
// different canonical payload.
type ConflictError struct{ Resource, Key string }

func (e *ConflictError) Error() string { return fmt.Sprintf("%s conflict for %s", e.Resource, e.Key) }
func (e *ConflictError) Unwrap() error { return ErrConflict }

func IsConflict(err error) bool { var c *ConflictError; return errors.As(err, &c) }

type ScoreRecord struct {
	ID                                                                          string
	AttemptID, RunID                                                            sql.NullString
	Processor, Scorer, ScorerVersion, Metric, Slice, Direction, AggregationKind string
	Value, AdditiveComponent, Numerator, Denominator                            sql.NullFloat64
	NonNull, Applicable                                                         bool
	Metadata                                                                    json.RawMessage
}
type ArtifactRecord struct {
	ID                 string
	AttemptID, RunID   sql.NullString
	Kind, Path, SHA256 string
	SizeBytes          int64
	Verified           bool
	Metadata           json.RawMessage
}

func (s SQLStore) ReportArtifacts(ctx context.Context, runID string) ([]ArtifactRecord, error) {
	if err := checkDB(s); err != nil {
		return nil, err
	}
	rows, e := s.DB.QueryContext(txctx(ctx), `SELECT a.id,a.attempt_id,a.run_id,a.kind,a.path,a.sha256,a.size_bytes,a.verified,a.metadata_json FROM kb.benchmark_artifacts a LEFT JOIN kb.benchmark_case_attempts at ON at.id=a.attempt_id LEFT JOIN kb.benchmark_case_runs c ON c.id=at.case_run_id LEFT JOIN kb.benchmark_case_attempts selected ON selected.id=c.selected_attempt_id WHERE a.run_id=$1 OR (c.run_id=$1 AND (c.selected_attempt_id=a.attempt_id OR (selected.kind='rescore' AND selected.source_execution_attempt_id=a.attempt_id AND a.verified=true))) ORDER BY a.kind,a.path,a.id`, runID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []ArtifactRecord
	for rows.Next() {
		var r ArtifactRecord
		if e = rows.Scan(&r.ID, &r.AttemptID, &r.RunID, &r.Kind, &r.Path, &r.SHA256, &r.SizeBytes, &r.Verified, &r.Metadata); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s SQLStore) InsertScore(ctx context.Context, r ScoreRecord) (string, error) {
	if s.DB == nil {
		return "", fmt.Errorf("nil database")
	}
	if r.AttemptID.Valid == r.RunID.Valid {
		return "", fmt.Errorf("score requires exactly one owner")
	}
	tx, err := s.DB.BeginTx(txctx(ctx), nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if r.AttemptID.Valid {
		if err = guardAttemptOwnerTx(txctx(ctx), tx, r.AttemptID.String); err != nil {
			return "", err
		}
	} else if err = guardRunOwnerTx(txctx(ctx), tx, r.RunID.String); err != nil {
		return "", err
	}
	b, err := canonicalJSON(r.Metadata)
	if err != nil {
		return "", err
	}
	var id string
	var old ScoreRecord
	q := `SELECT id,processor,scorer,scorer_version,direction,value,additive_component,numerator,denominator,non_null,applicable,metadata_json FROM kb.benchmark_scores WHERE ((attempt_id=$1 AND $1 IS NOT NULL) OR (run_id=$2 AND $2 IS NOT NULL)) AND metric=$3 AND slice=$4 AND aggregation_kind=$5`
	err = tx.QueryRowContext(txctx(ctx), q, nullArg(r.AttemptID), nullArg(r.RunID), r.Metric, r.Slice, r.AggregationKind).Scan(&old.ID, &old.Processor, &old.Scorer, &old.ScorerVersion, &old.Direction, &old.Value, &old.AdditiveComponent, &old.Numerator, &old.Denominator, &old.NonNull, &old.Applicable, &old.Metadata)
	if err == nil {
		if scoreEqual(r, old, b) {
			return old.ID, tx.Commit()
		}
		return "", &ConflictError{Resource: "score", Key: r.Metric + "/" + r.Slice + "/" + r.AggregationKind}
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	e := tx.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_scores (attempt_id,run_id,processor,scorer,scorer_version,metric,slice,direction,aggregation_kind,value,additive_component,numerator,denominator,non_null,applicable,metadata_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, r.AttemptID, r.RunID, r.Processor, r.Scorer, r.ScorerVersion, r.Metric, r.Slice, r.Direction, r.AggregationKind, r.Value, r.AdditiveComponent, r.Numerator, r.Denominator, r.NonNull, r.Applicable, b).Scan(&id)
	if e != nil {
		return "", e
	}
	return id, tx.Commit()
}
func (s SQLStore) InsertArtifact(ctx context.Context, r ArtifactRecord) (string, error) {
	if s.DB == nil {
		return "", fmt.Errorf("nil database")
	}
	if r.AttemptID.Valid == r.RunID.Valid {
		return "", fmt.Errorf("artifact requires exactly one owner")
	}
	tx, err := s.DB.BeginTx(txctx(ctx), nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if r.AttemptID.Valid {
		if err = guardAttemptOwnerTx(txctx(ctx), tx, r.AttemptID.String); err != nil {
			return "", err
		}
	} else if err = guardRunOwnerTx(txctx(ctx), tx, r.RunID.String); err != nil {
		return "", err
	}
	b, err := canonicalJSON(r.Metadata)
	if err != nil {
		return "", err
	}
	var id string
	var old ArtifactRecord
	err = tx.QueryRowContext(txctx(ctx), `SELECT id,sha256,size_bytes,verified,metadata_json FROM kb.benchmark_artifacts WHERE ((attempt_id=$1 AND $1 IS NOT NULL) OR (run_id=$2 AND $2 IS NOT NULL)) AND kind=$3 AND path=$4`, nullArg(r.AttemptID), nullArg(r.RunID), r.Kind, r.Path).Scan(&old.ID, &old.SHA256, &old.SizeBytes, &old.Verified, &old.Metadata)
	if err == nil {
		if old.SHA256 == r.SHA256 && old.SizeBytes == r.SizeBytes && old.Verified == r.Verified && string(old.Metadata) == string(b) {
			return old.ID, tx.Commit()
		}
		if old.Verified {
			return "", &VerifiedArtifactError{ArtifactID: old.ID}
		}
		return "", &ConflictError{Resource: "artifact", Key: r.Kind + "/" + r.Path}
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	e := tx.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_artifacts (attempt_id,run_id,kind,path,sha256,size_bytes,verified,metadata_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, r.AttemptID, r.RunID, r.Kind, r.Path, r.SHA256, r.SizeBytes, r.Verified, b).Scan(&id)
	if e != nil {
		return "", e
	}
	return id, tx.Commit()
}

func guardRunOwnerTx(ctx context.Context, tx *sql.Tx, id string) error {
	var lifecycle string
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle FROM kb.benchmark_runs WHERE id=$1 FOR UPDATE`, id).Scan(&lifecycle); err != nil {
		return err
	}
	if lifecycle != "queued" && lifecycle != "running" {
		return &RunLifecycleError{RunID: id, Lifecycle: lifecycle}
	}
	return nil
}

func guardAttemptOwnerTx(ctx context.Context, tx *sql.Tx, id string) error {
	var caseRunID string
	if err := tx.QueryRowContext(ctx, `SELECT case_run_id FROM kb.benchmark_case_attempts WHERE id=$1`, id).Scan(&caseRunID); err != nil {
		return err
	}
	var caseLifecycle, runID string
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle,run_id FROM kb.benchmark_case_runs WHERE id=$1 FOR UPDATE`, caseRunID).Scan(&caseLifecycle, &runID); err != nil {
		return err
	}
	var runLifecycle string
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle FROM kb.benchmark_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&runLifecycle); err != nil {
		return err
	}
	var lifecycle string
	var selected sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT a.lifecycle,c.selected_attempt_id FROM kb.benchmark_case_attempts a JOIN kb.benchmark_case_runs c ON c.id=a.case_run_id WHERE a.id=$1 FOR UPDATE`, id).Scan(&lifecycle, &selected); err != nil {
		return err
	}
	if runLifecycle != "queued" && runLifecycle != "running" {
		return &RunLifecycleError{RunID: runID, Lifecycle: runLifecycle}
	}
	if selected.Valid {
		return &SelectedAttemptError{AttemptID: id}
	}
	if lifecycle != "leased" && lifecycle != "running" {
		return &AttemptLifecycleError{AttemptID: id, Lifecycle: lifecycle}
	}
	return nil
}

/*
func (s SQLStore) guardRunOwner(ctx context.Context, id string) error {
	var lifecycle string
	if err := s.DB.QueryRowContext(txctx(ctx), `SELECT lifecycle FROM kb.benchmark_runs WHERE id=$1`, id).Scan(&lifecycle); err != nil {
		return err
	}
	if lifecycle != "queued" && lifecycle != "running" {
		return &RunLifecycleError{RunID: id, Lifecycle: lifecycle}
	}
	return nil
}

func (s SQLStore) guardAttemptOwner(ctx context.Context, id string) error {
	var lifecycle, runLifecycle string
	var selected sql.NullString
	if err := s.DB.QueryRowContext(txctx(ctx), `SELECT a.lifecycle,c.lifecycle,c.selected_attempt_id FROM kb.benchmark_case_attempts a JOIN kb.benchmark_case_runs c ON c.id=a.case_run_id WHERE a.id=$1`, id).Scan(&lifecycle, &runLifecycle, &selected); err != nil {
		return err
	}
	if runLifecycle != "queued" && runLifecycle != "running" {
		return &RunLifecycleError{Lifecycle: runLifecycle}
	}
	if selected.Valid {
		return &SelectedAttemptError{AttemptID: id}
	}
	if lifecycle != "leased" && lifecycle != "running" {
		return &AttemptLifecycleError{AttemptID: id, Lifecycle: lifecycle}
	}
	return nil
}
*/

func nullArg(v sql.NullString) any {
	if v.Valid {
		return v.String
	}
	return nil
}
func scoreEqual(r ScoreRecord, o ScoreRecord, metadata []byte) bool {
	return r.Processor == o.Processor && r.Scorer == o.Scorer && r.ScorerVersion == o.ScorerVersion && r.Direction == o.Direction && r.Value == o.Value && r.AdditiveComponent == o.AdditiveComponent && r.Numerator == o.Numerator && r.Denominator == o.Denominator && r.NonNull == o.NonNull && r.Applicable == o.Applicable && string(o.Metadata) == string(metadata)
}
func (s SQLStore) MarkWorkspaceCleanup(ctx context.Context, id, state string, errText *string) error {
	res, e := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_workspaces SET cleanup_state=$2,cleanup_error=$3,cleaned_at=CASE WHEN $2='cleaned' THEN now() ELSE cleaned_at END WHERE id=$1`, id, state, errText)
	if e != nil {
		return e
	}
	return affected(res)
}
func (s SQLStore) ReportScores(ctx context.Context, runID string) ([]ScoreRecord, error) {
	if err := checkDB(s); err != nil {
		return nil, err
	}
	rows, e := s.DB.QueryContext(txctx(ctx), `SELECT s.id,s.attempt_id,s.run_id,s.processor,s.scorer,s.scorer_version,s.metric,s.slice,s.direction,s.aggregation_kind,s.value,s.additive_component,s.numerator,s.denominator,s.non_null,s.applicable,s.metadata_json FROM kb.benchmark_scores s LEFT JOIN kb.benchmark_case_attempts a ON a.id=s.attempt_id LEFT JOIN kb.benchmark_case_runs c ON c.id=a.case_run_id WHERE s.run_id=$1 OR (c.run_id=$1 AND c.selected_attempt_id=s.attempt_id) ORDER BY s.processor,s.metric,s.slice,s.aggregation_kind,s.id`, runID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []ScoreRecord
	for rows.Next() {
		var r ScoreRecord
		if e = rows.Scan(&r.ID, &r.AttemptID, &r.RunID, &r.Processor, &r.Scorer, &r.ScorerVersion, &r.Metric, &r.Slice, &r.Direction, &r.AggregationKind, &r.Value, &r.AdditiveComponent, &r.Numerator, &r.Denominator, &r.NonNull, &r.Applicable, &r.Metadata); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
