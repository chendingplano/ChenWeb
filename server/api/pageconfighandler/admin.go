package pageconfighandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

// pageDef is one kb.page_def row for admin listing.
type pageDef struct {
	PageKey     string `json:"page_key"`
	Route       string `json:"route"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// adminEntry is one configurable entry with every language's content plus the
// access controls and admin metadata (taken from the default-language row,
// which is authoritative).
type adminEntry struct {
	EntryKey   string                     `json:"entry_key"`
	EntryDesc  string                     `json:"entry_desc"`
	Content    map[string]json.RawMessage `json:"content"`
	AccessRole []string                   `json:"access_role"`
	Accessible bool                       `json:"accessible"`
	Enabled    bool                       `json:"enabled"`
}

type upsertEntryRequest struct {
	EntryKey   string                     `json:"entry_key"`
	EntryDesc  string                     `json:"entry_desc"`
	Content    map[string]json.RawMessage `json:"content"`
	AccessRole []string                   `json:"access_role"`
	Accessible *bool                      `json:"accessible"`
	Enabled    *bool                      `json:"enabled"`
}

// requireAdmin authorizes admin write operations: an authenticated user who is
// admin/owner or holds the "admin" role (mirrors useradminhandler).
func requireAdmin(c echo.Context, loc string) (*ApiTypes.UserInfo, ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		return nil, rc, c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required", "loc": loc})
	}
	if !user.Admin && !user.IsOwner && !slices.Contains(user.Roles, "admin") {
		return nil, rc, c.JSON(http.StatusForbidden, map[string]string{"error": "Admin access required", "loc": loc})
	}
	return user, rc, nil
}

func requireAuth(c echo.Context, loc string) (*ApiTypes.UserInfo, ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		return nil, rc, c.JSON(http.StatusUnauthorized, map[string]string{"error": "Authentication required", "loc": loc})
	}
	return user, rc, nil
}

// ListPages returns all configurable page defs. Authenticated read.
// Endpoint: GET /api/v1/page-config/admin/pages
func ListPages(c echo.Context) error {
	_, rc, err := requireAuth(c, "CWB_PGC_010")
	if err != nil {
		return err
	}
	defer rc.Close()

	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
		SELECT page_key, COALESCE(route,''), COALESCE(title,''), COALESCE(description,'')
		FROM kb.page_def ORDER BY page_key`)
	if err != nil {
		rc.GetLogger().Error("page-config admin: list pages failed", "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load pages"})
	}
	defer rows.Close()

	out := []pageDef{}
	for rows.Next() {
		var p pageDef
		if err := rows.Scan(&p.PageKey, &p.Route, &p.Title, &p.Description); err != nil {
			rc.GetLogger().Error("page-config admin: scan page failed", "err", err)
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load pages"})
		}
		out = append(out, p)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "pages": out})
}

