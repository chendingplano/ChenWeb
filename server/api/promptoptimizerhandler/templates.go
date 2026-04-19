package promptoptimizerhandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var errTemplateNotFound = errors.New("promptoptimizerhandler: template not found")

// -----------------------------------------------------------------------
// Store
// -----------------------------------------------------------------------

func listTemplatesForUser(db *sql.DB, userID, kindFilter string) ([]Template, error) {
	base := `
		SELECT id, user_id, kind, name, COALESCE(description, ''), content,
		       language, variables, (user_id IS NULL) AS built_in,
		       created_at, updated_at
		FROM prompt_optimizer_templates
		WHERE (user_id = $1 OR user_id IS NULL)
	`
	args := []any{userID}
	if kindFilter != "" {
		base += " AND kind = $2"
		args = append(args, kindFilter)
	}
	base += " ORDER BY built_in ASC, created_at DESC"

	rows, err := db.Query(base, args...)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	out := make([]Template, 0)
	for rows.Next() {
		var t Template
		var uid sql.NullString
		var vars []byte
		if err := rows.Scan(
			&t.ID, &uid, &t.Kind, &t.Name, &t.Description, &t.Content,
			&t.Language, &vars, &t.BuiltIn, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		if uid.Valid {
			v := uid.String
			t.UserID = &v
		}
		t.Variables = json.RawMessage(vars)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate templates: %w", err)
	}
	return out, nil
}

func getTemplateForUser(db *sql.DB, userID, id string) (*Template, error) {
	const stmt = `
		SELECT id, user_id, kind, name, COALESCE(description, ''), content,
		       language, variables, (user_id IS NULL) AS built_in,
		       created_at, updated_at
		FROM prompt_optimizer_templates
		WHERE id = $1 AND (user_id = $2 OR user_id IS NULL)
	`
	var t Template
	var uid sql.NullString
	var vars []byte
	err := db.QueryRow(stmt, id, userID).Scan(
		&t.ID, &uid, &t.Kind, &t.Name, &t.Description, &t.Content,
		&t.Language, &vars, &t.BuiltIn, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errTemplateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get template: %w", err)
	}
	if uid.Valid {
		v := uid.String
		t.UserID = &v
	}
	t.Variables = json.RawMessage(vars)
	return &t, nil
}

func insertTemplate(
	db *sql.DB,
	userID, kind, name, description, content, language string,
	variables json.RawMessage,
) (string, error) {
	if len(variables) == 0 {
		variables = json.RawMessage(`[]`)
	}
	if language == "" {
		language = "en"
	}
	const stmt = `
		INSERT INTO prompt_optimizer_templates
		    (user_id, kind, name, description, content, language, variables)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7)
		RETURNING id
	`
	var id string
	err := db.QueryRow(stmt, userID, kind, name, description, content, language, []byte(variables)).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert template: %w", err)
	}
	return id, nil
}

func updateTemplateForUser(
	db *sql.DB,
	userID, id, kind, name, description, content, language string,
	variables json.RawMessage,
) error {
	if len(variables) == 0 {
		variables = json.RawMessage(`[]`)
	}
	if language == "" {
		language = "en"
	}
	const stmt = `
		UPDATE prompt_optimizer_templates SET
		    kind        = $3,
		    name        = $4,
		    description = NULLIF($5, ''),
		    content     = $6,
		    language    = $7,
		    variables   = $8,
		    updated_at  = NOW()
		WHERE id = $1 AND user_id = $2
	`
	res, err := db.Exec(stmt, id, userID, kind, name, description, content, language, []byte(variables))
	if err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errTemplateNotFound
	}
	return nil
}

func deleteTemplateForUser(db *sql.DB, userID, id string) error {
	const stmt = `DELETE FROM prompt_optimizer_templates WHERE id = $1 AND user_id = $2`
	res, err := db.Exec(stmt, id, userID)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errTemplateNotFound
	}
	return nil
}

// -----------------------------------------------------------------------
// Seed (built-in) templates
// -----------------------------------------------------------------------

type seedTemplate struct {
	Kind, Name, Description, Content string
	Variables                        []string
}

var builtinSeeds = []seedTemplate{
	{
		Kind:        "optimize",
		Name:        "Analytical Optimizer",
		Description: "Rewrite a user prompt with clearer structure, constraints, and examples.",
		Content:     "You are an expert prompt engineer. Rewrite the user's prompt below to be clearer, more specific, and more likely to produce excellent results. Keep the user's intent, but add structure (role, task, context, format, constraints). Return ONLY the rewritten prompt.\n\nUSER PROMPT:\n{{originalPrompt}}",
		Variables:   []string{"originalPrompt"},
	},
	{
		Kind:        "iterate",
		Name:        "Iterate with Feedback",
		Description: "Refine an already-optimized prompt given user feedback.",
		Content:     "You previously produced the following optimized prompt:\n\n{{optimizedPrompt}}\n\nThe user requested the following changes:\n\n{{feedback}}\n\nRewrite the optimized prompt applying those changes. Keep the overall structure. Return ONLY the rewritten prompt.",
		Variables:   []string{"optimizedPrompt", "feedback"},
	},
	{
		Kind:        "test",
		Name:        "Test Optimized Prompt",
		Description: "Run an optimized prompt against sample input to preview its output.",
		Content:     "{{optimizedPrompt}}\n\nInput:\n{{testInput}}",
		Variables:   []string{"optimizedPrompt", "testInput"},
	},
	{
		Kind:        "analyze",
		Name:        "Analyze Prompt Quality",
		Description: "Rate a prompt across clarity, specificity, safety, and completeness.",
		Content:     "Analyze the following prompt and return a short rubric assessment covering:\n- Clarity\n- Specificity\n- Safety / bias risks\n- Missing constraints or context\nFinish with 3 concrete suggestions.\n\nPROMPT:\n{{originalPrompt}}",
		Variables:   []string{"originalPrompt"},
	},
}

