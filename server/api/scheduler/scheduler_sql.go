package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	statusRunning = "running"
)

// SQLStore implements Store (and the CRUD/history operations the admin page
// needs) against kb.scheduled_jobs / kb.scheduled_job_runs.
type SQLStore struct {
	DB *sql.DB
}

// DueSchedules loads enabled schedules whose next_run_at has arrived.
func (s SQLStore) DueSchedules(ctx context.Context, now time.Time, limit int) ([]Schedule, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, name, job_type, interval_seconds, params, enabled, run_once, next_run_at
FROM kb.scheduled_jobs
WHERE enabled = true AND next_run_at <= $1
ORDER BY next_run_at
LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Schedule
	for rows.Next() {
		var (
			sched     Schedule
			paramsRaw []byte
		)
		if err := rows.Scan(&sched.ID, &sched.Name, &sched.JobType, &sched.IntervalSeconds, &paramsRaw, &sched.Enabled, &sched.RunOnce, &sched.NextRunAt); err != nil {
			return nil, err
		}
		sched.Params = parseParams(paramsRaw)
		out = append(out, sched)
	}
	return out, rows.Err()
}

// StartRun inserts a 'running' row and returns its id.
func (s SQLStore) StartRun(ctx context.Context, scheduleID int64, jobType string) (int64, error) {
	if s.DB == nil {
		return 0, fmt.Errorf("db is nil")
	}
	var runID int64
	err := s.DB.QueryRowContext(ctx, `
INSERT INTO kb.scheduled_job_runs (schedule_id, job_type, status)
VALUES ($1, $2, $3)
RETURNING id`, scheduleID, jobType, statusRunning).Scan(&runID)
	return runID, err
}

// FinishRun records the outcome of a started run.
func (s SQLStore) FinishRun(ctx context.Context, runID int64, status string, result map[string]any, runErr error) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	errMsg := ""
	if runErr != nil {
		errMsg = runErr.Error()
	}
	_, err = s.DB.ExecContext(ctx, `
UPDATE kb.scheduled_job_runs
SET status = $1, result = $2::jsonb, error = $3, finished_at = NOW()
WHERE id = $4`, status, string(resultJSON), nullEmptyString(errMsg), runID)
	return err
}

// AdvanceSchedule sets the next run time, whether the schedule stays
// enabled (false for a run-once schedule that just ran — see RunDueSchedules),
// and last-run bookkeeping.
func (s SQLStore) AdvanceSchedule(ctx context.Context, scheduleID int64, nextRunAt time.Time, enabled bool, status string) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := s.DB.ExecContext(ctx, `
UPDATE kb.scheduled_jobs
SET next_run_at = $1, enabled = $2, last_run_status = $3, last_run_at = NOW()
WHERE id = $4`, nextRunAt, enabled, status, scheduleID)
	return err
}

// CreateScheduleInput is the admin page's "add schedule" form payload.
// RunOnce jobs use IntervalSeconds only as the one-time delay before their
// single run ("run in 30 minutes") — see Schedule's doc comment.
type CreateScheduleInput struct {
	Name            string
	JobType         string
	IntervalSeconds int
	Params          map[string]any
	Enabled         bool
	RunOnce         bool
}

// CreateSchedule inserts a new schedule. A recurring schedule is due
// immediately (next_run_at = NOW()), unchanged from before RunOnce existed.
// A RunOnce schedule is due after its one-time delay
// (next_run_at = NOW() + IntervalSeconds) — "run the job in 30 minutes."
func (s SQLStore) CreateSchedule(ctx context.Context, input CreateScheduleInput) (Schedule, error) {
	if s.DB == nil {
		return Schedule{}, fmt.Errorf("db is nil")
	}
	paramsJSON, err := json.Marshal(input.Params)
	if err != nil {
		return Schedule{}, err
	}
	initialDelaySeconds := 0
	if input.RunOnce {
		initialDelaySeconds = input.IntervalSeconds
	}
	var id int64
	err = s.DB.QueryRowContext(ctx, `
INSERT INTO kb.scheduled_jobs (name, job_type, interval_seconds, params, enabled, run_once, next_run_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, NOW() + ($7 || ' seconds')::interval)
RETURNING id`, input.Name, input.JobType, input.IntervalSeconds, string(paramsJSON), input.Enabled, input.RunOnce, initialDelaySeconds).Scan(&id)
	if err != nil {
		return Schedule{}, err
	}
	return Schedule{
		ID: id, Name: input.Name, JobType: input.JobType, IntervalSeconds: input.IntervalSeconds,
		Params: input.Params, Enabled: input.Enabled, RunOnce: input.RunOnce,
	}, nil
}

