package appdatastores

import (
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

func CreateDocBenchmarkAdminJobsTable(logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	stmt := `CREATE TABLE IF NOT EXISTS doc_benchmark_admin_jobs (
		id BIGSERIAL PRIMARY KEY,
		scope TEXT NOT NULL DEFAULT 'default',
		step_id TEXT NOT NULL DEFAULT '',
		job_type TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'queued',
		message TEXT NOT NULL DEFAULT '',
		request_json JSONB NOT NULL DEFAULT '{}'::jsonb,
		result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
		error_text TEXT NOT NULL DEFAULT '',
		created_by TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		started_at TIMESTAMPTZ,
		finished_at TIMESTAMPTZ,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating doc_benchmark_admin_jobs table: %w", err)
	}
	logger.Info("Created doc_benchmark_admin_jobs table")
	return nil
}
