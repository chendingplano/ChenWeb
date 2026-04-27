package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	kbSkillsTableSingular      = "kb.skill"
	kbSkillsTablePlural        = "kb.skills"
	kbSkillCategoriesTableName = "kb.skill_categories"
)

// --- record types ---

type skillRecord struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Category    []string        `json:"category"`
	Status      *string         `json:"status,omitempty"`
	Tags        []string        `json:"tags"`
	Version     *string         `json:"version,omitempty"`
	Notes       *string         `json:"notes,omitempty"`
	PublicInfo  json.RawMessage `json:"public_info,omitempty"`
	CreateTime  time.Time       `json:"create_time"`
	ModifyTime  time.Time       `json:"modify_time"`
}

type skillCategoryRecord struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Level       int      `json:"level"`
	ParentID    *int64   `json:"parent_id,omitempty"`
	Path        []string `json:"path"`
	Description *string  `json:"description,omitempty"`
}

// --- helpers ---

func resolveSkillsTable(db *sql.DB) (string, error) {
	const query = `
SELECT
	to_regclass($1)::text AS singular,
	to_regclass($2)::text AS plural
`
	var singular, plural sql.NullString
	if err := db.QueryRow(query, kbSkillsTableSingular, kbSkillsTablePlural).Scan(&singular, &plural); err != nil {
		return "", err
	}
	if singular.Valid && strings.TrimSpace(singular.String) != "" {
		return singular.String, nil
	}
	if plural.Valid && strings.TrimSpace(plural.String) != "" {
		return plural.String, nil
	}
	return "", fmt.Errorf("neither %s nor %s exists", kbSkillsTableSingular, kbSkillsTablePlural)
}

func tableExists(db *sql.DB, tableName string) bool {
	var result sql.NullString
	_ = db.QueryRow(`SELECT to_regclass($1)::text`, tableName).Scan(&result)
	return result.Valid && strings.TrimSpace(result.String) != ""
}

func scanSkill(rows *sql.Rows) (skillRecord, error) {
	var r skillRecord
	var category, tags pq.StringArray
	var publicInfoRaw sql.NullString
	if err := rows.Scan(
		&r.ID, &r.Name, &r.Description, &category, &r.Status,
		&tags, &r.Version, &r.Notes, &publicInfoRaw,
		&r.CreateTime, &r.ModifyTime,
	); err != nil {
		return r, err
	}
	r.Category = []string(category)
	r.Tags = []string(tags)
	if publicInfoRaw.Valid && strings.TrimSpace(publicInfoRaw.String) != "" {
		r.PublicInfo = json.RawMessage(publicInfoRaw.String)
	}
	return r, nil
}

// --- handlers ---

// ListSkillCategories returns all skill categories from kb.skill_categories.
// GET /api/v1/kb/skill-categories
func ListSkillCategories(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SC_010")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if !tableExists(db, kbSkillCategoriesTableName) {
		logger.Info("skill_categories table does not exist, returning empty list")
		return c.JSON(http.StatusOK, map[string]any{"status": true, "results": []skillCategoryRecord{}, "total": 0})
	}

	rows, err := db.Query(`
SELECT id, "name", "level", parent_id, "path", description
FROM kb.skill_categories
ORDER BY "level" ASC, id ASC
`)
	if err != nil {
		logger.Error("query skill categories failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve skill categories (CWB_KB_SC_011)"})
	}
	defer rows.Close()

	results := make([]skillCategoryRecord, 0)
	for rows.Next() {
		var r skillCategoryRecord
		var path pq.StringArray
		if err := rows.Scan(&r.ID, &r.Name, &r.Level, &r.ParentID, &path, &r.Description); err != nil {
			logger.Error("scan skill category failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan skill categories (CWB_KB_SC_012)"})
		}
		r.Path = []string(path)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate skill categories failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate skill categories (CWB_KB_SC_013)"})
	}

	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": results, "total": len(results)})
}

