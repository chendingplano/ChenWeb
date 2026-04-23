package agentplatformhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// validStatuses mirrors the CHECK constraint on ap_issue.status. Keep in
// sync with migration 20260422000001.
var validStatuses = map[string]bool{
	"backlog":     true,
	"todo":        true,
	"in_progress": true,
	"in_review":   true,
	"done":        true,
	"canceled":    true,
}

// -----------------------------------------------------------------------------
// HTTP handlers
// -----------------------------------------------------------------------------

// ListIssues returns all issues in the workspace. Optional ?status=<csv> and
// ?project_id=<id> query params narrow the result set.
func ListIssues(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_ISSUE_LIST")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	statuses := parseStatusesCSV(c.QueryParam("status"))
	projectFilter := strings.TrimSpace(c.QueryParam("project_id"))
	items, err := listIssues(db, wid, statuses, projectFilter)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list issues: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"issues": items})
}

// GetIssue returns one issue by its per-workspace issue_number.
func GetIssue(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_ISSUE_GET")
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
	issue, err := getIssueByNumber(db, wid, num)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "issue not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "get issue: "+err.Error())
	}
	return c.JSON(http.StatusOK, issue)
}

// CreateIssue allocates a fresh issue_number and inserts the issue.
// Requires member+ role.
func CreateIssue(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_ISSUE_CREATE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	var req CreateIssueRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return jsonError(c, http.StatusBadRequest, "title is required")
	}
	status := req.Status
	if status == "" {
		status = "backlog"
	}
	if !validStatuses[status] {
		return jsonError(c, http.StatusBadRequest, "invalid status")
	}
	if req.Priority < 0 || req.Priority > 4 {
		return jsonError(c, http.StatusBadRequest, "priority must be 0-4")
	}
	if req.AssigneeUserID != "" && req.AssigneeAgentID != "" {
		return jsonError(c, http.StatusBadRequest, "issue cannot have both a user and an agent assignee")
	}
	if req.AssigneeAgentID != "" {
		if ok, err := agentBelongsToWorkspace(db, wid, req.AssigneeAgentID); err != nil {
			return jsonError(c, http.StatusInternalServerError, "verify agent: "+err.Error())
		} else if !ok {
			return jsonError(c, http.StatusBadRequest, "agent not in this workspace")
		}
	}
	issue, err := insertIssueTx(db, wid, userID, req, status)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "create issue: "+err.Error())
	}
	publishTo(wid, "issue.updated", issue)
	return c.JSON(http.StatusCreated, issue)
}

// UpdateIssue patches an issue. Requires member+ role.
func UpdateIssue(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_ISSUE_UPDATE")
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
	var req UpdateIssueRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	if req.Status != nil && !validStatuses[*req.Status] {
		return jsonError(c, http.StatusBadRequest, "invalid status")
	}
	if req.Priority != nil && (*req.Priority < 0 || *req.Priority > 4) {
		return jsonError(c, http.StatusBadRequest, "priority must be 0-4")
	}
	if req.AssigneeUserID != nil && req.AssigneeAgentID != nil &&
		*req.AssigneeUserID != "" && *req.AssigneeAgentID != "" {
		return jsonError(c, http.StatusBadRequest, "issue cannot have both a user and an agent assignee")
	}
	if req.AssigneeAgentID != nil && *req.AssigneeAgentID != "" {
		if ok, err := agentBelongsToWorkspace(db, wid, *req.AssigneeAgentID); err != nil {
			return jsonError(c, http.StatusInternalServerError, "verify agent: "+err.Error())
		} else if !ok {
			return jsonError(c, http.StatusBadRequest, "agent not in this workspace")
		}
	}

	issue, err := updateIssueByNumber(db, wid, num, req)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "issue not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "update issue: "+err.Error())
	}
	publishTo(wid, "issue.updated", issue)
	return c.JSON(http.StatusOK, issue)
}

// BulkMoveIssues applies a batch of (status, board_order) changes.
// Transactional: any failure rolls back the whole batch.
func BulkMoveIssues(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_ISSUE_BULKMOVE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	var req BulkMoveRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	if len(req.Moves) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"updated": 0})
	}
	for _, m := range req.Moves {
		if !validStatuses[m.Status] {
			return jsonError(c, http.StatusBadRequest, "invalid status in moves")
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "begin tx: "+err.Error())
	}
	defer func() { _ = tx.Rollback() }()

	const stmt = `
		UPDATE ap_issue SET
		    status      = $3,
		    board_order = $4,
		    updated_at  = NOW()
		WHERE workspace_id = $1 AND issue_number = $2
	`
	var updated int64
	for _, m := range req.Moves {
		res, err := tx.Exec(stmt, wid, m.IssueNumber, m.Status, m.BoardOrder)
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, "bulk move: "+err.Error())
		}
		n, _ := res.RowsAffected()
		updated += n
	}
	if err := tx.Commit(); err != nil {
		return jsonError(c, http.StatusInternalServerError, "commit: "+err.Error())
	}
	// Post-commit fan-out: one issue.updated per moved row. Uses the
	// outer db (not tx) so readers see committed state.
	for _, m := range req.Moves {
		if iss, err := getIssueByNumber(db, wid, m.IssueNumber); err == nil {
			publishTo(wid, "issue.updated", iss)
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"updated": updated})
}

