package docbenchmark

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

func (s SQLStore) AttachRunProvenance(ctx context.Context, runID, gitCommit, jjChange, executable, executableHash string, dirty bool, concurrency int) error {
	if err := checkDB(s); err != nil {
		return err
	}
	res, err := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_runs SET git_commit=NULLIF($2,''),jj_change=NULLIF($3,''),executable=NULLIF($4,''),executable_hash=NULLIF($5,''),dirty=$6,concurrency=$7,updated_at=now() WHERE id=$1 AND lifecycle IN ('queued','running') AND (executable_hash IS NULL OR (executable_hash=$5 AND dirty=$6))`, runID, gitCommit, jjChange, executable, executableHash, dirty, concurrency)
	if err != nil {
		return err
	}
	if err := affected(res); err != nil {
		var lifecycle string
		var storedDirty bool
		var storedHash sql.NullString
		if qerr := s.DB.QueryRowContext(txctx(ctx), `SELECT lifecycle,dirty,executable_hash FROM kb.benchmark_runs WHERE id=$1`, runID).Scan(&lifecycle, &storedDirty, &storedHash); qerr == nil {
			if (lifecycle == "queued" || lifecycle == "running") && storedHash.Valid && (storedHash.String != executableHash || storedDirty != dirty) {
				return fmt.Errorf("benchmark run %s cannot resume: stored provenance dirty=%t executable_hash=%q does not match current dirty=%t executable_hash=%q; start a fresh experiment (for example by changing the experiment name) or delete the stale benchmark rows", runID, storedDirty, storedHash.String, dirty, executableHash)
			}
		}
		return err
	}
	return nil
}

func (s SQLStore) AttachResolvedRuntime(ctx context.Context, runID string, snapshot []byte, hash string) error {
	if err := checkDB(s); err != nil {
		return err
	}
	canonical, err := canonicalJSON(snapshot)
	if err != nil {
		return err
	}
	res, err := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_runs SET resolved_json=$2,config_json=$2,resolved_hash=$3,config_hash=$3,updated_at=now() WHERE id=$1 AND lifecycle IN ('queued','running') AND (config_hash IS NULL OR (config_hash=$3 AND config_json=$2))`, runID, canonical, hash)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s SQLStore) MarkRunRunning(ctx context.Context, runID string) error {
	if err := checkDB(s); err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_runs SET lifecycle='running',started_at=COALESCE(started_at,$2),updated_at=$2 WHERE id=$1 AND lifecycle IN ('queued','running')`, runID, now)
	if err != nil {
		return err
	}
	return affected(res)
}

func (s SQLStore) FinalizeRunIfComplete(ctx context.Context, runID string) (bool, error) {
	if err := checkDB(s); err != nil {
		return false, err
	}
	var total, terminal int
	err := s.DB.QueryRowContext(txctx(ctx), `SELECT count(*),count(*) FILTER (WHERE lifecycle NOT IN ('pending','running')) FROM kb.benchmark_case_runs WHERE run_id=$1`, runID).Scan(&total, &terminal)
	if err != nil {
		return false, err
	}
	if total == 0 || total != terminal {
		return false, nil
	}
	now := time.Now().UTC()
	res, err := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_runs SET lifecycle=CASE WHEN NOT EXISTS (SELECT 1 FROM kb.benchmark_case_runs WHERE run_id=$1 AND lifecycle<>'success') THEN 'succeeded' WHEN EXISTS (SELECT 1 FROM kb.benchmark_case_runs WHERE run_id=$1 AND lifecycle='canceled') THEN 'canceled' ELSE 'failed' END,finished_at=$2,updated_at=$2 WHERE id=$1 AND lifecycle IN ('queued','running')`, runID, now)
	if err != nil {
		return false, err
	}
	if err := affected(res); err != nil {
		return false, err
	}
	return true, nil
}

func (s SQLStore) LoadVerifiedEvidence(ctx context.Context, sourceAttemptID string) (EvidenceBundle, error) {
	if err := checkDB(s); err != nil {
		return EvidenceBundle{}, err
	}
	var path, hash string
	var size int64
	err := s.DB.QueryRowContext(txctx(ctx), `SELECT a.path,a.sha256,a.size_bytes FROM kb.benchmark_artifacts a JOIN kb.benchmark_case_attempts at ON at.id=a.attempt_id WHERE a.attempt_id=$1 AND at.kind='execution' AND at.capture_verified=true AND a.kind='evidence_bundle' AND a.verified=true`, sourceAttemptID).Scan(&path, &hash, &size)
	if err == sql.ErrNoRows {
		return EvidenceBundle{}, fmt.Errorf("verified source evidence %s: %w", sourceAttemptID, ErrNotFound)
	}
	if err != nil {
		return EvidenceBundle{}, err
	}
	bundle, err := loadVerifiedEvidence(path, hash, size)
	if err != nil {
		return EvidenceBundle{}, err
	}
	if bundle.AttemptID != sourceAttemptID {
		return EvidenceBundle{}, fmt.Errorf("source evidence attempt mismatch")
	}
	return bundle, nil
}
