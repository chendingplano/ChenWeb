package promptoptimizerhandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

var (
	errHistoryNotFound  = errors.New("promptoptimizerhandler: history not found")
	errFavoriteNotFound = errors.New("promptoptimizerhandler: favorite not found")
	errVariableNotFound = errors.New("promptoptimizerhandler: variable not found")
)

// -----------------------------------------------------------------------
// History
// -----------------------------------------------------------------------

func insertHistory(
	db *sql.DB,
	userID, kind, originalPrompt, optimizedPrompt, testInput, testOutput string,
	templateID, modelID *string,
	params json.RawMessage,
) (string, error) {
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	const stmt = `
		INSERT INTO prompt_optimizer_history
		    (user_id, kind, original_prompt, optimized_prompt, test_input, test_output,
		     template_id, model_id, params)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''),
		        NULLIF($7, ''), NULLIF($8, ''), $9)
		RETURNING id
	`
	tid := ""
	if templateID != nil {
		tid = *templateID
	}
	mid := ""
	if modelID != nil {
		mid = *modelID
	}
	var id string
	err := db.QueryRow(stmt,
		userID, kind, originalPrompt, optimizedPrompt, testInput, testOutput,
		tid, mid, []byte(params),
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert history: %w", err)
	}
	return id, nil
}

func listHistoryForUser(db *sql.DB, userID, kindFilter string, page, pageSize int) ([]History, int64, error) {
	if pageSize <= 0 {
		pageSize = 25
	}
	if page < 0 {
		page = 0
	}

	countArgs := []any{userID}
	countSQL := `SELECT COUNT(*) FROM prompt_optimizer_history WHERE user_id = $1`
	if kindFilter != "" {
		countSQL += " AND kind = $2"
		countArgs = append(countArgs, kindFilter)
	}
	var total int64
	if err := db.QueryRow(countSQL, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count history: %w", err)
	}

	listArgs := []any{userID}
	listSQL := `
		SELECT id, user_id, kind,
		       COALESCE(original_prompt, ''), COALESCE(optimized_prompt, ''),
		       COALESCE(test_input, ''), COALESCE(test_output, ''),
		       template_id, model_id, params, created_at
		FROM prompt_optimizer_history
		WHERE user_id = $1
	`
	idx := 2
	if kindFilter != "" {
		listSQL += fmt.Sprintf(" AND kind = $%d", idx)
		listArgs = append(listArgs, kindFilter)
		idx++
	}
	listSQL += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	listArgs = append(listArgs, pageSize, page*pageSize)

	rows, err := db.Query(listSQL, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list history: %w", err)
	}
	defer rows.Close()

	out := make([]History, 0)
	for rows.Next() {
		var h History
		var tid, mid sql.NullString
		var params []byte
		if err := rows.Scan(
			&h.ID, &h.UserID, &h.Kind,
			&h.OriginalPrompt, &h.OptimizedPrompt,
			&h.TestInput, &h.TestOutput,
			&tid, &mid, &params, &h.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan history: %w", err)
		}
		if tid.Valid {
			v := tid.String
			h.TemplateID = &v
		}
		if mid.Valid {
			v := mid.String
			h.ModelID = &v
		}
		h.Params = json.RawMessage(params)
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate history: %w", err)
	}
	return out, total, nil
}

func deleteHistoryForUser(db *sql.DB, userID, id string) error {
	const stmt = `DELETE FROM prompt_optimizer_history WHERE id = $1 AND user_id = $2`
	res, err := db.Exec(stmt, id, userID)
	if err != nil {
		return fmt.Errorf("delete history: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errHistoryNotFound
	}
	return nil
}

func ListHistory(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_300")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_301")
	defer rc.Close()
	logger := rc.GetLogger()

	kind := strings.TrimSpace(c.QueryParam("kind"))
	if kind != "" && !validHistoryKinds[kind] {
		return jsonError(c, http.StatusBadRequest, "unsupported kind (CWB_POH_302)")
	}
	page, _ := strconv.Atoi(c.QueryParam("page"))
	pageSize, _ := strconv.Atoi(c.QueryParam("page_size"))
	if pageSize == 0 {
		pageSize = 25
	}

	history, total, err := listHistoryForUser(ApiTypes.ProjectDBHandle, userID, kind, page, pageSize)
	if err != nil {
		logger.Error("list history failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to list history (CWB_POH_303)")
	}
	return c.JSON(http.StatusOK, HistoryListResponse{
		Status: true, History: history, Total: total, Page: page, PageSize: pageSize,
	})
}

