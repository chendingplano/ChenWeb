package semantic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Retry job states.
const (
	RetryJobPending = "pending"
	RetryJobClaimed = "claimed"
	RetryJobDone    = "done"
	RetryJobStale   = "stale"
	RetryJobFailed  = "failed"
)

// RetryJob is one row of kb.semantic_retry_queue. The Outcome* fields are
// read-only context joined in by List for a diagnostic reader; Enqueue,
// Claim, and find leave them zero-valued.
type RetryJob struct {
	ID                          int64
	OutcomeID                   int64
	FindingID                   *int64
	TargetDependencyFingerprint string
	SourceInputFingerprint      string
	State                       string
	Attempts                    int
	LeaseToken                  string
	LeaseExpiresAt              *time.Time
	LastError                   string
	CreateTime                  time.Time
	ModifyTime                  time.Time

	OutcomeInputRecordID *int64
	OutcomeArtifactType  string
	OutcomeArtifactID    string
	OutcomeStageTermID   string
}

// RetryQueue implements DR10's dependency-driven semantic retry.
//
// This is deliberately NOT the failed-processor queue. Re-running a processor
// cannot resolve "no approved governed mapping exists"; only a change to the
// mapping can. Keying jobs on the dependency they are waiting for is what
// makes "schedule only affected outcomes" and "an unchanged fingerprint
// produces no work" expressible at all.
type RetryQueue struct {
	DB *sql.DB
}

// Enqueue registers a retry for one outcome, optionally scoped to one finding.
// Concurrent enqueues of the same target are an idempotent conflict (DR10):
// uq_semantic_retry_queue_target uses NULLS NOT DISTINCT so two whole-stage
// enqueues collide rather than inserting twice.
func (q RetryQueue) Enqueue(ctx context.Context, job RetryJob) (RetryJob, bool, error) {
	if q.DB == nil {
		return RetryJob{}, false, fmt.Errorf("db is nil")
	}
	if job.TargetDependencyFingerprint == "" {
		return RetryJob{}, false, fmt.Errorf("target dependency fingerprint is required (DR10)")
	}
	row := q.DB.QueryRowContext(ctx, `
INSERT INTO kb.semantic_retry_queue (
  outcome_id, finding_id, target_dependency_fingerprint, source_input_fingerprint, state, create_by)
VALUES ($1,$2,$3,$4,'pending',$5)
ON CONFLICT (outcome_id, finding_id, target_dependency_fingerprint) DO NOTHING
RETURNING `+retryColumns,
		job.OutcomeID, job.FindingID, job.TargetDependencyFingerprint,
		nullIfEmpty(job.SourceInputFingerprint), "semantic_retry_queue")
	created, err := scanRetryJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		// DO NOTHING returned no row: the job already exists. Return it so the
		// caller sees an idempotent no-op rather than an error.
		existing, err := q.find(ctx, job)
		if err != nil {
			return RetryJob{}, false, err
		}
		return existing, true, nil
	}
	if err != nil {
		return RetryJob{}, false, err
	}
	return created, false, nil
}

func (q RetryQueue) find(ctx context.Context, job RetryJob) (RetryJob, error) {
	row := q.DB.QueryRowContext(ctx, `
SELECT `+retryColumns+` FROM kb.semantic_retry_queue
WHERE outcome_id = $1 AND finding_id IS NOT DISTINCT FROM $2
  AND target_dependency_fingerprint = $3`,
		job.OutcomeID, job.FindingID, job.TargetDependencyFingerprint)
	return scanRetryJob(row)
}

