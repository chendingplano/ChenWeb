package wpi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

// ActionRef names an action instance by its place in a stored page:
// {kind: "page", id, pointer} (design D5).
type ActionRef struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Pointer string `json:"pointer"`
}

func (r ActionRef) String() string {
	if r.Pointer == "" {
		return r.Kind + ":" + r.ID
	}
	return r.Kind + ":" + r.ID + "#" + r.Pointer
}

type runRequest struct {
	Ref      ActionRef      `json:"ref"`
	Args     map[string]any `json:"args"`
	InputDoc map[string]any `json:"input_doc"`
}

// requestError is a problem with the request or the descriptor it names; its
// message is safe to show.
type requestError struct {
	status int
	msg    string
}

func (e *requestError) Error() string { return e.msg }

func badRequest(format string, a ...any) error {
	return &requestError{http.StatusBadRequest, fmt.Sprintf(format, a...)}
}

// RunAction runs a load, save, query or load-file instance written in a
// stored page.
// Endpoint: POST /api/v1/wpi/actions/run
func (h *Handler) RunAction(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	start := time.Now()
	var req runRequest
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes))
	if err == nil {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		err = d.Decode(&req)
	}
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Body must be a JSON object")
	}
	ctx := c.Request().Context()
	act, node, err := h.resolveAction(ctx, user, req.Ref)
	var rows int
	var result any
	if err == nil {
		result, rows, err = h.run(ctx, act, node, req, user)
	}
	logArgs := []any{"ref", req.Ref.String(), "user", actorName(user), "table", act.Table, "stored", act.SQLStmtID + act.FileID, "action", act.Class,
		"mode", act.Mode + act.Result, "rows", rows, "duration_ms", time.Since(start).Milliseconds()}
	var reqErr *requestError
	var fieldErrs fieldErrors
	switch {
	case errors.As(err, &fieldErrs):
		handlerLog.Info("WPI save rejected by field rules", append(logArgs, "field_errors", len(fieldErrs))...)
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"field_errors": fieldErrs})
	case errors.As(err, &reqErr):
		handlerLog.Warn("WPI action refused", append(logArgs, "error", err)...)
		return errorJSON(c, reqErr.status, reqErr.msg)
	case err != nil:
		handlerLog.Error("WPI action failed", append(logArgs, "error", err)...)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && (pqErr.Code.Class() == "22" || pqErr.Code.Class() == "23") {
			return errorJSON(c, http.StatusBadRequest, "The database rejected the values")
		}
		return errorJSON(c, http.StatusInternalServerError, "Unable to run action")
	}
	handlerLog.Info("WPI action run", logArgs...)
	return c.JSON(http.StatusOK, result)
}

type fieldErrors map[string][]string

func (f fieldErrors) Error() string { return "field rules failed" }

// resolveAction finds the referenced action instance in its stored page,
// checks it against its class row and returns it merged with the class.
func (h *Handler) resolveAction(ctx context.Context, user *ApiTypes.UserInfo, ref ActionRef) (Action, map[string]any, error) {
	var act Action
	if ref.Kind != string(KindPage) {
		return act, nil, badRequest("ref.kind must be page")
	}
	if !idPattern.MatchString(ref.ID) {
		return act, nil, badRequest("ref.id is not a valid descriptor ID")
	}
	if ref.Pointer == "" {
		return act, nil, badRequest("ref.pointer is required")
	}
	st := store{h.DB}
	found, err := st.latest(ctx, KindPage, []string{ref.ID})
	if err != nil {
		return act, nil, err
	}
	d, ok := found[ref.ID]
	if !ok {
		return act, nil, &requestError{http.StatusNotFound, fmt.Sprintf("page %q does not exist", ref.ID)}
	}
	if !mayAccessPage(user, d.Body) {
		return act, nil, &requestError{http.StatusForbidden, fmt.Sprintf("You may not use page %q", ref.ID)}
	}
	positions := actionPositions(d.Body)
	i := slices.IndexFunc(positions, func(p actionPos) bool { return p.path == ref.Pointer })
	if i < 0 {
		return act, nil, badRequest("pointer %q does not point at an action instance", ref.Pointer)
	}
	pos := positions[i]
	if pos.isStatic() {
		return act, nil, badRequest("pointer %q points at static data, not an action instance", ref.Pointer)
	}
	m, _ := pos.val.(map[string]any)
	cls := str(m["action"])
	if !slices.Contains(serverClasses, cls) {
		return act, nil, badRequest("only %s actions run on the server, not %q", strings.Join(serverClasses, ", "), cls)
	}
	rows, err := st.latest(ctx, KindAction, []string{cls})
	if err != nil {
		return act, nil, err
	}
	v := &validator{kind: KindPage, id: ref.ID, classes: map[Kind]map[string]Descriptor{KindAction: rows}}
	v.actionInstance(pos)
	var merged map[string]any
	if len(v.issues) == 0 {
		merged = v.actionComplete(pos)
	}
	if len(v.issues) > 0 {
		return act, nil, badRequest("the action instance is invalid: %s", v.issues[0].Message)
	}
	b, _ := json.Marshal(merged)
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&act); err != nil {
		return act, nil, err
	}
	act.Class = cls
	return act, merged, nil
}

