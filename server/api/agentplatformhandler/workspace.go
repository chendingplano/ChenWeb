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

// ListWorkspaces returns every workspace the caller is a member of.
//
// Auto-bootstrap: if the caller has zero memberships, we create a "Personal"
// workspace for them and return it. This is the onboarding path agreed in
// the design doc (§9.3).
func ListWorkspaces(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_WS_LIST")
	if !ok {
		return nil
	}
	db := projectDB()

	items, err := listWorkspacesForUser(db, userID)
	if err != nil {
		return jsonError(c, http.StatusInternalServerError, "list workspaces: "+err.Error())
	}
	if len(items) == 0 {
		// First login — auto-create a Personal workspace.
		if _, err := EnsurePersonalWorkspace(db, userID); err != nil {
			return jsonError(c, http.StatusInternalServerError, "bootstrap personal workspace: "+err.Error())
		}
		items, err = listWorkspacesForUser(db, userID)
		if err != nil {
			return jsonError(c, http.StatusInternalServerError, "re-list workspaces: "+err.Error())
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"workspaces": items})
}

// CreateWorkspace creates a new workspace and makes the caller its owner.
func CreateWorkspace(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_AP_WS_CREATE")
	if !ok {
		return nil
	}
	var req CreateWorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid body")
	}
	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	name := strings.TrimSpace(req.Name)
	if slug == "" || name == "" {
		return jsonError(c, http.StatusBadRequest, "slug and name are required")
	}
	if !validSlug(slug) {
		return jsonError(c, http.StatusBadRequest, "slug must be lowercase letters, digits, or '-' (2-40 chars)")
	}
	db := projectDB()
	ws, err := insertWorkspaceTx(db, userID, slug, name, "owner")
	if err != nil {
		if isUniqueViolation(err) {
			return jsonError(c, http.StatusConflict, "slug already taken")
		}
		return jsonError(c, http.StatusInternalServerError, "create workspace: "+err.Error())
	}
	return c.JSON(http.StatusCreated, ws)
}

// -----------------------------------------------------------------------------
// Bootstrap helper
// -----------------------------------------------------------------------------

// EnsurePersonalWorkspace is idempotent: if the user already has a "personal-*"
// workspace it returns it, otherwise it creates one. Safe under concurrent
// first-login races — relies on the slug unique index and membership PK.
func EnsurePersonalWorkspace(db *sql.DB, userID string) (Workspace, error) {
	slug := personalSlugFor(userID)

	// Best-effort insert; ON CONFLICT lets us no-op if a racing request got
	// there first. The RETURNING clause only fires on real insert.
	const ins = `
		INSERT INTO ap_workspace (slug, name, owner_user_id)
		VALUES ($1, 'Personal', $2)
		ON CONFLICT DO NOTHING
		RETURNING id
	`
	var wid string
	err := db.QueryRow(ins, slug, userID).Scan(&wid)
	if err != nil && err != sql.ErrNoRows {
		return Workspace{}, fmt.Errorf("ensure personal workspace insert: %w", err)
	}
	if wid == "" {
		// Row already existed — fetch it.
		if err := db.QueryRow(
			`SELECT id FROM ap_workspace WHERE LOWER(slug) = $1`, slug,
		).Scan(&wid); err != nil {
			return Workspace{}, fmt.Errorf("ensure personal workspace fetch: %w", err)
		}
	}

	// Guarantee membership + counter rows exist (idempotent).
	if _, err := db.Exec(
		`INSERT INTO ap_workspace_member (workspace_id, user_id, role)
		 VALUES ($1, $2, 'owner') ON CONFLICT DO NOTHING`,
		wid, userID,
	); err != nil {
		return Workspace{}, fmt.Errorf("ensure membership: %w", err)
	}
	if _, err := db.Exec(
		`INSERT INTO ap_workspace_counter (workspace_id, next_issue_number)
		 VALUES ($1, 1) ON CONFLICT DO NOTHING`,
		wid,
	); err != nil {
		return Workspace{}, fmt.Errorf("ensure counter: %w", err)
	}

	return getWorkspaceForUser(db, userID, wid)
}