// ListSkills returns skills from kb.skills, optionally filtered by category path.
// GET /api/v1/kb/skills?category=<path_json>&status=<active|inactive>&page=1&page_size=50
func ListSkills(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SK_010")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	skillsTable, err := resolveSkillsTable(db)
	if err != nil {
		logger.Info("skills table does not exist, returning empty list", "err", err)
		return c.JSON(http.StatusOK, map[string]any{"status": true, "results": []skillRecord{}, "total": 0, "page": 1, "page_size": 50})
	}

	page := 1
	pageSize := 50
	if v := c.QueryParam("page"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil && n > 0 {
			page = n
		}
	}
	if v := c.QueryParam("page_size"); v != "" {
		if n, err2 := strconv.Atoi(v); err2 == nil && n > 0 && n <= maxPageSize {
			pageSize = n
		}
	}

	conditions := []string{}
	args := []any{}
	argIdx := 1

	if catRaw := c.QueryParam("category"); catRaw != "" {
		var catPath []string
		if err2 := json.Unmarshal([]byte(catRaw), &catPath); err2 == nil && len(catPath) > 0 {
			// Match skills whose category starts with the given path prefix
			args = append(args, pq.Array(catPath))
			conditions = append(conditions, fmt.Sprintf("category[1:array_length($%d::text[], 1)] = $%d::text[]", argIdx, argIdx))
			argIdx++
		}
	}

	if status := strings.TrimSpace(c.QueryParam("status")); status != "" {
		args = append(args, status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", skillsTable, where)
	var total int64
	if err2 := db.QueryRow(countQuery, args...).Scan(&total); err2 != nil {
		logger.Error("count skills failed", "err", err2)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to count skills (CWB_KB_SK_011)"})
	}

	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	listQuery := fmt.Sprintf(`
SELECT id, "name", description, category, status, tags, version, notes, public_info, create_time, modify_time
FROM %s
%s
ORDER BY modify_time DESC, id DESC
LIMIT $%d OFFSET $%d
`, skillsTable, where, argIdx, argIdx+1)

	rows, err := db.Query(listQuery, args...)
	if err != nil {
		logger.Error("query skills failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve skills (CWB_KB_SK_012)"})
	}
	defer rows.Close()

	results := make([]skillRecord, 0)
	for rows.Next() {
		r, err2 := scanSkill(rows)
		if err2 != nil {
			logger.Error("scan skill failed", "err", err2)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan skills (CWB_KB_SK_013)"})
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate skills failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to iterate skills (CWB_KB_SK_014)"})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":    true,
		"results":   results,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// CreateSkill inserts a new skill record.
// POST /api/v1/kb/skills
func CreateSkill(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SK_050")
	defer rc.Close()
	logger := rc.GetLogger()

	var payload struct {
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		Category    []string `json:"category"`
		Status      *string  `json:"status"`
		Tags        []string `json:"tags"`
		Version     *string  `json:"version"`
		Notes       *string  `json:"notes"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_SK_051)"})
	}
	if strings.TrimSpace(payload.Name) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_SK_052)"})
	}

	db := ApiTypes.ProjectDBHandle
	skillsTable, err := resolveSkillsTable(db)
	if err != nil {
		logger.Error("skills table not found", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "skills table not found (CWB_KB_SK_053)"})
	}

	if payload.Category == nil {
		payload.Category = []string{}
	}
	if payload.Tags == nil {
		payload.Tags = []string{}
	}

	query := fmt.Sprintf(`
INSERT INTO %s ("name", description, category, status, tags, version, notes, create_time, modify_time)
VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
RETURNING id, "name", description, category, status, tags, version, notes, public_info, create_time, modify_time
`, skillsTable)

	rows, err := db.Query(query,
		payload.Name, payload.Description, pq.Array(payload.Category), payload.Status,
		pq.Array(payload.Tags), payload.Version, payload.Notes,
	)
	if err != nil {
		logger.Error("create skill failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create skill (CWB_KB_SK_054)"})
	}
	defer rows.Close()

	if !rows.Next() {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve created skill (CWB_KB_SK_055)"})
	}
	r, err := scanSkill(rows)
	if err != nil {
		logger.Error("scan created skill failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to scan created skill (CWB_KB_SK_056)"})
	}

	return c.JSON(http.StatusCreated, r)
}

// UpdateSkill updates a skill record.
// PUT /api/v1/kb/skills/:id
func UpdateSkill(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SK_100")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_SK_101)"})
	}

	var payload struct {
		Name        string   `json:"name"`
		Description *string  `json:"description"`
		Category    []string `json:"category"`
		Status      *string  `json:"status"`
		Tags        []string `json:"tags"`
		Version     *string  `json:"version"`
		Notes       *string  `json:"notes"`
	}
	if err2 := json.NewDecoder(c.Request().Body).Decode(&payload); err2 != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_SK_102)"})
	}
	if strings.TrimSpace(payload.Name) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_SK_103)"})
	}

	db := ApiTypes.ProjectDBHandle
	skillsTable, err := resolveSkillsTable(db)
	if err != nil {
		logger.Error("skills table not found", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "skills table not found (CWB_KB_SK_104)"})
	}

	if payload.Category == nil {
		payload.Category = []string{}
	}
	if payload.Tags == nil {
		payload.Tags = []string{}
	}

	query := fmt.Sprintf(`
UPDATE %s
SET "name" = $1, description = $2, category = $3, status = $4,
    tags = $5, version = $6, notes = $7, modify_time = NOW()
WHERE id = $8
`, skillsTable)

	result, err := db.Exec(query,
		payload.Name, payload.Description, pq.Array(payload.Category), payload.Status,
		pq.Array(payload.Tags), payload.Version, payload.Notes, id,
	)
	if err != nil {
		logger.Error("update skill failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to update skill (CWB_KB_SK_105)"})
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_SK_106)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}

// DeleteSkillRecord deletes a skill by id.
// DELETE /api/v1/kb/skills/:id
func DeleteSkillRecord(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SK_200")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_SK_201)"})
	}

	db := ApiTypes.ProjectDBHandle
	skillsTable, err := resolveSkillsTable(db)
	if err != nil {
		logger.Error("skills table not found", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "skills table not found (CWB_KB_SK_202)"})
	}

	query := fmt.Sprintf("DELETE FROM %s WHERE id = $1", skillsTable)
	result, err := db.Exec(query, id)
	if err != nil {
		logger.Error("delete skill failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete skill (CWB_KB_SK_203)"})
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (CWB_KB_SK_204)"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}
