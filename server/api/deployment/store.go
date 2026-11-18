package deployment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrActiveRun means a build (or deployment) is already running on this machine.
var ErrActiveRun = errors.New("a run is already active")

// ErrRunnerExited is the error stored for a run whose runner died without reporting.
// The page shows a translated message for this exact text.
const ErrRunnerExited = "runner exited without reporting"

// Run is one build (kb.deployment_builds) or deployment (kb.deployments).
type Run struct {
	ID                  int64      `json:"id"`
	Kind                string     `json:"kind"`
	BuildNumber         string     `json:"build_number"`
	BuildName           string     `json:"build_name,omitempty"`
	TargetMachine       string     `json:"target_machine,omitempty"`
	OSInfo              string     `json:"os_info,omitempty"`
	Description         string     `json:"description,omitempty"`
	ArchiveName         string     `json:"archive_name"`
	PreviousBuildNumber string     `json:"previous_build_number,omitempty"`
	Status              string     `json:"status"`
	ErrorMessage        string     `json:"error_message"`
	RequestedBy         string     `json:"requested_by"`
	RunnerHost          string     `json:"runner_host,omitempty"`
	RunnerPID           int        `json:"runner_pid"`
	RequestedAt         *time.Time `json:"requested_at,omitempty"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	Steps               []Step     `json:"steps,omitempty"`
}

const buildColumns = `id, COALESCE(build_number, ''), build_name, target_machine, os_info, description, archive_name,
	status, error_message, requested_by, runner_host, runner_pid, started_at, finished_at`

const deploymentColumns = `id, archive_name, build_number, previous_build_number, status, error_message,
	requested_by, runner_pid, requested_at, started_at, finished_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanBuild(row rowScanner) (Run, error) {
	r := Run{Kind: KindBuild}
	var started time.Time
	var finished sql.NullTime
	err := row.Scan(&r.ID, &r.BuildNumber, &r.BuildName, &r.TargetMachine, &r.OSInfo, &r.Description, &r.ArchiveName,
		&r.Status, &r.ErrorMessage, &r.RequestedBy, &r.RunnerHost, &r.RunnerPID, &started, &finished)
	r.StartedAt = &started
	r.FinishedAt = nullTime(finished)
	return r, err
}

func scanDeployment(row rowScanner) (Run, error) {
	r := Run{Kind: KindDeploy}
	var requested time.Time
	var started, finished sql.NullTime
	err := row.Scan(&r.ID, &r.ArchiveName, &r.BuildNumber, &r.PreviousBuildNumber, &r.Status, &r.ErrorMessage,
		&r.RequestedBy, &r.RunnerPID, &requested, &started, &finished)
	r.RequestedAt = &requested
	r.StartedAt = nullTime(started)
	r.FinishedAt = nullTime(finished)
	return r, err
}

func nullTime(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

func runTable(kind string) (table, columns, fkColumn string, scan func(rowScanner) (Run, error)) {
	if kind == KindBuild {
		return "kb.deployment_builds", buildColumns, "build_id", scanBuild
	}
	return "kb.deployments", deploymentColumns, "deployment_id", scanDeployment
}

func activeError(err error) error {
	if err != nil && strings.Contains(err.Error(), "_one_active") {
		return ErrActiveRun
	}
	return err
}

func insertPlannedSteps(ctx context.Context, tx *sql.Tx, kind string, runID int64) error {
	_, _, fk, _ := runTable(kind)
	for i, p := range Plan(kind) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO kb.deployment_steps (`+fk+`, seq, step_key, step_arg) VALUES ($1, $2, $3, $4)`,
			runID, i+1, p.Key, p.Arg); err != nil {
			return fmt.Errorf("insert planned step %s: %w", p.Key, err)
		}
	}
	return nil
}

// CreateBuild inserts a running build with all planned steps pending.
func CreateBuild(ctx context.Context, db *sql.DB, r Run) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO kb.deployment_builds
		(build_number, build_name, target_machine, os_info, description, status, requested_by, runner_host)
		VALUES (NULLIF($1, ''), $2, $3, $4, $5, 'running', $6, $7) RETURNING id`,
		r.BuildNumber, r.BuildName, r.TargetMachine, r.OSInfo, r.Description, r.RequestedBy, r.RunnerHost).Scan(&id)
	if err != nil {
		return 0, activeError(err)
	}
	if err := insertPlannedSteps(ctx, tx, KindBuild, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// CreateDeployment inserts a queued deployment with all planned steps pending.
func CreateDeployment(ctx context.Context, db *sql.DB, r Run) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `INSERT INTO kb.deployments
		(archive_name, build_number, previous_build_number, status, requested_by)
		VALUES ($1, $2, $3, 'queued', $4) RETURNING id`,
		r.ArchiveName, r.BuildNumber, r.PreviousBuildNumber, r.RequestedBy).Scan(&id)
	if err != nil {
		return 0, activeError(err)
	}
	if err := insertPlannedSteps(ctx, tx, KindDeploy, id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// SetBuildRunner records the process that runs a build.
func SetBuildRunner(ctx context.Context, db *sql.DB, id int64, pid int) error {
	_, err := db.ExecContext(ctx, `UPDATE kb.deployment_builds SET runner_pid = $2 WHERE id = $1`, id, pid)
	return err
}

// ClaimDeployment marks the oldest queued deployment running and returns it.
// ok is false when nothing is queued.
func ClaimDeployment(ctx context.Context, db *sql.DB, pid int) (run Run, ok bool, err error) {
	row := db.QueryRowContext(ctx, `UPDATE kb.deployments SET status = 'running', started_at = NOW(), runner_pid = $1
		WHERE id = (SELECT id FROM kb.deployments WHERE status = 'queued' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED)
		RETURNING `+deploymentColumns, pid)
	run, err = scanDeployment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	return run, err == nil, err
}

// SaveSteps writes step states; steps beyond the plan are inserted.
func SaveSteps(ctx context.Context, db *sql.DB, kind string, runID int64, steps []Step) error {
	_, _, fk, _ := runTable(kind)
	for _, s := range steps {
		_, err := db.ExecContext(ctx, `INSERT INTO kb.deployment_steps
			(`+fk+`, seq, step_key, step_arg, status, started_at, finished_at, output, output_truncated)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (`+fk+`, seq) WHERE `+fk+` IS NOT NULL DO UPDATE SET
				step_key = EXCLUDED.step_key, step_arg = EXCLUDED.step_arg, status = EXCLUDED.status, started_at = EXCLUDED.started_at, finished_at = EXCLUDED.finished_at,
				output = EXCLUDED.output, output_truncated = EXCLUDED.output_truncated`,
			runID, s.Seq, s.Key, s.Arg, s.Status, s.StartedAt, s.FinishedAt, s.Output, s.OutputTruncated)
		if err != nil {
			return fmt.Errorf("save step %d (%s): %w", s.Seq, s.Key, err)
		}
	}
	return nil
}

// FinishRun records a run's final status and, for a new build archive, its name and number.
func FinishRun(ctx context.Context, db *sql.DB, kind string, id int64, status, errorMessage, archiveName string) error {
	table, _, _, _ := runTable(kind)
	_, err := db.ExecContext(ctx, `UPDATE `+table+` SET status = $2, error_message = $3, finished_at = NOW(),
		archive_name = COALESCE(NULLIF($4, ''), archive_name),
		build_number = COALESCE(NULLIF($5, ''), build_number) WHERE id = $1`,
		id, status, errorMessage, archiveName, ArchiveBuildNumber(archiveName))
	return err
}

// FailRun marks an active run and its unfinished steps failed (running) or skipped (pending).
// It does nothing when the run has already finished.
func FailRun(ctx context.Context, db *sql.DB, kind string, id int64, errorMessage string) error {
	table, _, fk, _ := runTable(kind)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE `+table+` SET status = 'failed', error_message = $2, finished_at = NOW()
		WHERE id = $1 AND status IN ('queued', 'running')`, id, errorMessage)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE kb.deployment_steps SET finished_at = NOW(),
		status = CASE WHEN status = 'running' THEN 'failed' ELSE 'skipped' END
		WHERE `+fk+` = $1 AND status IN ('running', 'pending')`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ActiveRun returns the queued or running run, or nil.
func ActiveRun(ctx context.Context, db *sql.DB, kind string) (*Run, error) {
	table, columns, _, scan := runTable(kind)
	r, err := scan(db.QueryRowContext(ctx, `SELECT `+columns+` FROM `+table+
		` WHERE status IN ('queued', 'running') ORDER BY id DESC LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RecentRuns returns the latest runs, newest first, without steps.
func RecentRuns(ctx context.Context, db *sql.DB, kind string, limit int) ([]Run, error) {
	table, columns, _, scan := runTable(kind)
	rows, err := db.QueryContext(ctx, `SELECT `+columns+` FROM `+table+` ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []Run{}
	for rows.Next() {
		r, err := scan(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetRun returns one run with its steps; nil when it does not exist.
func GetRun(ctx context.Context, db *sql.DB, kind string, id int64) (*Run, error) {
	table, columns, fk, scan := runTable(kind)
	r, err := scan(db.QueryRowContext(ctx, `SELECT `+columns+` FROM `+table+` WHERE id = $1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT seq, step_key, step_arg, status, started_at, finished_at, output, output_truncated
		FROM kb.deployment_steps WHERE `+fk+` = $1 ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	r.Steps = []Step{}
	for rows.Next() {
		var s Step
		var started, finished sql.NullTime
		if err := rows.Scan(&s.Seq, &s.Key, &s.Arg, &s.Status, &started, &finished, &s.Output, &s.OutputTruncated); err != nil {
			return nil, err
		}
		s.StartedAt, s.FinishedAt = nullTime(started), nullTime(finished)
		r.Steps = append(r.Steps, s)
	}
	return &r, rows.Err()
}
