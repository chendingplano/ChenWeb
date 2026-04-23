// Package agentplatformhandler implements the home3 Agent Platform REST API:
// workspaces, agents, projects, issues (kanban), and comments.
//
// Routes are mounted under /api/v1/ap/ by ChenWeb/server/api/routes.go.
// All endpoints require an authenticated session (enforced by the /api/v1
// auth middleware). Every workspace-scoped endpoint additionally verifies
// the caller's membership and role via resolveWorkspace.
//
// Tables owned by this package live in
// project_migrations/20260422000001_create_agent_platform_tables.sql
// (all prefixed ap_*). No code or schema is derived from multica-ai/multica.
package agentplatformhandler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ErrNotAuthorized is an internal sentinel. Handlers translate authorization
// failures into 404 (on reads, to avoid leaking workspace existence to
// non-members) or 403 (on writes where membership is known).
var ErrNotAuthorized = errors.New("agentplatformhandler: not authorized")

// roleRank orders roles from least to most privileged. Compare with >= to
// enforce "at least" semantics (e.g. write requires >= member; viewer denied).
var roleRank = map[string]int{
	"viewer": 1,
	"member": 2,
	"owner":  3,
}

// requireUser resolves the authenticated user from the Echo context. On
// failure it writes a 403 JSON body and returns ok == false; callers should
// simply return nil.
func requireUser(c echo.Context, loc string) (userID string, ok bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	defer rc.Close()
	info := rc.IsAuthenticated()
	if info == nil {
		_ = c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "login required (" + loc + ")"})
		return "", false
	}
	return info.UserId, true
}

// jsonError writes an ErrorResponse with the given HTTP status.
func jsonError(c echo.Context, status int, msg string) error {
	return c.JSON(status, ErrorResponse{Status: false, ErrorMsg: msg})
}

// resolveWorkspace looks up a workspace by slug, verifies the caller is a
// member, and (if requireWrite) that their role is owner or member.
// Returns (workspaceID, role) on success.
//
// On a missing workspace OR a non-membership we both return 404 — we do not
// distinguish the two so outsiders cannot probe slug existence.
func resolveWorkspace(c echo.Context, db *sql.DB, userID, slug string, requireWrite bool) (workspaceID, role string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		_ = jsonError(c, http.StatusBadRequest, "workspace slug required")
		return "", "", false
	}
	const stmt = `
		SELECT w.id, m.role
		FROM ap_workspace w
		JOIN ap_workspace_member m ON m.workspace_id = w.id
		WHERE LOWER(w.slug) = $1 AND m.user_id = $2
	`
	var wid, r string
	err := db.QueryRow(stmt, slug, userID).Scan(&wid, &r)
	if err == sql.ErrNoRows {
		_ = jsonError(c, http.StatusNotFound, "workspace not found")
		return "", "", false
	}
	if err != nil {
		_ = jsonError(c, http.StatusInternalServerError, "workspace lookup failed")
		return "", "", false
	}
	if requireWrite && roleRank[r] < roleRank["member"] {
		_ = jsonError(c, http.StatusForbidden, "write access required")
		return "", "", false
	}
	return wid, r, true
}

// projectDB returns the shared Postgres pool. Extracted so tests can swap it.
func projectDB() *sql.DB { return ApiTypes.ProjectDBHandle }
