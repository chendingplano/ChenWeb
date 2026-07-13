package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type ExperimentRecord struct {
	ID, Name, DatasetID, DatasetVersion, DatasetHash, RawRequestTOML, RawRequestHash string
	ResolvedExperiment, ResolvedFileHashes, ResolvedCaseSet                          json.RawMessage
	CreatedAt, UpdatedAt                                                             time.Time
}
type RunRecord struct {
	ID, ExperimentID, VariantName, Lifecycle                                     string
	Requested, Resolved, Config, Prompt, Scorer, Pricing                         json.RawMessage
	RequestedHash, ResolvedHash, ConfigHash, PromptHash, ScorerHash, PricingHash sql.NullString
	CreatedAt, UpdatedAt                                                         time.Time
	StartedAt, FinishedAt                                                        sql.NullTime
}
type CaseRunRecord struct {
	ID, RunID, CaseID        string
	Repetition               int
	Applicability, Lifecycle string
	TagsJSON                 json.RawMessage
	UpstreamHash             sql.NullString
	SelectedAttemptID        sql.NullString
	CreatedAt, UpdatedAt     time.Time
}

func (s SQLStore) CreateExperiment(ctx context.Context, e Experiment) (string, error) {
	if err := checkDB(s); err != nil {
		return "", err
	}
	re, rf, rc := e.MaterializedConfigJSON, e.FileHashes, e.ProcessorCaseSetHashes
	b1, err := canonicalJSON(re)
	if err != nil {
		return "", err
	}
	b2, err := canonicalJSON(rf)
	if err != nil {
		return "", err
	}
	b3, err := canonicalJSON(rc)
	if err != nil {
		return "", err
	}
	var id string
	err = s.DB.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_experiments (name,dataset_id,dataset_version,dataset_hash,raw_request_toml,raw_request_hash,resolved_experiment_json,resolved_file_hashes_json,resolved_case_set_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT (raw_request_hash) DO UPDATE SET updated_at=kb.benchmark_experiments.updated_at RETURNING id`, e.Name, e.DatasetID, e.DatasetVersion, e.DatasetHash, string(e.RawTOML), e.RequestHash, b1, b2, b3).Scan(&id)
	return id, err
}
func (s SQLStore) GetExperimentByRequestHash(ctx context.Context, h string) (ExperimentRecord, error) {
	if err := checkDB(s); err != nil {
		return ExperimentRecord{}, err
	}
	var r ExperimentRecord
	err := s.DB.QueryRowContext(txctx(ctx), `SELECT id,name,dataset_id,dataset_version,dataset_hash,raw_request_toml,raw_request_hash,resolved_experiment_json,resolved_file_hashes_json,resolved_case_set_json,created_at,updated_at FROM kb.benchmark_experiments WHERE raw_request_hash=$1`, h).Scan(&r.ID, &r.Name, &r.DatasetID, &r.DatasetVersion, &r.DatasetHash, &r.RawRequestTOML, &r.RawRequestHash, &r.ResolvedExperiment, &r.ResolvedFileHashes, &r.ResolvedCaseSet, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}
func (s SQLStore) CreateRun(ctx context.Context, experimentID, variant string, requested, resolved, config, prompt, scorer, pricing any) (string, error) {
	if err := checkDB(s); err != nil {
		return "", err
	}
	vals := make([][]byte, 5)
	inputs := []any{requested, resolved, config, prompt, scorer}
	for i, v := range inputs {
		vals[i], _ = canonicalJSON(v)
	}
	p, _ := canonicalJSON(pricing)
	var id string
	err := s.DB.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_runs (experiment_id,variant_name,requested_json,resolved_json,config_json,prompt_json,scorer_json,pricing_json) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT (experiment_id,variant_name) DO UPDATE SET updated_at=kb.benchmark_runs.updated_at RETURNING id`, experimentID, variant, vals[0], vals[1], vals[2], vals[3], vals[4], p).Scan(&id)
	return id, err
}
func (s SQLStore) CreateCaseRun(ctx context.Context, runID, caseID string, repetition int, applicability string, tags any, upstreamHash *string) (string, error) {
	if err := checkDB(s); err != nil {
		return "", err
	}
	b, _ := canonicalJSON(tags)
	var id string
	err := s.DB.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_case_runs (run_id,case_id,repetition,applicability,tags_json,upstream_hash) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (run_id,case_id,repetition) DO UPDATE SET updated_at=kb.benchmark_case_runs.updated_at RETURNING id`, runID, caseID, repetition, applicability, b, upstreamHash).Scan(&id)
	return id, err
}
func (s SQLStore) GetRun(ctx context.Context, id string) (RunRecord, error) {
	var r RunRecord
	err := s.DB.QueryRowContext(txctx(ctx), `SELECT id,experiment_id,variant_name,lifecycle,requested_json,resolved_json,config_json,prompt_json,scorer_json,pricing_json,requested_hash,resolved_hash,config_hash,prompt_hash,scorer_hash,pricing_hash,created_at,updated_at,started_at,finished_at FROM kb.benchmark_runs WHERE id=$1`, id).Scan(&r.ID, &r.ExperimentID, &r.VariantName, &r.Lifecycle, &r.Requested, &r.Resolved, &r.Config, &r.Prompt, &r.Scorer, &r.Pricing, &r.RequestedHash, &r.ResolvedHash, &r.ConfigHash, &r.PromptHash, &r.ScorerHash, &r.PricingHash, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.FinishedAt)
	return r, err
}
func (s SQLStore) ListCaseRuns(ctx context.Context, runID string) ([]CaseRunRecord, error) {
	rows, e := s.DB.QueryContext(txctx(ctx), `SELECT id,run_id,case_id,repetition,applicability,lifecycle,tags_json,upstream_hash,selected_attempt_id,created_at,updated_at FROM kb.benchmark_case_runs WHERE run_id=$1 ORDER BY case_id,repetition`, runID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []CaseRunRecord
	for rows.Next() {
		var r CaseRunRecord
		if e = rows.Scan(&r.ID, &r.RunID, &r.CaseID, &r.Repetition, &r.Applicability, &r.Lifecycle, &r.TagsJSON, &r.UpstreamHash, &r.SelectedAttemptID, &r.CreatedAt, &r.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
