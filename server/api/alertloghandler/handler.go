// Package alertloghandler serves the System Admin > Logs > Warnings and Errors
// page: it lists public.jimo_alert_log (JimoLogger Warn/Error events) and lets
// users assign an event to an owner or resolve it.
package alertloghandler

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

type alertRow struct {
	ID          int64   `json:"id"`
	Level       string  `json:"level"`
	Message     string  `json:"message"`
	ReqID       string  `json:"req_id"`
	Loc         string  `json:"loc"`
	CallFlow    string  `json:"call_flow"`
	Attrs       string  `json:"attrs"`
	CreatedAt   string  `json:"created_at"`
	Status      string  `json:"status"`
	Owner       *string `json:"owner"`
	Resolution  *string `json:"resolution"`
	Resolver    *string `json:"resolver"`
	ResolveTime *string `json:"resolve_time"`
	Comments    int     `json:"comment_count"`
}

type commentRow struct {
	ID        int64  `json:"id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type commentRequest struct {
	Body string `json:"body"`
}

// Columns the list may be sorted or filtered by, and the SQL operators allowed.
var sortableColumns = map[string]string{
	"created_at": "l.created_at", "level": "l.level", "message": "l.message",
	"loc": "l.loc", "status": "l.status", "owner": "l.owner",
}

var filterOps = map[string]string{"eq": "=", "ne": "<>", "gt": ">", "lt": "<"}

type listResponse struct {
	Status   bool       `json:"status"`
	Results  []alertRow `json:"results"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
	Total    int64      `json:"total"`
}

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

type assignRequest struct {
	Owner *string `json:"owner"` // null or blank clears the owner
}

type resolveRequest struct {
	Resolution string `json:"resolution"`
}

func fail(c echo.Context, code int, msg string) error {
	return c.JSON(code, errorResponse{Status: false, ErrorMsg: msg})
}

