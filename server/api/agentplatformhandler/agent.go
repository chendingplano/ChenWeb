package agentplatformhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// validRuntimeKinds mirrors the CHECK constraint on ap_agent.runtime_kind.
// Keep in sync with migration 20260422000001.
var validRuntimeKinds = map[string]bool{
	"claude_code": true,
	"codex":       true,
	"openclaw":    true,
	"opencode":    true,
}

// -----------------------------------------------------------------------------
// HTTP handlers
// -----------------------------------------------------------------------------

// ListAgents returns all non-archived agents in the workspace, newest first.
func ListAgents(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_AGENT_LIST")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	items, err := listAgents(db, wid)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list agents: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"agents": items})
}

// CreateAgent creates a new agent in the workspace. Requires member+ role.
func CreateAgent(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_AGENT_CREATE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	var req CreateAgentRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return jsonError(c, http.StatusBadRequest, "name is required")
	}
	if !validRuntimeKinds[req.RuntimeKind] {
		return jsonError(c, http.StatusBadRequest, "invalid runtime_kind")
	}
	avatar := req.AvatarEmoji
	if strings.TrimSpace(avatar) == "" {
		avatar = "🤖"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	a, err := insertAgent(db, wid, name, avatar, req.RuntimeKind, req.Model, req.Instructions, enabled)
	if err != nil {
		if isUniqueViolation(err) {
			return jsonError(c, http.StatusConflict, "agent name already used in this workspace")
		}
		return jsonError(c, http.StatusInternalServerError, "create agent: "+err.Error())
	}
	publishTo(wid, "agent.updated", a)
	return c.JSON(http.StatusCreated, a)
}

// UpdateAgent patches an agent. Requires member+ role.
func UpdateAgent(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_AGENT_UPDATE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	agentID := c.Param("id")
	if strings.TrimSpace(agentID) == "" {
		return jsonError(c, http.StatusBadRequest, "agent id required")
	}
	var req UpdateAgentRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	a, err := updateAgent(db, wid, agentID, req)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "agent not found")
	}
	if err != nil {
		if isUniqueViolation(err) {
			return jsonError(c, http.StatusConflict, "agent name already used in this workspace")
		}
		return jsonError(c, http.StatusInternalServerError, "update agent: "+err.Error())
	}
	publishTo(wid, "agent.updated", a)
	return c.JSON(http.StatusOK, a)
}

// DeleteAgent soft-deletes an agent by setting archived_at = NOW().
// Historical task_run rows keep referencing it via ON DELETE RESTRICT.
func DeleteAgent(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_AGENT_DELETE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	agentID := c.Param("id")
	if strings.TrimSpace(agentID) == "" {
		return jsonError(c, http.StatusBadRequest, "agent id required")
	}
	res, err := db.Exec(
		`UPDATE ap_agent SET archived_at = NOW(), updated_at = NOW()
		 WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL`,
		agentID, wid,
	)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "delete agent: "+err.Error())
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return jsonError(c, http.StatusNotFound, "agent not found")
	}
	// Publish the archived row so subscribers can drop it from their list.
	if fresh, err := getAgentByID(db, wid, agentID); err == nil {
		publishTo(wid, "agent.updated", fresh)
	}
	return c.NoContent(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// Store functions
// -----------------------------------------------------------------------------

func listAgents(db *sql.DB, workspaceID string) ([]Agent, error) {
	const stmt = `
		SELECT id, workspace_id, name, avatar_emoji, runtime_kind,
		       model, instructions, enabled, archived_at, created_at, updated_at
		FROM ap_agent
		WHERE workspace_id = $1 AND archived_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := db.Query(stmt, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	out := make([]Agent, 0)
	for rows.Next() {
		var a Agent
		var archived sql.NullTime
		if err := rows.Scan(
			&a.ID, &a.WorkspaceID, &a.Name, &a.AvatarEmoji, &a.RuntimeKind,
			&a.Model, &a.Instructions, &a.Enabled, &archived, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan agent row: %w", err)
		}
		if archived.Valid {
			t := archived.Time
			a.ArchivedAt = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// getAgentByID returns a single agent by id — used to re-read the
// archived row after DeleteAgent for realtime fan-out. Does not filter
// on archived_at (callers want to see the archived flag flip).
func getAgentByID(db *sql.DB, workspaceID, agentID string) (Agent, error) {
	const stmt = `
		SELECT id, workspace_id, name, avatar_emoji, runtime_kind,
		       model, instructions, enabled, archived_at, created_at, updated_at
		FROM ap_agent
		WHERE id = $1 AND workspace_id = $2
	`
	var a Agent
	var archived sql.NullTime
	if err := db.QueryRow(stmt, agentID, workspaceID).Scan(
		&a.ID, &a.WorkspaceID, &a.Name, &a.AvatarEmoji, &a.RuntimeKind,
		&a.Model, &a.Instructions, &a.Enabled, &archived, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return Agent{}, err
	}
	if archived.Valid {
		t := archived.Time
		a.ArchivedAt = &t
	}
	return a, nil
}

func insertAgent(db *sql.DB, workspaceID, name, avatar, runtimeKind, model, instructions string, enabled bool) (Agent, error) {
	const stmt = `
		INSERT INTO ap_agent (workspace_id, name, avatar_emoji, runtime_kind, model, instructions, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, workspace_id, name, avatar_emoji, runtime_kind,
		          model, instructions, enabled, archived_at, created_at, updated_at
	`
	var a Agent
	var archived sql.NullTime
	err := db.QueryRow(stmt, workspaceID, name, avatar, runtimeKind, model, instructions, enabled).Scan(
		&a.ID, &a.WorkspaceID, &a.Name, &a.AvatarEmoji, &a.RuntimeKind,
		&a.Model, &a.Instructions, &a.Enabled, &archived, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return Agent{}, err
	}
	if archived.Valid {
		t := archived.Time
		a.ArchivedAt = &t
	}
	return a, nil
}

// updateAgent applies the partial update to a live (non-archived) agent.
// Uses COALESCE-on-null to leave untouched fields alone.
func updateAgent(db *sql.DB, workspaceID, agentID string, req UpdateAgentRequest) (Agent, error) {
	const stmt = `
		UPDATE ap_agent SET
		    name         = COALESCE($3, name),
		    avatar_emoji = COALESCE($4, avatar_emoji),
		    model        = COALESCE($5, model),
		    instructions = COALESCE($6, instructions),
		    enabled      = COALESCE($7, enabled),
		    updated_at   = NOW()
		WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
		RETURNING id, workspace_id, name, avatar_emoji, runtime_kind,
		          model, instructions, enabled, archived_at, created_at, updated_at
	`
	var a Agent
	var archived sql.NullTime
	err := db.QueryRow(stmt,
		agentID, workspaceID,
		req.Name, req.AvatarEmoji, req.Model, req.Instructions, req.Enabled,
	).Scan(
		&a.ID, &a.WorkspaceID, &a.Name, &a.AvatarEmoji, &a.RuntimeKind,
		&a.Model, &a.Instructions, &a.Enabled, &archived, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return Agent{}, err
	}
	if archived.Valid {
		t := archived.Time
		a.ArchivedAt = &t
	}
	return a, nil
}