func DeleteHistory(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_310")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_311")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_312)")
	}
	err := deleteHistoryForUser(ApiTypes.ProjectDBHandle, userID, id)
	if errors.Is(err, errHistoryNotFound) {
		return jsonError(c, http.StatusNotFound, "history not found (CWB_POH_313)")
	}
	if err != nil {
		logger.Error("delete history failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to delete history (CWB_POH_314)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// -----------------------------------------------------------------------
// Favorites
// -----------------------------------------------------------------------

func listFavoritesForUser(db *sql.DB, userID string) ([]Favorite, error) {
	const stmt = `
		SELECT id, user_id, ref_kind, ref_id, COALESCE(note, ''), created_at
		FROM prompt_optimizer_favorites
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := db.Query(stmt, userID)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	out := make([]Favorite, 0)
	for rows.Next() {
		var f Favorite
		if err := rows.Scan(&f.ID, &f.UserID, &f.RefKind, &f.RefID, &f.Note, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate favorites: %w", err)
	}
	return out, nil
}

func insertFavorite(db *sql.DB, userID, refKind, refID, note string) (string, error) {
	const stmt = `
		INSERT INTO prompt_optimizer_favorites (user_id, ref_kind, ref_id, note)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		ON CONFLICT (user_id, ref_kind, ref_id) DO UPDATE
		SET note = EXCLUDED.note
		RETURNING id
	`
	var id string
	if err := db.QueryRow(stmt, userID, refKind, refID, note).Scan(&id); err != nil {
		return "", fmt.Errorf("insert favorite: %w", err)
	}
	return id, nil
}

func deleteFavoriteForUser(db *sql.DB, userID, id string) error {
	const stmt = `DELETE FROM prompt_optimizer_favorites WHERE id = $1 AND user_id = $2`
	res, err := db.Exec(stmt, id, userID)
	if err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errFavoriteNotFound
	}
	return nil
}

func ListFavorites(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_400")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_401")
	defer rc.Close()
	logger := rc.GetLogger()

	favs, err := listFavoritesForUser(ApiTypes.ProjectDBHandle, userID)
	if err != nil {
		logger.Error("list favorites failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to list favorites (CWB_POH_402)")
	}
	return c.JSON(http.StatusOK, FavoriteListResponse{Status: true, Favorites: favs})
}

func CreateFavorite(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_410")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_411")
	defer rc.Close()
	logger := rc.GetLogger()

	var req CreateFavoriteRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_412)")
	}
	req.RefKind = strings.TrimSpace(req.RefKind)
	req.RefID = strings.TrimSpace(req.RefID)
	if req.RefKind != "history" && req.RefKind != "template" {
		return jsonError(c, http.StatusBadRequest, "ref_kind must be 'history' or 'template' (CWB_POH_413)")
	}
	if req.RefID == "" {
		return jsonError(c, http.StatusBadRequest, "ref_id is required (CWB_POH_414)")
	}

	id, err := insertFavorite(ApiTypes.ProjectDBHandle, userID, req.RefKind, req.RefID, req.Note)
	if err != nil {
		logger.Error("insert favorite failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to add favorite (CWB_POH_415)")
	}
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "id": id})
}

func DeleteFavorite(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_420")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_421")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_422)")
	}
	err := deleteFavoriteForUser(ApiTypes.ProjectDBHandle, userID, id)
	if errors.Is(err, errFavoriteNotFound) {
		return jsonError(c, http.StatusNotFound, "favorite not found (CWB_POH_423)")
	}
	if err != nil {
		logger.Error("delete favorite failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to remove favorite (CWB_POH_424)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// -----------------------------------------------------------------------
// Variables
// -----------------------------------------------------------------------

func listVariablesForUser(db *sql.DB, userID string) ([]Variable, error) {
	const stmt = `
		SELECT id, user_id, name, value, created_at, updated_at
		FROM prompt_optimizer_variables
		WHERE user_id = $1
		ORDER BY name ASC
	`
	rows, err := db.Query(stmt, userID)
	if err != nil {
		return nil, fmt.Errorf("list variables: %w", err)
	}
	defer rows.Close()

	out := make([]Variable, 0)
	for rows.Next() {
		var v Variable
		if err := rows.Scan(&v.ID, &v.UserID, &v.Name, &v.Value, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan variable: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate variables: %w", err)
	}
	return out, nil
}

func upsertVariable(db *sql.DB, userID, name, value string) (string, error) {
	const stmt = `
		INSERT INTO prompt_optimizer_variables (user_id, name, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, name) DO UPDATE
		SET value = EXCLUDED.value, updated_at = NOW()
		RETURNING id
	`
	var id string
	if err := db.QueryRow(stmt, userID, name, value).Scan(&id); err != nil {
		return "", fmt.Errorf("upsert variable: %w", err)
	}
	return id, nil
}

func deleteVariableForUser(db *sql.DB, userID, id string) error {
	const stmt = `DELETE FROM prompt_optimizer_variables WHERE id = $1 AND user_id = $2`
	res, err := db.Exec(stmt, id, userID)
	if err != nil {
		return fmt.Errorf("delete variable: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errVariableNotFound
	}
	return nil
}

// varsByName returns the user's variables indexed by name, used by the
// optimize handler for placeholder substitution.
func varsByName(db *sql.DB, userID string) (map[string]string, error) {
	list, err := listVariablesForUser(db, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(list))
	for _, v := range list {
		out[v.Name] = v.Value
	}
	return out, nil
}

func ListVariables(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_500")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_501")
	defer rc.Close()
	logger := rc.GetLogger()

	vars, err := listVariablesForUser(ApiTypes.ProjectDBHandle, userID)
	if err != nil {
		logger.Error("list variables failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to list variables (CWB_POH_502)")
	}
	return c.JSON(http.StatusOK, VariableListResponse{Status: true, Variables: vars})
}

func UpsertVariable(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_510")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_511")
	defer rc.Close()
	logger := rc.GetLogger()

	var req UpsertVariableRequest
	if err := c.Bind(&req); err != nil {
		return jsonError(c, http.StatusBadRequest, "invalid request body (CWB_POH_512)")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return jsonError(c, http.StatusBadRequest, "name is required (CWB_POH_513)")
	}

	id, err := upsertVariable(ApiTypes.ProjectDBHandle, userID, req.Name, req.Value)
	if err != nil {
		logger.Error("upsert variable failed", "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to save variable (CWB_POH_514)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "id": id})
}

func DeleteVariable(c echo.Context) error {
	userID, ok := requireUser(c, "CWB_POH_520")
	if !ok {
		return nil
	}
	rc := EchoFactory.NewFromEcho(c, "CWB_POH_521")
	defer rc.Close()
	logger := rc.GetLogger()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return jsonError(c, http.StatusBadRequest, "missing id (CWB_POH_522)")
	}
	err := deleteVariableForUser(ApiTypes.ProjectDBHandle, userID, id)
	if errors.Is(err, errVariableNotFound) {
		return jsonError(c, http.StatusNotFound, "variable not found (CWB_POH_523)")
	}
	if err != nil {
		logger.Error("delete variable failed", "id", id, "err", err)
		return jsonError(c, http.StatusInternalServerError, "failed to delete variable (CWB_POH_524)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