// ListComments returns comments on an issue, oldest first.
func ListComments(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_CMT_LIST")
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
	items, err := listComments(db, issueID)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list comments: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"comments": items})
}

// CreateComment posts a new comment authored by the current user.
func CreateComment(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_CMT_CREATE")
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
	var req CreateCommentRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return jsonError(c, http.StatusBadRequest, "body is required")
	}
	issueID, err := lookupIssueID(db, wid, num)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "issue not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "lookup issue: "+err.Error())
	}
	cm, err := insertComment(db, issueID, userID, body)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "create comment: "+err.Error())
	}
	publishTo(wid, "comment.created", cm)
	return c.JSON(http.StatusCreated, cm)
}

// -----------------------------------------------------------------------------
// Store functions
// -----------------------------------------------------------------------------

// issueColumns is the SELECT list shared by every query returning Issue.
const issueColumns = `
	id, workspace_id, project_id, issue_number, title, description,
	status, priority, board_order,
	assignee_user_id, assignee_agent_id,
	created_by, created_at, updated_at
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIssue(scanner rowScanner) (Issue, error) {
	var iss Issue
	var projectID, assigneeUser, assigneeAgent sql.NullString
	err := scanner.Scan(
		&iss.ID, &iss.WorkspaceID, &projectID, &iss.IssueNumber, &iss.Title, &iss.Description,
		&iss.Status, &iss.Priority, &iss.BoardOrder,
		&assigneeUser, &assigneeAgent,
		&iss.CreatedBy, &iss.CreatedAt, &iss.UpdatedAt,
	)
	if err != nil {
		return Issue{}, err
	}
	if projectID.Valid {
		s := projectID.String
		iss.ProjectID = &s
	}
	if assigneeUser.Valid {
		s := assigneeUser.String
		iss.AssigneeUserID = &s
	}
	if assigneeAgent.Valid {
		s := assigneeAgent.String
		iss.AssigneeAgentID = &s
	}
	return iss, nil
}

func listIssues(db *sql.DB, workspaceID string, statuses []string, projectFilter string) ([]Issue, error) {
	args := []any{workspaceID}
	stmt := `SELECT ` + issueColumns + ` FROM ap_issue WHERE workspace_id = $1`
	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for i, s := range statuses {
			args = append(args, s)
			placeholders[i] = "$" + strconv.Itoa(len(args))
		}
		stmt += " AND status IN (" + strings.Join(placeholders, ",") + ")"
	}
	if projectFilter != "" {
		args = append(args, projectFilter)
		stmt += " AND project_id = $" + strconv.Itoa(len(args))
	}
	stmt += " ORDER BY status, board_order ASC, issue_number ASC"

	rows, err := db.Query(stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	out := make([]Issue, 0)
	for rows.Next() {
		iss, err := scanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan issue row: %w", err)
		}
		out = append(out, iss)
	}
	return out, rows.Err()
}

func getIssueByNumber(db *sql.DB, workspaceID string, number int) (Issue, error) {
	stmt := `SELECT ` + issueColumns + ` FROM ap_issue WHERE workspace_id = $1 AND issue_number = $2`
	return scanIssue(db.QueryRow(stmt, workspaceID, number))
}

func lookupIssueID(db *sql.DB, workspaceID string, number int) (string, error) {
	var id string
	err := db.QueryRow(
		`SELECT id FROM ap_issue WHERE workspace_id = $1 AND issue_number = $2`,
		workspaceID, number,
	).Scan(&id)
	return id, err
}

// insertIssueTx allocates a fresh issue_number from ap_workspace_counter and
// inserts the issue in the same transaction. The UPDATE ... RETURNING on
// the counter row takes a row-level lock, so concurrent creators serialize.
func insertIssueTx(db *sql.DB, workspaceID, creatorUserID string, req CreateIssueRequest, status string) (Issue, error) {
	tx, err := db.Begin()
	if err != nil {
		return Issue{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var next int
	if err := tx.QueryRow(
		`UPDATE ap_workspace_counter
		 SET next_issue_number = next_issue_number + 1
		 WHERE workspace_id = $1
		 RETURNING next_issue_number - 1`,
		workspaceID,
	).Scan(&next); err != nil {
		return Issue{}, fmt.Errorf("allocate issue number: %w", err)
	}

	stmt := `
		INSERT INTO ap_issue (
		    workspace_id, project_id, issue_number, title, description,
		    status, priority, assignee_user_id, assignee_agent_id, created_by
		) VALUES (
		    $1, NULLIF($2, ''), $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10
		) RETURNING ` + issueColumns

	row := tx.QueryRow(stmt,
		workspaceID, req.ProjectID, next, strings.TrimSpace(req.Title), req.Description,
		status, req.Priority, req.AssigneeUserID, req.AssigneeAgentID, creatorUserID,
	)
	iss, err := scanIssue(row)
	if err != nil {
		return Issue{}, fmt.Errorf("insert issue: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Issue{}, fmt.Errorf("commit: %w", err)
	}
	return iss, nil
}

// updateIssueByNumber applies the partial UpdateIssueRequest. Nullable fields
// use CASE-WHEN to distinguish "set to NULL/clear" from "leave alone":
//   - passing a non-nil empty string clears the field
//   - passing a non-nil non-empty value sets it
//   - passing nil leaves the existing value
func updateIssueByNumber(db *sql.DB, workspaceID string, number int, req UpdateIssueRequest) (Issue, error) {
	stmt := `
		UPDATE ap_issue SET
		    project_id        = CASE WHEN $3::text IS NULL THEN project_id ELSE NULLIF($3, '') END,
		    title             = COALESCE($4, title),
		    description       = COALESCE($5, description),
		    status            = COALESCE($6, status),
		    priority          = COALESCE($7, priority),
		    board_order       = COALESCE($8, board_order),
		    assignee_user_id  = CASE WHEN $9::text  IS NULL THEN assignee_user_id  ELSE NULLIF($9, '')  END,
		    assignee_agent_id = CASE WHEN $10::text IS NULL THEN assignee_agent_id ELSE NULLIF($10, '') END,
		    updated_at        = NOW()
		WHERE workspace_id = $1 AND issue_number = $2
		RETURNING ` + issueColumns

	row := db.QueryRow(stmt,
		workspaceID, number,
		req.ProjectID, req.Title, req.Description, req.Status, req.Priority, req.BoardOrder,
		req.AssigneeUserID, req.AssigneeAgentID,
	)
	return scanIssue(row)
}

// agentBelongsToWorkspace guards against cross-tenant agent assignment.
func agentBelongsToWorkspace(db *sql.DB, workspaceID, agentID string) (bool, error) {
	var exists bool
	err := db.QueryRow(
		`SELECT EXISTS(
		     SELECT 1 FROM ap_agent
		     WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
		 )`,
		agentID, workspaceID,
	).Scan(&exists)
	return exists, err
}

// -----------------------------------------------------------------------------
// Comments
// -----------------------------------------------------------------------------

func listComments(db *sql.DB, issueID string) ([]Comment, error) {
	const stmt = `
		SELECT id, issue_id, author_user_id, author_agent_id, body, created_at
		FROM ap_comment
		WHERE issue_id = $1
		ORDER BY created_at ASC, id ASC
	`
	rows, err := db.Query(stmt, issueID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	out := make([]Comment, 0)
	for rows.Next() {
		var cm Comment
		var user, agent sql.NullString
		if err := rows.Scan(&cm.ID, &cm.IssueID, &user, &agent, &cm.Body, &cm.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment row: %w", err)
		}
		if user.Valid {
			s := user.String
			cm.AuthorUserID = &s
		}
		if agent.Valid {
			s := agent.String
			cm.AuthorAgentID = &s
		}
		out = append(out, cm)
	}
	return out, rows.Err()
}

func insertComment(db *sql.DB, issueID, userID, body string) (Comment, error) {
	const stmt = `
		INSERT INTO ap_comment (issue_id, author_user_id, body)
		VALUES ($1, $2, $3)
		RETURNING id, issue_id, author_user_id, author_agent_id, body, created_at
	`
	var cm Comment
	var user, agent sql.NullString
	err := db.QueryRow(stmt, issueID, userID, body).Scan(
		&cm.ID, &cm.IssueID, &user, &agent, &cm.Body, &cm.CreatedAt,
	)
	if err != nil {
		return Comment{}, err
	}
	if user.Valid {
		s := user.String
		cm.AuthorUserID = &s
	}
	if agent.Valid {
		s := agent.String
		cm.AuthorAgentID = &s
	}
	return cm, nil
}

// -----------------------------------------------------------------------------
// Small helpers
// -----------------------------------------------------------------------------

// parseIssueNum reads the :num URL param and writes a 400 on failure.
func parseIssueNum(c echo.Context) (int, bool) {
	raw := c.Param("num")
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		_ = jsonError(c, http.StatusBadRequest, "issue number must be a positive integer")
		return 0, false
	}
	return n, true
}

// parseStatusesCSV splits a "?status=todo,in_progress" param into known values.
func parseStatusesCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if validStatuses[p] {
			out = append(out, p)
		}
	}
	return out
}