// serverClasses are the action classes that run on the server.
var serverClasses = []string{"load", "save", "query", "load-file"}

func (h *Handler) run(ctx context.Context, act Action, node map[string]any, req runRequest, user *ApiTypes.UserInfo) (any, int, error) {
	switch act.Class {
	case "query":
		return h.query(ctx, act, req.Args, user)
	case "load-file":
		return h.loadFile(ctx, act, user)
	}
	db, table, err := h.tableDatabase(act.Table)
	if err != nil {
		return nil, 0, badRequest("%v", err)
	}
	cols, err := catalogColumns(db)(ctx, table)
	if err != nil {
		return nil, 0, err
	}
	if cols == nil {
		return nil, 0, badRequest("table %q does not exist", act.Table)
	}
	for _, col := range actionColumns(node) {
		if col.name != "" && !cols[col.name] {
			return nil, 0, badRequest("column %q does not exist in table %q", col.name, act.Table)
		}
	}
	act.Table = table
	target := *h
	target.DB = db
	if act.Class == "load" {
		return target.load(ctx, act, req.Args)
	}
	return target.save(ctx, act, req.InputDoc)
}

func (h *Handler) load(ctx context.Context, act Action, args map[string]any) (any, int, error) {
	where := map[string]any{}
	for col, v := range act.Where {
		val, err := substituteArgs(v, args)
		if err != nil {
			return nil, 0, badRequest("%v", err)
		}
		if _, isObj := val.(map[string]any); isObj {
			return nil, 0, badRequest("argument for %q must be a string, number, boolean or null", col)
		}
		if _, isArr := val.([]any); isArr {
			return nil, 0, badRequest("argument for %q must be a string, number, boolean or null", col)
		}
		where[col] = val
	}
	q := buildLoad(act, where)
	rows, err := h.DB.QueryContext(ctx, q.sql, q.args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	records := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(act.Fields))
		ptrs := make([]any, len(vals))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, 0, err
		}
		rec := make(map[string]any, len(vals))
		for i, f := range act.Fields {
			rec[f.Attribute] = jsonValue(vals[i])
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if act.Result == "record" {
		if len(records) == 0 {
			return nil, 0, &requestError{http.StatusNotFound, "Record not found"}
		}
		return map[string]any{"result": records[0]}, 1, nil
	}
	return map[string]any{"result": records}, len(records), nil
}

// queryTimeout bounds one stored statement (design D6).
const queryTimeout = "10s"

