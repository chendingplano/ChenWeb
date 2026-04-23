package agentplatformhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// -----------------------------------------------------------------------------
// HTTP handlers — task runs
// -----------------------------------------------------------------------------

// RunIssue enqueues a new ap_task_run for the named issue. The issue must
// have an agent assignee (the worker needs something to dispatch to).
// Returns the created TaskRun (status=queued). Requires member+ role.
func RunIssue(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_RUN_CREATE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	num, ok := parseIssueNum(c)
	if !ok {
		return nil
	}
	iss, err := getIssueByNumber(db, wid, num)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "issue not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "get issue: "+err.Error())
	}
	if iss.AssigneeAgentID == nil {
		return jsonError(c, http.StatusBadRequest, "assign an agent before running")
	}
	run, err := insertTaskRun(db, wid, iss.ID, *iss.AssigneeAgentID)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "enqueue run: "+err.Error())
	}
	run.IssueNumber = iss.IssueNumber
	publishTo(wid, "run.created", run)
	return c.JSON(http.StatusCreated, run)
}

// ListIssueRuns returns every task run for an issue, newest first.
func ListIssueRuns(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_RUN_LIST")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	num, ok := parseIssueNum(c)
	if !ok {
		return nil
	}
	issueID, err := lookupIssueID(db, wid, num)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "issue not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "lookup issue: "+err.Error())
	}
	runs, err := listTaskRunsForIssue(db, wid, issueID)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list runs: "+err.Error())
	}
	// Backfill IssueNumber so the UI can link cleanly without a separate JOIN.
	for i := range runs {
		runs[i].IssueNumber = num
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": runs})
}

// GetTaskRun returns a single run by id.
func GetTaskRun(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_RUN_GET")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "run id required")
	}
	run, err := getTaskRun(db, wid, id)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "run not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "get run: "+err.Error())
	}
	return c.JSON(http.StatusOK, run)
}

// ListRunEvents returns events for a run. ?since=<event_id> paginates
// forward — clients poll by passing the last id they saw.
func ListRunEvents(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_EVT_LIST")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "run id required")
	}
	// Tenant boundary: make sure the run belongs to the addressed workspace.
	if _, err := getTaskRun(db, wid, id); err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "run not found")
	} else if err != nil {
		return jsonError(c, http.StatusInternalServerError, "check run: "+err.Error())
	}
	var since int64
	if raw := strings.TrimSpace(c.QueryParam("since")); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			return jsonError(c, http.StatusBadRequest, "since must be a non-negative integer")
		}
		since = n
	}
	events, err := listTaskEvents(db, id, since, 500)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list events: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"events": events})
}

// CancelTaskRun marks a run as canceled and signals any in-flight worker.
func CancelTaskRun(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_RUN_CANCEL")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "run id required")
	}
	run, err := getTaskRun(db, wid, id)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "run not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "get run: "+err.Error())
	}
	switch run.Status {
	case "succeeded", "failed", "canceled":
		return jsonError(c, http.StatusConflict, "run already finished")
	}
	// Mark canceled. If still queued, this ends the story. If running,
	// the worker also gets signaled via the in-process cancel registry.
	if _, err := db.Exec(
		`UPDATE ap_task_run SET status='canceled', finished_at=NOW()
		 WHERE id=$1 AND workspace_id=$2 AND status IN ('queued','claimed','running')`,
		id, wid,
	); err != nil {
		return jsonError(c, http.StatusInternalServerError, "cancel run: "+err.Error())
	}
	signalCancel(id)
	// Publish the post-cancel row shape so the UI updates without polling
	// (and even for queued runs where no worker is in-flight).
	if fresh, err := getTaskRun(db, wid, id); err == nil {
		publishTo(wid, "task.status", fresh)
	}
	return c.NoContent(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// Store functions
// -----------------------------------------------------------------------------

const taskRunColumns = `
	id, workspace_id, issue_id, agent_id, status,
	queued_at, claimed_at, started_at, finished_at,
	exit_code, error_message, runner_version, workdir_path, lease_expires_at
`

func scanTaskRun(s rowScanner) (TaskRun, error) {
	var r TaskRun
	var claimedAt, startedAt, finishedAt, lease sql.NullTime
	var exitCode sql.NullInt32
	var errMsg, runnerVer, workdir sql.NullString
	err := s.Scan(
		&r.ID, &r.WorkspaceID, &r.IssueID, &r.AgentID, &r.Status,
		&r.QueuedAt, &claimedAt, &startedAt, &finishedAt,
		&exitCode, &errMsg, &runnerVer, &workdir, &lease,
	)
	if err != nil {
		return TaskRun{}, err
	}
	if claimedAt.Valid {
		t := claimedAt.Time
		r.ClaimedAt = &t
	}
	if startedAt.Valid {
		t := startedAt.Time
		r.StartedAt = &t
	}
	if finishedAt.Valid {
		t := finishedAt.Time
		r.FinishedAt = &t
	}
	if lease.Valid {
		t := lease.Time
		r.LeaseExpiresAt = &t
	}
	if exitCode.Valid {
		v := int(exitCode.Int32)
		r.ExitCode = &v
	}
	if errMsg.Valid {
		s := errMsg.String
		r.ErrorMessage = &s
	}
	if runnerVer.Valid {
		s := runnerVer.String
		r.RunnerVersion = &s
	}
	if workdir.Valid {
		s := workdir.String
		r.WorkdirPath = &s
	}
	return r, nil
}

func insertTaskRun(db *sql.DB, workspaceID, issueID, agentID string) (TaskRun, error) {
	stmt := `
		INSERT INTO ap_task_run (workspace_id, issue_id, agent_id, status)
		VALUES ($1, $2, $3, 'queued')
		RETURNING ` + taskRunColumns
	return scanTaskRun(db.QueryRow(stmt, workspaceID, issueID, agentID))
}

func getTaskRun(db *sql.DB, workspaceID, id string) (TaskRun, error) {
	stmt := `SELECT ` + taskRunColumns + ` FROM ap_task_run
	         WHERE id=$1 AND workspace_id=$2`
	return scanTaskRun(db.QueryRow(stmt, id, workspaceID))
}

func listTaskRunsForIssue(db *sql.DB, workspaceID, issueID string) ([]TaskRun, error) {
	stmt := `SELECT ` + taskRunColumns + ` FROM ap_task_run
	         WHERE workspace_id=$1 AND issue_id=$2
	         ORDER BY queued_at DESC`
	rows, err := db.Query(stmt, workspaceID, issueID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	out := make([]TaskRun, 0)
	for rows.Next() {
		r, err := scanTaskRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func listTaskEvents(db *sql.DB, runID string, sinceID int64, limit int) ([]TaskEvent, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	stmt := `
		SELECT id, task_run_id, kind, payload, at
		FROM ap_task_event
		WHERE task_run_id = $1 AND id > $2
		ORDER BY id ASC
		LIMIT $3
	`
	rows, err := db.Query(stmt, runID, sinceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()
	out := make([]TaskEvent, 0)
	for rows.Next() {
		var e TaskEvent
		if err := rows.Scan(&e.ID, &e.TaskRunID, &e.Kind, &e.Payload, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