// ListSchedules returns every schedule, most recently created first.
func (s SQLStore) ListSchedules(ctx context.Context) ([]Schedule, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, name, job_type, interval_seconds, params, enabled, run_once, next_run_at, last_run_at, COALESCE(last_run_status, '')
FROM kb.scheduled_jobs
ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Schedule
	for rows.Next() {
		var (
			sched     Schedule
			paramsRaw []byte
			lastRunAt sql.NullTime
		)
		if err := rows.Scan(&sched.ID, &sched.Name, &sched.JobType, &sched.IntervalSeconds, &paramsRaw, &sched.Enabled, &sched.RunOnce, &sched.NextRunAt, &lastRunAt, &sched.LastRunStatus); err != nil {
			return nil, err
		}
		sched.Params = parseParams(paramsRaw)
		if lastRunAt.Valid {
			t := lastRunAt.Time
			sched.LastRunAt = &t
		}
		out = append(out, sched)
	}
	return out, rows.Err()
}

// UpdateScheduleInput is the admin page's "edit schedule" payload. All
// fields are applied — this is a full replace of the editable fields, the
// same convention as this codebase's other single-row PATCH handlers
// (e.g. UpdateArtifactObject). Note: editing does not recompute next_run_at
// — changing IntervalSeconds or RunOnce takes effect starting from the
// schedule's current next_run_at, not a fresh NOW()-based delay.
type UpdateScheduleInput struct {
	Name            string
	IntervalSeconds int
	Params          map[string]any
	Enabled         bool
	RunOnce         bool
}

// UpdateSchedule applies an edit to an existing schedule.
func (s SQLStore) UpdateSchedule(ctx context.Context, id int64, input UpdateScheduleInput) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	paramsJSON, err := json.Marshal(input.Params)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
UPDATE kb.scheduled_jobs
SET name = $1, interval_seconds = $2, params = $3::jsonb, enabled = $4, run_once = $5
WHERE id = $6`, input.Name, input.IntervalSeconds, string(paramsJSON), input.Enabled, input.RunOnce, id)
	return err
}

// DeleteSchedule removes a schedule; its run history is cascade-deleted.
func (s SQLStore) DeleteSchedule(ctx context.Context, id int64) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
	}
	_, err := s.DB.ExecContext(ctx, `DELETE FROM kb.scheduled_jobs WHERE id = $1`, id)
	return err
}

// Run is one row of kb.scheduled_job_runs, for the admin page's history view.
type Run struct {
	ID         int64
	ScheduleID int64
	JobType    string
	Status     string
	StartedAt  time.Time
	FinishedAt *time.Time
	Result     map[string]any
	Error      string
}

// ListRuns returns run history for one schedule, most recent first.
func (s SQLStore) ListRuns(ctx context.Context, scheduleID int64, limit int) ([]Run, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("db is nil")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.DB.QueryContext(ctx, `
SELECT id, schedule_id, job_type, status, started_at, finished_at, COALESCE(result, '{}'::jsonb), COALESCE(error, '')
FROM kb.scheduled_job_runs
WHERE schedule_id = $1
ORDER BY started_at DESC
LIMIT $2`, scheduleID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Run
	for rows.Next() {
		var (
			run        Run
			resultRaw  []byte
			finishedAt sql.NullTime
		)
		if err := rows.Scan(&run.ID, &run.ScheduleID, &run.JobType, &run.Status, &run.StartedAt, &finishedAt, &resultRaw, &run.Error); err != nil {
			return nil, err
		}
		run.Result = parseParams(resultRaw)
		if finishedAt.Valid {
			t := finishedAt.Time
			run.FinishedAt = &t
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func parseParams(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func nullEmptyString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