// Claim takes one claimable job under an expiring lease using FOR UPDATE SKIP
// LOCKED. Concurrent execution therefore cannot activate two outcomes for the
// same target (DR10), and an expired lease makes a crashed worker's job
// claimable again.
func (q RetryQueue) Claim(ctx context.Context, leaseToken string, leaseFor time.Duration) (RetryJob, error) {
	if leaseToken == "" {
		return RetryJob{}, fmt.Errorf("lease token is required")
	}
	tx, err := q.DB.BeginTx(ctx, nil)
	if err != nil {
		return RetryJob{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
SELECT `+retryColumns+` FROM kb.semantic_retry_queue
WHERE state IN ('pending', 'claimed')
  AND (lease_expires_at IS NULL OR lease_expires_at < NOW())
ORDER BY id
FOR UPDATE SKIP LOCKED
LIMIT 1`)
	job, err := scanRetryJob(row)
	if err != nil {
		return RetryJob{}, err
	}
	expires := time.Now().Add(leaseFor)
	if _, err := tx.ExecContext(ctx, `
UPDATE kb.semantic_retry_queue
SET state = 'claimed', lease_token = $2, lease_expires_at = $3,
    attempts = attempts + 1, modify_time = NOW()
WHERE id = $1`, job.ID, leaseToken, expires); err != nil {
		return RetryJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return RetryJob{}, err
	}
	job.State = RetryJobClaimed
	job.LeaseToken = leaseToken
	job.LeaseExpiresAt = &expires
	job.Attempts++
	return job, nil
}

// StalenessCheck is what a worker evaluates before doing any semantic work.
type StalenessCheck struct {
	CurrentInputFingerprint      string
	CurrentDependencyFingerprint string
}

// IsStale implements DR10: "A stale job whose source input or target dependency
// no longer matches records stale and performs no semantic writes."
//
// Both axes matter. A job can go stale because the source was re-extracted
// (input axis) or because the dependency it was waiting for has already moved
// past the value it targeted (dependency axis) -- acting on either would write
// a semantic result derived from state that no longer exists.
func (j RetryJob) IsStale(check StalenessCheck) bool {
	if j.SourceInputFingerprint != "" && j.SourceInputFingerprint != check.CurrentInputFingerprint {
		return true
	}
	return j.TargetDependencyFingerprint != check.CurrentDependencyFingerprint
}

// MarkStale records a job as stale. The caller must not have written any
// semantic rows before calling this.
func (q RetryQueue) MarkStale(ctx context.Context, jobID int64, reason string) error {
	_, err := q.DB.ExecContext(ctx, `
UPDATE kb.semantic_retry_queue
SET state = 'stale', last_error = $2, lease_token = NULL, lease_expires_at = NULL, modify_time = NOW()
WHERE id = $1`, jobID, nullIfEmpty(reason))
	return err
}

// MarkDone records successful completion.
func (q RetryQueue) MarkDone(ctx context.Context, jobID int64) error {
	_, err := q.DB.ExecContext(ctx, `
UPDATE kb.semantic_retry_queue
SET state = 'done', lease_token = NULL, lease_expires_at = NULL, modify_time = NOW()
WHERE id = $1`, jobID)
	return err
}

// MarkFailed records an execution failure so the job can be re-claimed.
func (q RetryQueue) MarkFailed(ctx context.Context, jobID int64, cause string) error {
	_, err := q.DB.ExecContext(ctx, `
UPDATE kb.semantic_retry_queue
SET state = 'pending', last_error = $2, lease_token = NULL, lease_expires_at = NULL, modify_time = NOW()
WHERE id = $1`, jobID, nullIfEmpty(cause))
	return err
}

// ScheduleForDependencyChange enqueues retries for exactly the active outcomes
// whose current dependency fingerprint differs from the new target (DR10:
// "When a dependency changes, the system schedules only affected outcomes. An
// unchanged fingerprint reuses the existing outcome and does not generate
// repeated work or alerts.").
//
// The `<> $3` predicate is what implements "only affected": an outcome already
// at the target fingerprint is skipped entirely, so a no-op sweep enqueues
// nothing and emits no alert.
func (q RetryQueue) ScheduleForDependencyChange(ctx context.Context, findingTermID, targetFingerprint string) (int, error) {
	if err := ValidateGovernedIdentifier(findingTermID); err != nil {
		return 0, fmt.Errorf("finding term: %w", err)
	}
	res, err := q.DB.ExecContext(ctx, `
INSERT INTO kb.semantic_retry_queue (
  outcome_id, finding_id, target_dependency_fingerprint, source_input_fingerprint, state, create_by)
SELECT o.id, f.id, $2, o.input_fingerprint, 'pending', 'dependency_change'
FROM kb.semantic_processing_outcomes o
JOIN kb.semantic_processing_findings f ON f.outcome_id = o.id AND f.active = true
WHERE o.active = true
  AND f.finding_term_id = $1
  AND f.dependency_fingerprint <> $2
ON CONFLICT (outcome_id, finding_id, target_dependency_fingerprint) DO NOTHING`,
		findingTermID, targetFingerprint)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// PendingByDependency reports queue size grouped by target dependency, one of
// the required pre-cutover reports (ADR section 6).
func (q RetryQueue) PendingByDependency(ctx context.Context) (map[string]int, error) {
	rows, err := q.DB.QueryContext(ctx, `
SELECT target_dependency_fingerprint, count(*)
FROM kb.semantic_retry_queue WHERE state = 'pending' GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// RetryJobFilter selects rows for List, the diagnostic reader over
// kb.semantic_retry_queue (task 5.2's "retry tooling" consumer). This queue
// has no accepted/represented distinction of its own -- every job is
// diagnostic by nature, so List has no accepted-only mode.
type RetryJobFilter struct {
	State     string
	OutcomeID int64
	Page      int
	PageSize  int
}

// List returns retry jobs joined with their outcome's artifact identity, so
// an operator can see what a queued retry is waiting to resolve without a
// second lookup.
func (q RetryQueue) List(ctx context.Context, f RetryJobFilter) ([]RetryJob, int64, error) {
	if q.DB == nil {
		return nil, 0, fmt.Errorf("db is nil")
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 50
	}
	if f.PageSize > 200 {
		f.PageSize = 200
	}
	where := []string{"1=1"}
	args := []any{}
	if f.State != "" {
		args = append(args, f.State)
		where = append(where, fmt.Sprintf("q.state = $%d", len(args)))
	}
	if f.OutcomeID > 0 {
		args = append(args, f.OutcomeID)
		where = append(where, fmt.Sprintf("q.outcome_id = $%d", len(args)))
	}
	from := `FROM kb.semantic_retry_queue q LEFT JOIN kb.semantic_processing_outcomes o ON o.id = q.outcome_id WHERE ` + strings.Join(where, " AND ")

	var total int64
	if err := q.DB.QueryRowContext(ctx, "SELECT COUNT(*) "+from, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := q.DB.QueryContext(ctx, `
SELECT q.id, q.outcome_id, q.finding_id, q.target_dependency_fingerprint,
  coalesce(q.source_input_fingerprint,''), q.state, q.attempts,
  coalesce(q.lease_token,''), q.lease_expires_at, coalesce(q.last_error,''),
  q.create_time, q.modify_time,
  o.input_record_id, coalesce(o.artifact_type,''), coalesce(o.artifact_id,''), coalesce(o.stage_term_id,'')
`+from+`
ORDER BY q.id DESC
LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]RetryJob, 0)
	for rows.Next() {
		var j RetryJob
		if err := rows.Scan(&j.ID, &j.OutcomeID, &j.FindingID, &j.TargetDependencyFingerprint,
			&j.SourceInputFingerprint, &j.State, &j.Attempts,
			&j.LeaseToken, &j.LeaseExpiresAt, &j.LastError, &j.CreateTime, &j.ModifyTime,
			&j.OutcomeInputRecordID, &j.OutcomeArtifactType, &j.OutcomeArtifactID, &j.OutcomeStageTermID); err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}

const retryColumns = `id, outcome_id, finding_id, target_dependency_fingerprint,
	coalesce(source_input_fingerprint,''), state, attempts,
	coalesce(lease_token,''), lease_expires_at, coalesce(last_error,'')`

func scanRetryJob(row rowScanner) (RetryJob, error) {
	var j RetryJob
	if err := row.Scan(&j.ID, &j.OutcomeID, &j.FindingID, &j.TargetDependencyFingerprint,
		&j.SourceInputFingerprint, &j.State, &j.Attempts,
		&j.LeaseToken, &j.LeaseExpiresAt, &j.LastError); err != nil {
		return RetryJob{}, err
	}
	return j, nil
}