// List handles GET /api/v1/system-admin/alert-logs, newest first by default.
// Optional query params: page, page_size, sort, filter (repeatable).
func List(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_ALT_001")
	defer rc.Close()
	logger := rc.GetLogger()
	if rc.IsAuthenticated() == nil {
		return fail(c, http.StatusUnauthorized, "authentication required (CWB_ALT_002)")
	}

	page, pageSize := 1, defaultPageSize
	if raw := strings.TrimSpace(c.QueryParam("page")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return fail(c, http.StatusBadRequest, "query param 'page' must be a positive integer (CWB_ALT_003)")
		}
		page = v
	}
	if raw := strings.TrimSpace(c.QueryParam("page_size")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return fail(c, http.StatusBadRequest, "query param 'page_size' must be a positive integer (CWB_ALT_004)")
		}
		pageSize = min(v, maxPageSize)
	}

	// filter=<column>|<op>|<value> (repeatable), op in eq|ne|gt|lt
	where := []string{"1=1"}
	args := []any{}
	for _, raw := range c.QueryParams()["filter"] {
		parts := strings.SplitN(raw, "|", 3)
		col, ok := sortableColumns[parts[0]]
		if len(parts) != 3 || !ok || filterOps[parts[1]] == "" {
			return fail(c, http.StatusBadRequest, "invalid filter '"+raw+"' (CWB_ALT_030)")
		}
		args = append(args, parts[2])
		ph := "$" + strconv.Itoa(len(args))
		if parts[0] == "created_at" {
			ph += "::timestamptz"
		}
		where = append(where, col+" "+filterOps[parts[1]]+" "+ph)
	}
	whereSQL := strings.Join(where, " AND ")

	// sort=<column>|<asc|desc>; default newest first.
	orderSQL := "l.created_at DESC, l.id DESC"
	if raw := strings.TrimSpace(c.QueryParam("sort")); raw != "" {
		parts := strings.SplitN(raw, "|", 2)
		col, ok := sortableColumns[parts[0]]
		dir := strings.ToUpper(parts[len(parts)-1])
		if !ok || len(parts) != 2 || (dir != "ASC" && dir != "DESC") {
			return fail(c, http.StatusBadRequest, "invalid sort '"+raw+"' (CWB_ALT_031)")
		}
		orderSQL = col + " " + dir + ", l.id DESC"
	}

	db := ApiTypes.SharedDBHandle
	if db == nil {
		return fail(c, http.StatusInternalServerError, "database not initialised (CWB_ALT_005)")
	}
	var total int64
	if err := db.QueryRowContext(c.Request().Context(),
		"SELECT COUNT(*) FROM public.jimo_alert_log l WHERE "+whereSQL, args...).Scan(&total); err != nil {
		logger.Error("count alert logs failed", "error", err)
		return fail(c, http.StatusInternalServerError, "failed to list alert logs (CWB_ALT_006)")
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := db.QueryContext(c.Request().Context(),
		`SELECT id, level, message, COALESCE(req_id,''), COALESCE(loc,''), COALESCE(call_flow,''),
		        COALESCE(attrs::text,'{}'), created_at, status, owner, resolution, resolver, resolve_time,
		        (SELECT COUNT(*) FROM public.jimo_alert_comments k WHERE k.alert_id = l.id)
		   FROM public.jimo_alert_log l WHERE `+whereSQL+
			` ORDER BY `+orderSQL+` LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		logger.Error("list alert logs failed", "error", err)
		return fail(c, http.StatusInternalServerError, "failed to list alert logs (CWB_ALT_007)")
	}
	defer rows.Close()

	results := make([]alertRow, 0, pageSize)
	for rows.Next() {
		var r alertRow
		var created time.Time
		var resolveTime sql.NullTime
		if err := rows.Scan(&r.ID, &r.Level, &r.Message, &r.ReqID, &r.Loc, &r.CallFlow, &r.Attrs,
			&created, &r.Status, &r.Owner, &r.Resolution, &r.Resolver, &resolveTime, &r.Comments); err != nil {
			logger.Error("scan alert log failed", "error", err)
			return fail(c, http.StatusInternalServerError, "failed to list alert logs (CWB_ALT_008)")
		}
		r.CreatedAt = created.Format(time.RFC3339)
		if resolveTime.Valid {
			s := resolveTime.Time.Format(time.RFC3339)
			r.ResolveTime = &s
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate alert logs failed", "error", err)
		return fail(c, http.StatusInternalServerError, "failed to list alert logs (CWB_ALT_009)")
	}
	return c.JSON(http.StatusOK, listResponse{Status: true, Results: results, Page: page, PageSize: pageSize, Total: total})
}

// Assign handles PUT /api/v1/system-admin/alert-logs/:id/assign.
func Assign(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_ALT_010")
	defer rc.Close()
	logger := rc.GetLogger()
	if rc.IsAuthenticated() == nil {
		return fail(c, http.StatusUnauthorized, "authentication required (CWB_ALT_011)")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid id (CWB_ALT_012)")
	}
	var req assignRequest
	if err := c.Bind(&req); err != nil {
		return fail(c, http.StatusBadRequest, "invalid request body (CWB_ALT_013)")
	}
	var owner any // nil -> NULL
	if req.Owner != nil && strings.TrimSpace(*req.Owner) != "" {
		owner = strings.TrimSpace(*req.Owner)
	}
	return update(c, logger, id, "owner = $1", []any{owner}, "assign", "CWB_ALT_014")
}

// Resolve handles PUT /api/v1/system-admin/alert-logs/:id/resolve. The resolver
// is the authenticated user.
func Resolve(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_ALT_020")
	defer rc.Close()
	logger := rc.GetLogger()
	user := rc.IsAuthenticated()
	if user == nil {
		return fail(c, http.StatusUnauthorized, "authentication required (CWB_ALT_021)")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid id (CWB_ALT_022)")
	}
	var req resolveRequest
	if err := c.Bind(&req); err != nil {
		return fail(c, http.StatusBadRequest, "invalid request body (CWB_ALT_023)")
	}
	resolution := strings.TrimSpace(req.Resolution)
	if resolution == "" {
		return fail(c, http.StatusBadRequest, "resolution is required (CWB_ALT_024)")
	}
	resolver := strings.TrimSpace(user.Email)
	if resolver == "" {
		resolver = strings.TrimSpace(user.UserName)
	}
	return update(c, logger, id,
		"status = 'resolved', resolution = $1, resolver = $2, resolve_time = NOW()",
		[]any{resolution, resolver}, "resolve", "CWB_ALT_025")
}

func update(c echo.Context, logger ApiTypes.JimoLogger, id int64, set string, args []any, op, loc string) error {
	db := ApiTypes.SharedDBHandle
	if db == nil {
		return fail(c, http.StatusInternalServerError, "database not initialised ("+loc+")")
	}
	args = append(args, id)
	res, err := db.ExecContext(c.Request().Context(),
		"UPDATE public.jimo_alert_log SET "+set+" WHERE id = $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		logger.Error("alert log update failed", "op", op, "id", id, "error", err)
		return fail(c, http.StatusInternalServerError, "failed to "+op+" alert log ("+loc+")")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(c, http.StatusNotFound, "alert log not found ("+loc+")")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// ListComments handles GET /api/v1/system-admin/alert-logs/:id/comments, oldest first.
func ListComments(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_ALT_040")
	defer rc.Close()
	logger := rc.GetLogger()
	if rc.IsAuthenticated() == nil {
		return fail(c, http.StatusUnauthorized, "authentication required (CWB_ALT_041)")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid id (CWB_ALT_042)")
	}
	db := ApiTypes.SharedDBHandle
	if db == nil {
		return fail(c, http.StatusInternalServerError, "database not initialised (CWB_ALT_043)")
	}
	rows, err := db.QueryContext(c.Request().Context(),
		`SELECT id, author, body, created_at FROM public.jimo_alert_comments
		  WHERE alert_id = $1 ORDER BY created_at, id`, id)
	if err != nil {
		logger.Error("list alert comments failed", "id", id, "error", err)
		return fail(c, http.StatusInternalServerError, "failed to list comments (CWB_ALT_044)")
	}
	defer rows.Close()
	results := []commentRow{}
	for rows.Next() {
		var r commentRow
		var created time.Time
		if err := rows.Scan(&r.ID, &r.Author, &r.Body, &created); err != nil {
			logger.Error("scan alert comment failed", "error", err)
			return fail(c, http.StatusInternalServerError, "failed to list comments (CWB_ALT_045)")
		}
		r.CreatedAt = created.Format(time.RFC3339)
		results = append(results, r)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": results})
}

// AddComment handles POST /api/v1/system-admin/alert-logs/:id/comments. The
// author is the authenticated user.
func AddComment(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_ALT_050")
	defer rc.Close()
	logger := rc.GetLogger()
	user := rc.IsAuthenticated()
	if user == nil {
		return fail(c, http.StatusUnauthorized, "authentication required (CWB_ALT_051)")
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return fail(c, http.StatusBadRequest, "invalid id (CWB_ALT_052)")
	}
	var req commentRequest
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Body) == "" {
		return fail(c, http.StatusBadRequest, "comment text is required (CWB_ALT_053)")
	}
	author := strings.TrimSpace(user.Email)
	if author == "" {
		author = strings.TrimSpace(user.UserName)
	}
	db := ApiTypes.SharedDBHandle
	if db == nil {
		return fail(c, http.StatusInternalServerError, "database not initialised (CWB_ALT_054)")
	}
	res, err := db.ExecContext(c.Request().Context(),
		`INSERT INTO public.jimo_alert_comments (alert_id, author, body)
		 SELECT id, $2, $3 FROM public.jimo_alert_log WHERE id = $1`,
		id, author, strings.TrimSpace(req.Body))
	if err != nil {
		logger.Error("add alert comment failed", "id", id, "error", err)
		return fail(c, http.StatusInternalServerError, "failed to add comment (CWB_ALT_055)")
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fail(c, http.StatusNotFound, "alert log not found (CWB_ALT_056)")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