// ListEntries returns all entries for a page, each with every language's content
// and the access controls from the default-language row. Authenticated read.
// Endpoint: GET /api/v1/page-config/admin/pages/:pageKey/entries
func ListEntries(c echo.Context) error {
	_, rc, err := requireAuth(c, "CWB_PGC_011")
	if err != nil {
		return err
	}
	defer rc.Close()

	pageKey := strings.TrimSpace(c.Param("pageKey"))
	ctx := c.Request().Context()

	exists, err := pageExists(ctx, ApiTypes.ProjectDBHandle, pageKey)
	if err != nil {
		rc.GetLogger().Error("page-config admin: page lookup failed", "page_key", pageKey, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load entries"})
	}
	if !exists {
		rc.GetLogger().Warn("page-config admin: unknown page_key", "page_key", pageKey)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown page_key", "page_key": pageKey})
	}

	rows, err := loadPageConfigRows(ctx, ApiTypes.ProjectDBHandle, pageKey)
	if err != nil {
		rc.GetLogger().Error("page-config admin: load rows failed", "page_key", pageKey, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load entries"})
	}

	defaultLang := appconfig.GetDefaultLanguage()
	grouped := groupByEntry(rows)
	out := []adminEntry{}
	for entryKey, e := range grouped {
		entry := adminEntry{EntryKey: entryKey, Content: map[string]json.RawMessage{}, AccessRole: []string{}}
		for lang, row := range e.byLang {
			entry.Content[lang] = row.Content
		}
		if def, ok := e.byLang[defaultLang]; ok {
			entry.Accessible = def.Accessible
			entry.Enabled = def.Enabled
			entry.EntryDesc = def.EntryDesc
			if def.AccessRole != nil {
				entry.AccessRole = def.AccessRole
			}
		}
		out = append(out, entry)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "page_key": pageKey, "entries": out})
}

// UpsertEntry creates or updates an entry, writing one row per configured
// language. Access controls (access_role/accessible/enabled) are written to
// every language row so the default-language row is always authoritative and in
// sync. Admin write.
// Endpoints: POST /api/v1/page-config/admin/pages/:pageKey/entries
//            PUT  /api/v1/page-config/admin/pages/:pageKey/entries/:entryKey
func UpsertEntry(c echo.Context) error {
	user, rc, err := requireAdmin(c, "CWB_PGC_012")
	if err != nil {
		return err
	}
	defer rc.Close()

	pageKey := strings.TrimSpace(c.Param("pageKey"))
	ctx := c.Request().Context()

	exists, err := pageExists(ctx, ApiTypes.ProjectDBHandle, pageKey)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load page"})
	}
	if !exists {
		rc.GetLogger().Warn("page-config admin: upsert into unknown page_key", "page_key", pageKey)
		return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown page_key", "page_key": pageKey})
	}

	var req upsertEntryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	entryKey := strings.TrimSpace(req.EntryKey)
	if entryKey == "" {
		entryKey = strings.TrimSpace(c.Param("entryKey"))
	}
	if entryKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "entry_key is required"})
	}

	accessible := true
	if req.Accessible != nil {
		accessible = *req.Accessible
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	accessRoleJSON, _ := json.Marshal(normalizeRoles(req.AccessRole))
	actor := actorName(user)

	if err := upsertEntryRows(ctx, ApiTypes.ProjectDBHandle, pageKey, entryKey, strings.TrimSpace(req.EntryDesc), req.Content, accessRoleJSON, accessible, enabled, actor); err != nil {
		rc.GetLogger().Error("page-config admin: upsert failed", "page_key", pageKey, "entry_key", entryKey, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to save entry"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "page_key": pageKey, "entry_key": entryKey})
}

// DeleteEntry removes all per-language rows for an entry. Admin write.
// Endpoint: DELETE /api/v1/page-config/admin/pages/:pageKey/entries/:entryKey
func DeleteEntry(c echo.Context) error {
	_, rc, err := requireAdmin(c, "CWB_PGC_013")
	if err != nil {
		return err
	}
	defer rc.Close()

	pageKey := strings.TrimSpace(c.Param("pageKey"))
	entryKey := strings.TrimSpace(c.Param("entryKey"))
	if pageKey == "" || entryKey == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "page_key and entry_key are required"})
	}

	res, err := ApiTypes.ProjectDBHandle.ExecContext(c.Request().Context(),
		`DELETE FROM kb.page_config WHERE page_key = $1 AND entry_key = $2`, pageKey, entryKey)
	if err != nil {
		rc.GetLogger().Error("page-config admin: delete failed", "page_key", pageKey, "entry_key", entryKey, "err", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to delete entry"})
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		rc.GetLogger().Warn("page-config admin: delete matched no rows", "page_key", pageKey, "entry_key", entryKey)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "deleted": n})
}

// upsertEntryRows writes one row per configured language in a transaction. For
// languages present in content it stores that content; for the rest it stores
// {} (so the entry stays visible and falls back to defaults). Access controls
// are identical across all language rows.
func upsertEntryRows(ctx context.Context, db *sql.DB, pageKey, entryKey, entryDesc string, content map[string]json.RawMessage, accessRoleJSON []byte, accessible, enabled bool, actor string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, lang := range appconfig.GetLanguages() {
		payload := json.RawMessage("{}")
		if v, ok := content[lang]; ok && len(v) > 0 {
			payload = v
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO kb.page_config
				(page_key, entry_key, language, content, access_role, accessible, enabled, entry_desc, created_by, updated_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
			ON CONFLICT (page_key, entry_key, language) DO UPDATE SET
				content = EXCLUDED.content,
				access_role = EXCLUDED.access_role,
				accessible = EXCLUDED.accessible,
				enabled = EXCLUDED.enabled,
				entry_desc = EXCLUDED.entry_desc,
				updated_by = EXCLUDED.updated_by,
				updated_at = NOW()`,
			pageKey, entryKey, lang, []byte(payload), accessRoleJSON, accessible, enabled, entryDesc, actor); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// normalizeRoles trims, lower-cases, and de-duplicates role tokens.
func normalizeRoles(roles []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range roles {
		key := strings.ToLower(strings.TrimSpace(r))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func actorName(user *ApiTypes.UserInfo) string {
	if user == nil {
		return ""
	}
	if user.Email != "" {
		return user.Email
	}
	return user.UserName
}