// personalSlugFor returns a deterministic slug for a user's personal
// workspace. Kratos user ids are UUIDs, so the first 12 hex chars give us
// 48 bits of uniqueness — collision-resistant for realistic user counts.
func personalSlugFor(userID string) string {
	trim := strings.ReplaceAll(userID, "-", "")
	if len(trim) > 12 {
		trim = trim[:12]
	}
	return "personal-" + strings.ToLower(trim)
}

// -----------------------------------------------------------------------------
// Store functions
// -----------------------------------------------------------------------------

// listWorkspacesForUser returns every workspace the user belongs to, newest
// membership first. my_role is the caller's role in that workspace.
func listWorkspacesForUser(db *sql.DB, userID string) ([]Workspace, error) {
	const stmt = `
		SELECT w.id, w.slug, w.name, w.owner_user_id, m.role, w.created_at, w.updated_at
		FROM ap_workspace w
		JOIN ap_workspace_member m ON m.workspace_id = w.id
		WHERE m.user_id = $1
		ORDER BY m.created_at DESC
	`
	rows, err := db.Query(stmt, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	out := make([]Workspace, 0)
	for rows.Next() {
		var w Workspace
		if err := rows.Scan(
			&w.ID, &w.Slug, &w.Name, &w.OwnerUserID, &w.MyRole,
			&w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan workspace row: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// getWorkspaceForUser fetches one workspace by id, ensuring the caller is
// a member. Returns sql.ErrNoRows when they are not.
func getWorkspaceForUser(db *sql.DB, userID, workspaceID string) (Workspace, error) {
	const stmt = `
		SELECT w.id, w.slug, w.name, w.owner_user_id, m.role, w.created_at, w.updated_at
		FROM ap_workspace w
		JOIN ap_workspace_member m ON m.workspace_id = w.id
		WHERE w.id = $1 AND m.user_id = $2
	`
	var w Workspace
	err := db.QueryRow(stmt, workspaceID, userID).Scan(
		&w.ID, &w.Slug, &w.Name, &w.OwnerUserID, &w.MyRole,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return Workspace{}, err
	}
	return w, nil
}

// insertWorkspaceTx creates workspace + owner membership + counter atomically.
func insertWorkspaceTx(db *sql.DB, userID, slug, name, role string) (Workspace, error) {
	tx, err := db.Begin()
	if err != nil {
		return Workspace{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var w Workspace
	err = tx.QueryRow(
		`INSERT INTO ap_workspace (slug, name, owner_user_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, slug, name, owner_user_id, created_at, updated_at`,
		slug, name, userID,
	).Scan(&w.ID, &w.Slug, &w.Name, &w.OwnerUserID, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return Workspace{}, fmt.Errorf("insert workspace: %w", err)
	}

	if _, err := tx.Exec(
		`INSERT INTO ap_workspace_member (workspace_id, user_id, role)
		 VALUES ($1, $2, $3)`,
		w.ID, userID, role,
	); err != nil {
		return Workspace{}, fmt.Errorf("insert membership: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO ap_workspace_counter (workspace_id, next_issue_number)
		 VALUES ($1, 1)`,
		w.ID,
	); err != nil {
		return Workspace{}, fmt.Errorf("insert counter: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Workspace{}, fmt.Errorf("commit: %w", err)
	}
	w.MyRole = role
	return w, nil
}

// -----------------------------------------------------------------------------
// Small helpers
// -----------------------------------------------------------------------------

// validSlug allows [a-z0-9-] of length 2-40, not starting or ending with '-'.
func validSlug(s string) bool {
	if len(s) < 2 || len(s) > 40 {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

// isUniqueViolation reports whether an error is a Postgres unique-constraint
// violation. Matches on error text to avoid importing the pq package just
// for SQLSTATE 23505; lib/pq's format is stable.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate key value") ||
		strings.Contains(s, "23505") ||
		strings.Contains(s, "unique constraint")
}