// query runs a stored SQL statement read-only, binding params to $1…$n, and
// returns its rows as JSON objects keyed by column name (design D6).
func (h *Handler) query(ctx context.Context, act Action, args map[string]any, user *ApiTypes.UserInfo) (any, int, error) {
	stmt, err := h.storedFor(ctx, storedStatement, act.SQLStmtID, "SQL statement", user)
	if err != nil {
		return nil, 0, err
	}
	params := make([]any, len(act.Params))
	for i, p := range act.Params {
		val, err := substituteArgs(p, args)
		if err != nil {
			return nil, 0, badRequest("%v", err)
		}
		params[i] = paramValue(val)
	}
	limit := maxListLimit
	if act.Result == "record" {
		limit = 1
	}
	tx, err := h.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SET LOCAL statement_timeout = '"+queryTimeout+"'"); err != nil {
		return nil, 0, err
	}
	rows, err := tx.QueryContext(ctx, wrapStatement(stmt, limit), params...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	records := []json.RawMessage{}
	for rows.Next() {
		var rec string
		if err := rows.Scan(&rec); err != nil {
			return nil, 0, err
		}
		records = append(records, json.RawMessage(rec))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if act.Result == "record" {
		if len(records) == 0 {
			return nil, 0, &requestError{http.StatusNotFound, "Record not found"}
		}
		return map[string]any{"result": records[0]}, 1, nil
	}
	return map[string]any{"result": records}, len(records), nil
}

// wrapStatement turns each row of a stored statement into one JSON object, so
// PostgreSQL converts every column type. The newlines keep a trailing line
// comment in the statement from swallowing the wrapper.
func wrapStatement(stmt string, limit int) string {
	stmt = strings.TrimRight(strings.TrimSpace(stmt), "; \t\r\n")
	return "SELECT to_jsonb(t)::text FROM (\n" + stmt + "\n) t LIMIT " + strconv.Itoa(limit)
}

// loadFile returns a stored file's JSON content (design D7).
func (h *Handler) loadFile(ctx context.Context, act Action, user *ApiTypes.UserInfo) (any, int, error) {
	content, err := h.storedFor(ctx, storedFile, act.FileID, "file", user)
	if err != nil {
		return nil, 0, err
	}
	return map[string]any{"result": json.RawMessage(content)}, 1, nil
}

// storedFor returns a stored statement or file that the user may use.
func (h *Handler) storedFor(ctx context.Context, kind, name, what string, user *ApiTypes.UserInfo) (string, error) {
	content, access, found, err := store{h.DB}.stored(ctx, kind, name)
	if err != nil {
		return "", err
	}
	if !found {
		return "", badRequest("%s %q is not registered", what, name)
	}
	if !mayUse(user, access) {
		return "", &requestError{http.StatusForbidden, fmt.Sprintf("You may not use %s %q", what, name)}
	}
	return content, nil
}

// mayUse applies a stored row's access_controls: none, or "all", lets every
// logged-in user use it; otherwise the user needs one of the roles, and an
// admin has the role "admin" (design D8).
func mayUse(user *ApiTypes.UserInfo, access []string) bool {
	if len(access) == 0 || slices.Contains(access, "all") {
		return true
	}
	if isAdmin(user) && slices.Contains(access, "admin") {
		return true
	}
	return slices.ContainsFunc(user.Roles, func(r string) bool { return slices.Contains(access, r) })
}

// pageAccess returns a page body's access_controls; none means public.
func pageAccess(body map[string]any) []string {
	list, _ := body["access_controls"].([]any)
	roles := make([]string, 0, len(list))
	for _, r := range list {
		if s, ok := r.(string); ok {
			roles = append(roles, s)
		}
	}
	return roles
}

// mayAccessPage applies a page's access_controls with the rule of mayUse.
func mayAccessPage(user *ApiTypes.UserInfo, body map[string]any) bool {
	return mayUse(user, pageAccess(body))
}

func (h *Handler) save(ctx context.Context, act Action, doc map[string]any) (any, int, error) {
	if doc == nil {
		return nil, 0, badRequest("input_doc is required for a save action")
	}
	if errs := checkFields(act.Fields, doc); len(errs) > 0 {
		return nil, 0, fieldErrors(errs)
	}
	keyValue := doc[act.Key]
	insert := act.Mode == "insert" || (act.Mode == "upsert" && isEmpty(keyValue))
	var q query
	if insert {
		q = buildInsert(act, doc)
	} else {
		if isEmpty(keyValue) {
			return nil, 0, badRequest("input_doc has no value for key %q", act.Key)
		}
		var ok bool
		if q, ok = buildUpdate(act, doc, keyValue); !ok {
			return nil, 0, badRequest("input_doc has none of the attributes this action saves")
		}
	}
	var key any
	err := h.DB.QueryRowContext(ctx, q.sql, q.args...).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, &requestError{http.StatusNotFound, "Record not found"}
	}
	if err != nil {
		return nil, 0, err
	}
	return map[string]any{"key_attribute": act.Key, "key_value": jsonValue(key)}, 1, nil
}

// jsonValue converts a scanned database value into something JSON shows well.
func jsonValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case time.Time:
		return x.Format(time.RFC3339)
	}
	return v
}
