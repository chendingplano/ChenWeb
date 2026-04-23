package agentplatformhandler

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/llm/agentrun"
)

// -----------------------------------------------------------------------------
// Cancellation registry
//
// When a worker starts executing a task it registers its cancel func keyed by
// run id. The HTTP CancelTaskRun handler looks it up and invokes it so the
// running Docker container is torn down promptly. The registry is purely a
// fast-path — DB status is the source of truth.
// -----------------------------------------------------------------------------

var runCancels sync.Map // map[string]context.CancelFunc

func registerCancel(runID string, cancel context.CancelFunc) {
	runCancels.Store(runID, cancel)
}
func unregisterCancel(runID string) { runCancels.Delete(runID) }

// signalCancel invokes the cancel func for a running task if present.
// Safe to call for unknown run ids.
func signalCancel(runID string) {
	if v, ok := runCancels.Load(runID); ok {
		if cancel, ok := v.(context.CancelFunc); ok {
			cancel()
		}
	}
}

// -----------------------------------------------------------------------------
// Worker loop
// -----------------------------------------------------------------------------

type worker struct {
	id          int
	db          *sql.DB
	workdirRoot string
	logger      ApiTypes.JimoLogger
}

// runLoop polls for queued tasks and executes them one at a time.
// It exits when ctx is canceled.
func (w *worker) runLoop(ctx context.Context) {
	w.logger.Info("agent-platform worker started", "id", w.id)
	for {
		select {
		case <-ctx.Done():
			w.logger.Info("agent-platform worker stopping", "id", w.id)
			return
		default:
		}

		run, ok, err := claimNextRun(w.db)
		if err != nil {
			w.logger.Error("claim task failed", "id", w.id, "err", err.Error())
			sleepJitter(ctx, 5*time.Second)
			continue
		}
		if !ok {
			sleepJitter(ctx, 2*time.Second)
			continue
		}
		w.executeRun(ctx, run)
	}
}

// sleepJitter sleeps ~d with ±20% jitter; returns early on ctx done.
func sleepJitter(ctx context.Context, d time.Duration) {
	jitter := time.Duration(rand.Int63n(int64(d) / 5))
	select {
	case <-ctx.Done():
	case <-time.After(d + jitter):
	}
}

