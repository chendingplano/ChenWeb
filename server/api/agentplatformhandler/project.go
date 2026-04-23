package agentplatformhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
)

// -----------------------------------------------------------------------------
// HTTP handlers
// -----------------------------------------------------------------------------

func ListProjects(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_PROJ_LIST")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), false)
	if !ok {
		return nil
	}
	items, err := listProjects(db, wid)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list projects: "+err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"projects": items})
}

func CreateProject(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_PROJ_CREATE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	var req CreateProjectRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return jsonError(c, http.StatusBadRequest, "name is required")
	}
	p, err := insertProject(db, wid, name, req.Description)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "create project: "+err.Error())
	}
	return c.JSON(http.StatusCreated, p)
}

func UpdateProject(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_PROJ_UPDATE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		return jsonError(c, http.StatusBadRequest, "project id required")
	}
	var req UpdateProjectRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	p, err := updateProject(db, wid, id, req)
	if err == sql.ErrNoRows {
		return jsonError(c, http.StatusNotFound, "project not found")
	}
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "update project: "+err.Error())
	}
	return c.JSON(http.StatusOK, p)
}

func DeleteProject(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_PROJ_DELETE")
	if !ok {
		return nil
	}
	db := projectDB()
	wid, _, ok := resolveWorkspace(c, db, userID, c.Param("slug"), true)
	if !ok {
		return nil
	}
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		return jsonError(c, http.StatusBadRequest, "project id required")
	}
	res, err := db.Exec(
		`DELETE FROM ap_project WHERE id = $1 AND workspace_id = $2`,
		id, wid,
	)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "delete project: "+err.Error())
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return jsonError(c, http.StatusNotFound, "project not found")
	}
	return c.NoContent(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// Store functions
// -----------------------------------------------------------------------------

func listProjects(db *sql.DB, workspaceID string) ([]Project, error) {
	const stmt = `
		SELECT id, workspace_id, name, description, created_at, updated_at
		FROM ap_project
		WHERE workspace_id = $1
		ORDER BY created_at DESC
	`
	rows, err := db.Query(stmt, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	out := make([]Project, 0)
	for rows.Next() {
		var p Project
		if err := rows.Scan(
			&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan project row: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func insertProject(db *sql.DB, workspaceID, name, description string) (Project, error) {
	const stmt = `
		INSERT INTO ap_project (workspace_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, workspace_id, name, description, created_at, updated_at
	`
	var p Project
	err := db.QueryRow(stmt, workspaceID, name, description).Scan(
		&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

func updateProject(db *sql.DB, workspaceID, projectID string, req UpdateProjectRequest) (Project, error) {
	const stmt = `
		UPDATE ap_project SET
		    name        = COALESCE($3, name),
		    description = COALESCE($4, description),
		    updated_at  = NOW()
		WHERE id = $1 AND workspace_id = $2
		RETURNING id, workspace_id, name, description, created_at, updated_at
	`
	var p Project
	err := db.QueryRow(stmt, projectID, workspaceID, req.Name, req.Description).Scan(
		&p.ID, &p.WorkspaceID, &p.Name, &p.Description, &p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}
