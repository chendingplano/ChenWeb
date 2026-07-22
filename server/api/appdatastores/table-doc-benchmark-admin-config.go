package appdatastores

import (
	"fmt"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/databaseutil"
)

func CreateDocBenchmarkAdminConfigTable(logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	stmt := `CREATE TABLE IF NOT EXISTS doc_benchmark_admin_config (
		scope TEXT PRIMARY KEY,
		experiment_path TEXT NOT NULL DEFAULT '',
		dataset_root TEXT NOT NULL DEFAULT 'benchmark/doc-processors/datasets',
		artifact_root TEXT NOT NULL DEFAULT '',
		work_root TEXT NOT NULL DEFAULT '.benchmark/work',
		evidence_root TEXT NOT NULL DEFAULT '.benchmark/evidence',
		store_id BIGINT NOT NULL DEFAULT 1,
		owner TEXT NOT NULL DEFAULT '',
		tenant_id TEXT NOT NULL DEFAULT 'benchmark',
		metrics_model_name TEXT NOT NULL DEFAULT '',
		allow_dirty BOOLEAN NOT NULL DEFAULT false,
		report_format TEXT NOT NULL DEFAULT 'markdown',
		report_output_path TEXT NOT NULL DEFAULT '',
		metrics_baseline TEXT NOT NULL DEFAULT 'metrics-baseline',
		metrics_candidate TEXT NOT NULL DEFAULT 'metrics-alt',
		chunk_baseline TEXT NOT NULL DEFAULT 'chunk-small',
		chunk_candidate TEXT NOT NULL DEFAULT 'chunk-large',
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`
	if err := databaseutil.ExecuteStatement(db, stmt); err != nil {
		return fmt.Errorf("failed creating doc_benchmark_admin_config table: %w", err)
	}
	logger.Info("Created doc_benchmark_admin_config table")
	return nil
}
