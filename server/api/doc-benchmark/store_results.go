package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

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
	rows, e := s.DB.QueryContext(txctx(ctx), `SELECT a.id,a.attempt_id,a.run_id,a.kind,a.path,a.sha256,a.size_bytes,a.verified,a.metadata_json FROM kb.benchmark_artifacts a LEFT JOIN kb.benchmark_case_attempts at ON at.id=a.attempt_id LEFT JOIN kb.benchmark_case_runs c ON c.id=at.case_run_id WHERE a.run_id=$1 OR (c.run_id=$1 AND c.selected_attempt_id=a.attempt_id) ORDER BY a.kind,a.path,a.id`, runID)
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
	b, _ := canonicalJSON(r.Metadata)
	var id string
	e := s.DB.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_scores (attempt_id,run_id,processor,scorer,scorer_version,metric,slice,direction,aggregation_kind,value,additive_component,numerator,denominator,non_null,applicable,metadata_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, r.AttemptID, r.RunID, r.Processor, r.Scorer, r.ScorerVersion, r.Metric, r.Slice, r.Direction, r.AggregationKind, r.Value, r.AdditiveComponent, r.Numerator, r.Denominator, r.NonNull, r.Applicable, b).Scan(&id)
	return id, e
}
func (s SQLStore) InsertArtifact(ctx context.Context, r ArtifactRecord) (string, error) {
	if s.DB == nil {
		return "", fmt.Errorf("nil database")
	}
	b, _ := canonicalJSON(r.Metadata)
	var id string
	e := s.DB.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_artifacts (attempt_id,run_id,kind,path,sha256,size_bytes,verified,metadata_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, r.AttemptID, r.RunID, r.Kind, r.Path, r.SHA256, r.SizeBytes, r.Verified, b).Scan(&id)
	return id, e
}
func (s SQLStore) MarkWorkspaceCleanup(ctx context.Context, id, state string, errText *string) error {
	res, e := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_workspaces SET cleanup_state=$2,cleanup_error=$3,cleaned_at=CASE WHEN $2='cleaned' THEN now() ELSE cleaned_at END WHERE id=$1`, id, state, errText)
	if e != nil {
		return e
	}
	return affected(res)
}
func (s SQLStore) ReportScores(ctx context.Context, runID string) ([]ScoreRecord, error) {
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