// claimNextRun atomically grabs the oldest queued row and marks it claimed,
// setting the 60s lease. Returns (run, false, nil) when the queue is empty.
func claimNextRun(db *sql.DB) (TaskRun, bool, error) {
	stmt := `
		UPDATE ap_task_run
		SET status='claimed',
		    claimed_at=NOW(),
		    lease_expires_at=NOW() + interval '60 seconds'
		WHERE id = (
		    SELECT id FROM ap_task_run
		    WHERE status='queued'
		    ORDER BY queued_at
		    LIMIT 1
		    FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + taskRunColumns
	run, err := scanTaskRun(db.QueryRow(stmt))
	if err == sql.ErrNoRows {
		return TaskRun{}, false, nil
	}
	if err != nil {
		return TaskRun{}, false, err
	}
	return run, true, nil
}

// executeRun drives one task through its lifecycle. Errors are persisted
// into the run row (status=failed, error_message) rather than returned.
func (w *worker) executeRun(parent context.Context, run TaskRun) {
	runCtx, cancel := context.WithCancel(parent)
	defer cancel()
	registerCancel(run.ID, cancel)
	defer unregisterCancel(run.ID)

	spec, err := w.buildTaskSpec(run)
	if err != nil {
		w.finishRun(run.WorkspaceID, run.ID, "failed", nil, fmt.Sprintf("build task spec: %v", err), "")
		return
	}

	runner, err := agentrun.NewRunnerByKind(spec.RuntimeKind)
	if err != nil {
		w.finishRun(run.WorkspaceID, run.ID, "failed", nil, fmt.Sprintf("resolve runner: %v", err), "")
		return
	}

	// Mark running + stamp workdir path now, before Prepare, so the UI shows state.
	workdir := filepath.Join(w.workdirRoot, run.ID)
	spec.WorkdirPath = workdir
	if _, err := w.db.Exec(
		`UPDATE ap_task_run SET status='running', started_at=NOW(), workdir_path=$2 WHERE id=$1`,
		run.ID, workdir,
	); err != nil {
		w.logger.Error("mark running failed", "run", run.ID, "err", err.Error())
	}
	w.insertEvent(run.WorkspaceID, run.ID, agentrun.Event{
		Kind: agentrun.EventStatus, Payload: "running", At: time.Now(),
	})
	w.publishRunStatus(run.WorkspaceID, run.ID)

	wd, err := runner.Prepare(runCtx, spec)
	if err != nil {
		w.finishRun(run.WorkspaceID, run.ID, "failed", nil, fmt.Sprintf("prepare: %v", err), runnerVersion(runner))
		return
	}

	// Lease renewer — every 30s while we're running.
	leaseCtx, leaseCancel := context.WithCancel(runCtx)
	defer leaseCancel()
	go w.renewLease(leaseCtx, run.ID)

	// Event pump: drain runner events to DB until the channel closes.
	events := make(chan agentrun.Event, 64)
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for ev := range events {
			w.insertEvent(run.WorkspaceID, run.ID, ev)
		}
	}()

	runErr := runner.Run(runCtx, wd, spec, events)
	close(events)
	<-pumpDone

	// Collect artifacts best-effort before marking final status.
	if arts, err := runner.Collect(runCtx, wd); err == nil {
		for _, a := range arts {
			if _, err := w.db.Exec(
				`INSERT INTO ap_artifact (task_run_id, path, kind, size_bytes)
				 VALUES ($1, $2, $3, $4)`,
				run.ID, a.Path, a.Kind, a.SizeBytes,
			); err != nil {
				w.logger.Error("insert artifact failed", "run", run.ID, "err", err.Error())
			}
		}
	} else {
		w.logger.Warn("collect failed", "run", run.ID, "err", err.Error())
	}

	// Terminal status + cleanup.
	version := runnerVersion(runner)
	switch {
	case runErr == nil:
		w.finishRun(run.WorkspaceID, run.ID, "succeeded", intPtr(0), "", version)
		_ = os.RemoveAll(workdir) // success → drop workdir
	case runCtx.Err() != nil:
		// Either explicit cancel or parent shutdown. If the DB status is
		// already 'canceled' (the HTTP handler set it), the HTTP handler
		// already emitted the status event — still publish the row shape
		// so the UI can update without polling.
		if statusOf(w.db, run.ID) != "canceled" {
			w.finishRun(run.WorkspaceID, run.ID, "canceled", nil, "canceled", version)
		} else {
			w.publishRunStatus(run.WorkspaceID, run.ID)
		}
	default:
		w.finishRun(run.WorkspaceID, run.ID, "failed", nil, runErr.Error(), version)
	}
}

func (w *worker) buildTaskSpec(run TaskRun) (agentrun.TaskSpec, error) {
	const q = `
		SELECT i.issue_number, i.title, i.description,
		       a.name, a.runtime_kind, a.model, a.instructions
		FROM ap_task_run r
		JOIN ap_issue i ON i.id = r.issue_id
		JOIN ap_agent a ON a.id = r.agent_id
		WHERE r.id = $1
	`
	spec := agentrun.TaskSpec{
		RunID:         run.ID,
		IssueID:       run.IssueID,
		AgentID:       run.AgentID,
		NetworkPolicy: "none",
		Timeout:       15 * time.Minute,
	}
	if err := w.db.QueryRow(q, run.ID).Scan(
		&spec.IssueNumber, &spec.IssueTitle, &spec.IssueDesc,
		&spec.AgentName, &spec.RuntimeKind, &spec.Model, &spec.Instructions,
	); err != nil {
		return agentrun.TaskSpec{}, err
	}
	spec.EnvSecrets = envSecretsForKind(spec.RuntimeKind)
	// Agents that call external LLM APIs need network access.
	switch spec.RuntimeKind {
	case "claude_code", "codex", "openclaw", "opencode":
		spec.NetworkPolicy = "bridge"
	}
	return spec, nil
}

// envSecretsForKind pulls the appropriate provider API key from the
// server environment. Empty values are filtered so we don't pass
// "CLAUDE_API_KEY=" to the container.
func envSecretsForKind(kind string) map[string]string {
	keys := []string{}
	switch kind {
	case "claude_code", "openclaw":
		keys = []string{"ANTHROPIC_API_KEY", "CLAUDE_API_KEY"}
	case "codex", "opencode":
		keys = []string{"OPENAI_API_KEY"}
	}
	out := map[string]string{}
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			out[k] = v
		}
	}
	return out
}

func (w *worker) insertEvent(workspaceID, runID string, ev agentrun.Event) {
	var eventID int64
	if err := w.db.QueryRow(
		`INSERT INTO ap_task_event (task_run_id, kind, payload, at)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id`,
		runID, string(ev.Kind), ev.Payload, ev.At,
	).Scan(&eventID); err != nil {
		w.logger.Error("insert event failed", "run", runID, "err", err.Error())
		return
	}
	// Realtime fan-out — inert if no hub / no subscribers.
	publishTo(workspaceID, "task.event", TaskEvent{
		ID:        eventID,
		TaskRunID: runID,
		Kind:      string(ev.Kind),
		Payload:   ev.Payload,
		At:        ev.At,
	})
}

// publishRunStatus reads the current run row and emits a task.status event
// carrying the full TaskRun shape. Used by lifecycle transitions so the UI
// can reliably refresh a row without replaying every event.
func (w *worker) publishRunStatus(workspaceID, runID string) {
	run, err := getTaskRun(w.db, workspaceID, runID)
	if err != nil {
		return
	}
	publishTo(workspaceID, "task.status", run)
}

func (w *worker) renewLease(ctx context.Context, runID string) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := w.db.Exec(
				`UPDATE ap_task_run
				 SET lease_expires_at = NOW() + interval '60 seconds'
				 WHERE id = $1 AND status IN ('claimed','running')`,
				runID,
			); err != nil {
				w.logger.Error("renew lease failed", "run", runID, "err", err.Error())
			}
		}
	}
}

// finishRun writes the terminal status row update and an accompanying
// status event. Safe to call with status='canceled' — the SQL is a no-op
// if the HTTP cancel handler already set it.
func (w *worker) finishRun(workspaceID, runID, status string, exitCode *int, errMsg, runnerVersion string) {
	_, err := w.db.Exec(
		`UPDATE ap_task_run
		 SET status=$2,
		     finished_at=NOW(),
		     exit_code=$3,
		     error_message=NULLIF($4, ''),
		     runner_version=NULLIF($5, '')
		 WHERE id=$1 AND status NOT IN ('succeeded','failed','canceled')`,
		runID, status, nullableInt(exitCode), errMsg, runnerVersion,
	)
	if err != nil {
		w.logger.Error("finalize run failed", "run", runID, "err", err.Error())
	}
	w.insertEvent(workspaceID, runID, agentrun.Event{
		Kind: agentrun.EventStatus, Payload: status, At: time.Now(),
	})
	if errMsg != "" && status == "failed" {
		w.insertEvent(workspaceID, runID, agentrun.Event{
			Kind: agentrun.EventError, Payload: errMsg, At: time.Now(),
		})
	}
	// Publish the final row state so the UI flips its status row
	// immediately (not only upon replaying the events).
	w.publishRunStatus(workspaceID, runID)
}

// statusOf is a small helper to read the current status of a run.
func statusOf(db *sql.DB, runID string) string {
	var s string
	if err := db.QueryRow(`SELECT status FROM ap_task_run WHERE id=$1`, runID).Scan(&s); err != nil {
		return ""
	}
	return s
}

// nullableInt converts *int to driver value; returns nil so the DB stores NULL.
func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
func intPtr(v int) *int { return &v }

// runnerVersion prefers the runner's concrete Version() string (e.g. a
// Docker image tag like "chenweb/agentrun-claude:v1") when the runner
// implements it, falling back to Kind(). The value is persisted into
// ap_task_run.runner_version for audit.
func runnerVersion(r agentrun.Runner) string {
	if v, ok := r.(interface{ Version() string }); ok {
		if s := v.Version(); s != "" {
			return s
		}
	}
	return r.Kind()
}

// -----------------------------------------------------------------------------
// Sweeper — reclaims expired leases as 'failed' so crash-looped workers
// don't leave tasks stuck in 'claimed'/'running'.
// -----------------------------------------------------------------------------

func leaseSweeper(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			rows, err := db.Query(
				`UPDATE ap_task_run
				 SET status='failed',
				     finished_at=NOW(),
				     error_message='lease expired'
				 WHERE status IN ('claimed','running')
				   AND lease_expires_at IS NOT NULL
				   AND lease_expires_at < NOW()
				 RETURNING workspace_id, id`,
			)
			if err != nil {
				logger.Error("lease sweeper failed", "err", err.Error())
				continue
			}
			type reclaimed struct{ wid, rid string }
			var hits []reclaimed
			for rows.Next() {
				var r reclaimed
				if err := rows.Scan(&r.wid, &r.rid); err != nil {
					logger.Error("lease sweeper scan", "err", err.Error())
					continue
				}
				hits = append(hits, r)
			}
			_ = rows.Close()
			if len(hits) > 0 {
				logger.Warn("lease sweeper reclaimed stale runs", "count", len(hits))
				for _, r := range hits {
					if run, err := getTaskRun(db, r.wid, r.rid); err == nil {
						publishTo(r.wid, "task.status", run)
					}
				}
			}
		}
	}
}