// SeedBuiltinTemplates inserts each seed row if it is not already present.
// Idempotent — safe to call on every startup.
func SeedBuiltinTemplates(db *sql.DB) error {
	if db == nil {
		return errors.New("SeedBuiltinTemplates: db is nil")
	}
	const checkStmt = `
		SELECT 1 FROM prompt_optimizer_templates
		WHERE user_id IS NULL AND kind = $1 AND name = $2
	`
	const insertStmt = `
		INSERT INTO prompt_optimizer_templates
		    (user_id, kind, name, description, content, language, variables)
		VALUES (NULL, $1, $2, $3, $4, 'en', $5)
	`
	for _, s := range builtinSeeds {
		var exists int
		err := db.QueryRow(checkStmt, s.Kind, s.Name).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check seed %q: %w", s.Name, err)
		}
		vars, _ := json.Marshal(s.Variables)
		if _, err := db.Exec(insertStmt, s.Kind, s.Name, s.Description, s.Content, vars); err != nil {
			return fmt.Errorf("insert seed %q: %w", s.Name, err)
		}
	}
	return nil
}

// -----------------------------------------------------------------------
// HTTP handlers
// -----------------------------------------------------------------------

func ListTemplates(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_200")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_201")
	defer rc.Close()
	logger := rc.GetLogger()

	kind := strings.TrimSpace(c.QueryParam("kind"))
	if kind != "" && !validTemplateKinds[kind] {
		return jsonError(c, http.StatusBadRequest, "unsupported kind (CWB_POH_202)")
	}

	templates, err := listTemplatesForUser(ApiTypes.ProjectDBHandle, userID, kind)
	if err != nil {
		logger.Error("list templates failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to list templates (CWB_POH_203)")
	}
	return c.JSON(http.StatusOK, TemplateListResponse{Status: true, Templates: templates})
}

func GetTemplate(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_210")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_211")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_212)")
	}

	t, err := getTemplateForUser(ApiTypes.ProjectDBHandle, userID, id)
	if errors.Is(err, errTemplateNotFound) {
		return jsonError(c, http.StatusNotFound, "template not found (CWB_POH_213)")
	}
	if err != nil {
		logger.Error("get template failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to get template (CWB_POH_214)")
	}
	return c.JSON(http.StatusOK, TemplateResponse{Status: true, Template: t})
}

func CreateTemplate(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_220")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_221")
	defer rc.Close()
	logger := rc.GetLogger()

	var req CreateTemplateRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_222)")
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	req.Content = strings.TrimSpace(req.Content)

	if req.Name == "" || req.Content == "" {
		return jsonError(c, http.StatusBadRequest, "name and content are required (CWB_POH_223)")
	}
	if !validTemplateKinds[req.Kind] {
		return jsonError(c, http.StatusBadRequest, "unsupported kind (CWB_POH_224)")
	}

	id, err := insertTemplate(ApiTypes.ProjectDBHandle, userID, req.Kind, req.Name, req.Description, req.Content, req.Language, req.Variables)
	if err != nil {
		logger.Error("insert template failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to create template (CWB_POH_225)")
	}
	t, err := getTemplateForUser(ApiTypes.ProjectDBHandle, userID, id)
	if err != nil {
		logger.Error("reload template failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to read new template (CWB_POH_226)")
	}
	return c.JSON(http.StatusCreated, TemplateResponse{Status: true, Template: t})
}

func UpdateTemplate(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_230")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_231")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_232)")
	}
	var req UpdateTemplateRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_233)")
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	req.Content = strings.TrimSpace(req.Content)
	if req.Name == "" || req.Content == "" {
		return jsonError(c, http.StatusBadRequest, "name and content are required (CWB_POH_234)")
	}
	if !validTemplateKinds[req.Kind] {
		return jsonError(c, http.StatusBadRequest, "unsupported kind (CWB_POH_235)")
	}

	err := updateTemplateForUser(ApiTypes.ProjectDBHandle, userID, id, req.Kind, req.Name, req.Description, req.Content, req.Language, req.Variables)
	if errors.Is(err, errTemplateNotFound) {
		return jsonError(c, http.StatusNotFound, "template not found (CWB_POH_236)")
	}
	if err != nil {
		logger.Error("update template failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to update template (CWB_POH_237)")
	}
	t, err := getTemplateForUser(ApiTypes.ProjectDBHandle, userID, id)
	if err != nil {
		logger.Error("reload template failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to read updated template (CWB_POH_238)")
	}
	return c.JSON(http.StatusOK, TemplateResponse{Status: true, Template: t})
}

func DeleteTemplate(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_240")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_241")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_242)")
	}

	err := deleteTemplateForUser(ApiTypes.ProjectDBHandle, userID, id)
	if errors.Is(err, errTemplateNotFound) {
		return jsonError(c, http.StatusNotFound, "template not found (CWB_POH_243)")
	}
	if err != nil {
		logger.Error("delete template failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to delete template (CWB_POH_244)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
