package docbenchmark

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type AttemptRecord struct {
	ID, CaseRunID                                      string
	Number                                             int
	Kind                                               string
	SourceExecutionAttemptID, LeaseOwner               sql.NullString
	InputRecordID                                      sql.NullInt64
	Lifecycle                                          string
	FailureKind                                        sql.NullString
	LeaseExpiresAt, HeartbeatAt, StartedAt, FinishedAt sql.NullTime
	RuntimeMS                                          sql.NullInt64
	Telemetry                                          json.RawMessage
	Provider, Model                                    sql.NullString
	CaptureVerified                                    bool
}
type Claim struct {
	Attempt AttemptRecord
	Claimed bool
}

func (s SQLStore) ClaimAttempt(ctx context.Context, caseRunID, owner string, now time.Time, lease time.Duration, maxAttempts int) (Claim, error) {
	if maxAttempts <= 0 {
		return Claim{}, fmt.Errorf("maxAttempts must be positive")
	}
	if s.DB == nil {
		return Claim{}, fmt.Errorf("nil database")
	}
	tx, e := s.DB.BeginTx(txctx(ctx), nil)
	if e != nil {
		return Claim{}, e
	}
	defer tx.Rollback()
	var lifecycle string
	var selected sql.NullString
	var max int
	e = tx.QueryRowContext(txctx(ctx), `SELECT lifecycle,selected_attempt_id,COALESCE((SELECT max(attempt_number) FROM kb.benchmark_case_attempts WHERE case_run_id=$1),0) FROM kb.benchmark_case_runs WHERE id=$1 FOR UPDATE`, caseRunID).Scan(&lifecycle, &selected, &max)
	if e != nil {
		return Claim{}, e
	}
	if lifecycle != "pending" && lifecycle != "running" {
		if e = tx.Commit(); e != nil {
			return Claim{}, e
		}
		return Claim{Claimed: false}, nil
	}
	now = utc(now)
	if selected.Valid {
		if e = tx.Commit(); e != nil {
			return Claim{}, e
		}
		return Claim{Claimed: false}, nil
	}
	var active int
	if e = tx.QueryRowContext(txctx(ctx), `SELECT count(*) FROM kb.benchmark_case_attempts WHERE case_run_id=$1 AND lifecycle IN ('leased','running') AND (lease_expires_at IS NULL OR lease_expires_at >= $2)`, caseRunID, now).Scan(&active); e != nil {
		return Claim{}, e
	}
	if active > 0 {
		if e = tx.Commit(); e != nil {
			return Claim{}, e
		}
		return Claim{Claimed: false}, nil
	}
	if _, e = tx.ExecContext(txctx(ctx), `UPDATE kb.benchmark_case_attempts SET lifecycle='failed',failure_kind='stale_lease',finished_at=$2,lease_owner=NULL,lease_expires_at=NULL WHERE case_run_id=$1 AND lifecycle IN ('leased','running') AND lease_expires_at < $2`, caseRunID, now); e != nil {
		return Claim{}, e
	}
	if maxAttempts > 0 && max >= maxAttempts {
		if _, e = tx.ExecContext(txctx(ctx), `UPDATE kb.benchmark_case_runs SET selected_attempt_id=(SELECT id FROM kb.benchmark_case_attempts WHERE case_run_id=$1 ORDER BY attempt_number DESC LIMIT 1) WHERE id=$1 AND selected_attempt_id IS NULL`, caseRunID); e != nil {
			return Claim{}, e
		}
		return Claim{Claimed: false}, tx.Commit()
	}
	n := max + 1
	kind := "execution"
	var src any
	var input any
	var verified bool
	e = tx.QueryRowContext(txctx(ctx), `SELECT capture_verified,input_record_id_snapshot FROM kb.benchmark_case_attempts WHERE case_run_id=$1 AND kind='execution' AND capture_verified=true AND lifecycle='failed' AND failure_kind IN ('infrastructure_failed','scorer_failed','stale_lease') ORDER BY attempt_number DESC LIMIT 1`, caseRunID).Scan(&verified, &input)
	if e == nil && verified {
		kind = "rescore"
		e = tx.QueryRowContext(txctx(ctx), `SELECT id FROM kb.benchmark_case_attempts WHERE case_run_id=$1 AND kind='execution' AND capture_verified=true ORDER BY attempt_number DESC LIMIT 1`, caseRunID).Scan(&src)
	}
	if e != nil && e != sql.ErrNoRows {
		return Claim{}, e
	}
	var id string
	exp := now.Add(lease)
	e = tx.QueryRowContext(txctx(ctx), `INSERT INTO kb.benchmark_case_attempts (case_run_id,attempt_number,kind,source_execution_attempt_id,input_record_id_snapshot,lifecycle,lease_owner,lease_expires_at,heartbeat_at,started_at) VALUES ($1,$2,$3,$4,$5,'running',$6,$7,$7,$7) RETURNING id`, caseRunID, n, kind, src, input, owner, exp).Scan(&id)
	if e != nil {
		return Claim{}, e
	}
	if e = tx.Commit(); e != nil {
		return Claim{}, e
	}
	return Claim{Claimed: true, Attempt: AttemptRecord{ID: id, CaseRunID: caseRunID, Number: n, Kind: kind, Lifecycle: "running", LeaseOwner: sql.NullString{String: owner, Valid: true}, LeaseExpiresAt: sql.NullTime{Time: exp, Valid: true}, CaptureVerified: verified}}, nil
}
func (s SQLStore) HeartbeatAttempt(ctx context.Context, id, owner string, until time.Time, telemetry any) error {
	b, err := canonicalJSON(telemetry)
	if err != nil {
		return err
	}
	res, e := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_case_attempts SET heartbeat_at=$3,lease_expires_at=$4,telemetry_json=$5 WHERE id=$1 AND lease_owner=$2 AND lifecycle IN ('leased','running')`, id, owner, utc(time.Now()), utc(until), b)
	if e != nil {
		return e
	}
	return affected(res)
}
func (s SQLStore) FinishAttempt(ctx context.Context, id, owner, lifecycle, failure string, runtimeMS int64, verified bool) error {
	valid := map[string]bool{"succeeded": true, "failed": true, "canceled": true}
	if !valid[lifecycle] {
		return fmt.Errorf("invalid terminal lifecycle %q", lifecycle)
	}
	res, e := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_case_attempts SET lifecycle=$4,failure_kind=NULLIF($5,''),runtime_ms=$6,capture_verified=$7,finished_at=$3,lease_owner=NULL,lease_expires_at=NULL WHERE id=$1 AND lease_owner=$2 AND lifecycle IN ('leased','running')`, id, owner, utc(time.Now()), lifecycle, failure, runtimeMS, verified)
	if e != nil {
		return e
	}
	if err := affected(res); err != nil {
		var l, f string
		if s.DB.QueryRowContext(txctx(ctx), `SELECT lifecycle,COALESCE(failure_kind,'') FROM kb.benchmark_case_attempts WHERE id=$1`, id).Scan(&l, &f) == nil && l == lifecycle && f == failure {
			return nil
		}
		return err
	}
	return nil
}
func (s SQLStore) SelectAttempt(ctx context.Context, caseRunID, attemptID string) error {
	res, e := s.DB.ExecContext(txctx(ctx), `UPDATE kb.benchmark_case_runs SET selected_attempt_id=$2,lifecycle=(SELECT CASE WHEN lifecycle='succeeded' THEN 'success' WHEN lifecycle='failed' THEN CASE failure_kind WHEN 'processor_failed' THEN 'processor_failed' WHEN 'timed_out' THEN 'timed_out' WHEN 'invalid_output' THEN 'invalid_output' WHEN 'scorer_failed' THEN 'scorer_failed' WHEN 'canceled' THEN 'canceled' ELSE 'infrastructure_failed' END ELSE lifecycle END FROM kb.benchmark_case_attempts WHERE id=$2 AND case_run_id=$1 AND lifecycle IN ('succeeded','failed','cancelled')) WHERE id=$1 AND selected_attempt_id IS NULL AND EXISTS (SELECT 1 FROM kb.benchmark_case_attempts WHERE id=$2 AND case_run_id=$1 AND lifecycle IN ('succeeded','failed','cancelled'))`, caseRunID, attemptID)
	if e != nil {
		return e
	}
	if err := affected(res); err != nil {
		var cur sql.NullString
		if s.DB.QueryRowContext(txctx(ctx), `SELECT selected_attempt_id FROM kb.benchmark_case_runs WHERE id=$1`, caseRunID).Scan(&cur) == nil && cur.Valid && cur.String == attemptID {
			return nil
		}
		return err
	}
	return nil
}
