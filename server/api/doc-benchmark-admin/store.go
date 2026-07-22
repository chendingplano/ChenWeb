package docbenchmarkadmin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

type Store struct {
	DB *sql.DB
}

func (s Store) ensureDB() error {
	if s.DB == nil {
		return errors.New("project database is not initialized")
	}
	return nil
}

func (s Store) LoadConfig(ctx context.Context, scope string) (Config, error) {
	if err := s.ensureDB(); err != nil {
		return Config{}, err
	}
	cfg := defaultConfig()
	cfg.Scope = scope
	var createdAt, updatedAt sql.NullTime
	err := s.DB.QueryRowContext(ctx, `SELECT scope,experiment_path,dataset_root,artifact_root,work_root,evidence_root,store_id,owner,tenant_id,metrics_model_name,allow_dirty,report_format,report_output_path,metrics_baseline,metrics_candidate,chunk_baseline,chunk_candidate,created_at,updated_at FROM doc_benchmark_admin_config WHERE scope=$1`, scope).
		Scan(&cfg.Scope, &cfg.ExperimentPath, &cfg.DatasetRoot, &cfg.ArtifactRoot, &cfg.WorkRoot, &cfg.EvidenceRoot, &cfg.StoreID, &cfg.Owner, &cfg.TenantID, &cfg.MetricsModelName, &cfg.AllowDirty, &cfg.ReportFormat, &cfg.ReportOutputPath, &cfg.MetricsBaseline, &cfg.MetricsCandidate, &cfg.ChunkBaseline, &cfg.ChunkCandidate, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	cfg.CreatedAt = formatTS(createdAt.Time)
	cfg.UpdatedAt = formatTS(updatedAt.Time)
	return cfg, nil
}

func (s Store) SaveConfig(ctx context.Context, cfg Config) (Config, error) {
	if err := s.ensureDB(); err != nil {
		return Config{}, err
	}
	if cfg.Scope == "" {
		cfg.Scope = DefaultScope
	}
	err := s.DB.QueryRowContext(ctx, `INSERT INTO doc_benchmark_admin_config
		(scope,experiment_path,dataset_root,artifact_root,work_root,evidence_root,store_id,owner,tenant_id,metrics_model_name,allow_dirty,report_format,report_output_path,metrics_baseline,metrics_candidate,chunk_baseline,chunk_candidate)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		ON CONFLICT (scope) DO UPDATE SET
			experiment_path=EXCLUDED.experiment_path,
			dataset_root=EXCLUDED.dataset_root,
			artifact_root=EXCLUDED.artifact_root,
			work_root=EXCLUDED.work_root,
			evidence_root=EXCLUDED.evidence_root,
			store_id=EXCLUDED.store_id,
			owner=EXCLUDED.owner,
			tenant_id=EXCLUDED.tenant_id,
			metrics_model_name=EXCLUDED.metrics_model_name,
			allow_dirty=EXCLUDED.allow_dirty,
			report_format=EXCLUDED.report_format,
			report_output_path=EXCLUDED.report_output_path,
			metrics_baseline=EXCLUDED.metrics_baseline,
			metrics_candidate=EXCLUDED.metrics_candidate,
			chunk_baseline=EXCLUDED.chunk_baseline,
			chunk_candidate=EXCLUDED.chunk_candidate,
			updated_at=NOW()
		RETURNING created_at,updated_at`,
		cfg.Scope, cfg.ExperimentPath, cfg.DatasetRoot, cfg.ArtifactRoot, cfg.WorkRoot, cfg.EvidenceRoot,
		cfg.StoreID, cfg.Owner, cfg.TenantID, cfg.MetricsModelName, cfg.AllowDirty, cfg.ReportFormat,
		cfg.ReportOutputPath, cfg.MetricsBaseline, cfg.MetricsCandidate, cfg.ChunkBaseline, cfg.ChunkCandidate).
		Scan(&cfg.CreatedAt, &cfg.UpdatedAt)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (s Store) InsertJob(ctx context.Context, scope, stepID, jobType string, request map[string]any, createdBy string) (Job, error) {
	if err := s.ensureDB(); err != nil {
		return Job{}, err
	}
	raw, _ := json.Marshal(request)
	var job Job
	var req json.RawMessage
	err := s.DB.QueryRowContext(ctx, `INSERT INTO doc_benchmark_admin_jobs
		(scope,step_id,job_type,status,message,request_json,created_by)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7)
		RETURNING id,scope,step_id,job_type,status,message,request_json,error_text,created_by,created_at,started_at,finished_at,updated_at`,
		scope, stepID, jobType, JobQueued, "", string(raw), createdBy).
		Scan(&job.ID, &job.Scope, &job.StepID, &job.JobType, &job.Status, &job.Message, &req, &job.ErrorText, &job.CreatedBy, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	_ = json.Unmarshal(req, &job.Request)
	return job, nil
}

func (s Store) UpdateJob(ctx context.Context, id int64, status, message string, result map[string]any, errText string, setStarted, setFinished bool) error {
	if err := s.ensureDB(); err != nil {
		return err
	}
	raw, _ := json.Marshal(result)
	startedSQL := "started_at"
	if setStarted {
		startedSQL = "COALESCE(started_at, NOW())"
	}
	finishedSQL := "finished_at"
	if setFinished {
		finishedSQL = "NOW()"
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE doc_benchmark_admin_jobs
		SET status=$2,message=$3,result_json=$4::jsonb,error_text=$5,started_at=`+startedSQL+`,finished_at=`+finishedSQL+`,updated_at=NOW()
		WHERE id=$1`, id, status, message, string(raw), errText)
	return err
}

func (s Store) ListJobs(ctx context.Context, scope string, limit int) ([]Job, error) {
	if err := s.ensureDB(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id,scope,step_id,job_type,status,message,request_json,result_json,error_text,created_by,created_at,started_at,finished_at,updated_at
		FROM doc_benchmark_admin_jobs WHERE scope=$1 ORDER BY id DESC LIMIT $2`, scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var job Job
		var req, result json.RawMessage
		if err := rows.Scan(&job.ID, &job.Scope, &job.StepID, &job.JobType, &job.Status, &job.Message, &req, &result, &job.ErrorText, &job.CreatedBy, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(req, &job.Request)
		_ = json.Unmarshal(result, &job.Result)
		jobs = append(jobs, job)
	}
	if jobs == nil {
		jobs = []Job{}
	}
	return jobs, rows.Err()
}

func (s Store) GetJob(ctx context.Context, id int64) (Job, error) {
	if err := s.ensureDB(); err != nil {
		return Job{}, err
	}
	var job Job
	var req, result json.RawMessage
	err := s.DB.QueryRowContext(ctx, `SELECT id,scope,step_id,job_type,status,message,request_json,result_json,error_text,created_by,created_at,started_at,finished_at,updated_at
		FROM doc_benchmark_admin_jobs WHERE id=$1`, id).
		Scan(&job.ID, &job.Scope, &job.StepID, &job.JobType, &job.Status, &job.Message, &req, &result, &job.ErrorText, &job.CreatedBy, &job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	if err != nil {
		return Job{}, err
	}
	_ = json.Unmarshal(req, &job.Request)
	_ = json.Unmarshal(result, &job.Result)
	return job, nil
}

func defaultConfig() Config {
	return Config{
		Scope:            DefaultScope,
		DatasetRoot:      "benchmark/doc-processors/datasets",
		WorkRoot:         ".benchmark/work",
		EvidenceRoot:     ".benchmark/evidence",
		StoreID:          1,
		TenantID:         "benchmark",
		ReportFormat:     "markdown",
		MetricsBaseline:  "metrics-baseline",
		MetricsCandidate: "metrics-alt",
		ChunkBaseline:    "chunk-small",
		ChunkCandidate:   "chunk-large",
	}
}
