package wpi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

var handlerLog = loggerutil.CreateDefaultLogger("20261026-418")

// maxBodyBytes bounds descriptor and action request bodies.
const maxBodyBytes = 1 << 20

// Handler serves the WPI endpoints. User returns the logged-in user, or nil.
type Handler struct {
	DB   *sql.DB
	User func(c echo.Context) *ApiTypes.UserInfo
}

// New returns a Handler that reads the user from the request session.
func New(db *sql.DB) *Handler {
	return &Handler{DB: db, User: sessionUser}
}

func sessionUser(c echo.Context) *ApiTypes.UserInfo {
	rc := EchoFactory.NewFromEcho(c, "20261026-418")
	defer rc.Close()
	return rc.IsAuthenticated()
}

// isAdmin mirrors the admin rule of pageconfighandler.
func isAdmin(u *ApiTypes.UserInfo) bool {
	return u.Admin || u.IsOwner || slices.Contains(u.Roles, "admin")
}

func actorName(u *ApiTypes.UserInfo) string {
	if u.Email != "" {
		return u.Email
	}
	return u.UserName
}

func errorJSON(c echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]string{"error": msg})
}

// GetApp returns an app with every descriptor reachable from it.
// Endpoint: GET /api/v1/wpi/apps/:appId
func (h *Handler) GetApp(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	appID := c.Param("appId")
	if !idPattern.MatchString(appID) {
		return errorJSON(c, http.StatusNotFound, "App not found")
	}
	st := store{h.DB}
	res, err := BuildBundle(c.Request().Context(), appID, st.latest, catalogColumns(h.DB), st.registered)
	if errors.Is(err, ErrNotFound) {
		return errorJSON(c, http.StatusNotFound, "App not found")
	}
	if err != nil {
		if errors.Is(c.Request().Context().Err(), context.Canceled) {
			handlerLog.Info("WPI app bundle request canceled", "app", appID, "user", actorName(user))
			return nil
		}
		handlerLog.Error("WPI app bundle failed", "app", appID, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to load app")
	}
	if len(res.Errors) > 0 {
		handlerLog.Warn("WPI app has invalid descriptors", "app", appID, "errors", len(res.Errors))
		return c.JSON(http.StatusUnprocessableEntity, map[string]any{"errors": res.Errors, "warnings": nonNil(res.Warnings)})
	}
	handlerLog.Info("WPI app bundle served", "app", appID, "user", actorName(user), "pages", len(res.Bundle.Pages), "warnings", len(res.Warnings))
	return c.JSON(http.StatusOK, res.Bundle)
}

func nonNil(issues []Issue) []Issue {
	if issues == nil {
		return []Issue{}
	}
	return issues
}

// PutDescriptor validates a descriptor and stores it as a new active version.
// Endpoint: PUT /api/v1/wpi/descriptors/:kind/:descriptorId (admins only)
func (h *Handler) PutDescriptor(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(user) {
		return errorJSON(c, http.StatusForbidden, "Admin access required")
	}
	kind, id := c.Param("kind"), c.Param("descriptorId")
	if !validKind(kind) {
		return errorJSON(c, http.StatusBadRequest, "Unknown descriptor kind")
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes))
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Unable to read request body")
	}
	body, err := decodeBody(raw)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Body must be a JSON object")
	}
	if issues := ValidateDescriptor(Kind(kind), id, body); len(issues) > 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"errors": issues})
	}
	st := store{h.DB}
	ctx := c.Request().Context()
	if Kind(kind) == KindElement {
		if refused, err := checkModule(c, st, id, body, user); refused {
			return err
		}
	}
	// A page's instances are checked against the active class rows, since
	// which attributes an element instance may set depends on its class
	// (OpenSpec wpi-code-modules, design D5).
	if Kind(kind) == KindPage {
		classes, err := loadClasses(ctx, st.latest)
		if err != nil {
			handlerLog.Error("WPI class load failed", "kind", kind, "id", id, "user", actorName(user), "error", err)
			return errorJSON(c, http.StatusInternalServerError, "Unable to check descriptor")
		}
		if issues := ValidatePageInstances(id, body, classes); len(issues) > 0 {
			return c.JSON(http.StatusBadRequest, map[string]any{"errors": issues})
		}
	}
	version, err := st.insertVersion(ctx, Kind(kind), id, raw, actorName(user))
	if err != nil {
		handlerLog.Error("WPI descriptor insert failed", "kind", kind, "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store descriptor")
	}
	var warnings []Issue
	if Kind(kind) == KindElement {
		warnings = classWarnings(id, body)
	}
	handlerLog.Info("WPI descriptor stored", "kind", kind, "id", id, "version", version, "user", actorName(user), "warnings", len(warnings))
	return c.JSON(http.StatusOK, map[string]any{"kind": kind, "descriptor_id": id, "version": version, "warnings": nonNil(warnings)})
}

// CreateElementClass stores a new element class as a draft: version 1, before
// its code exists. A class ID that already has a row is a conflict (OpenSpec
// wpi-create-element-class). The class becomes active through PutDescriptor
// once its code is registered.
// Endpoint: POST /api/v1/wpi/classes/element (admins only)
func (h *Handler) CreateElementClass(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(user) {
		return errorJSON(c, http.StatusForbidden, "Admin access required")
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes))
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Unable to read request body")
	}
	body, err := decodeBody(raw)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Body must be a JSON object")
	}
	if issues := ValidateDraftClass(body); len(issues) > 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"errors": issues})
	}
	id := body["descriptor_id"].(string)
	st := store{h.DB}
	if refused, err := checkModule(c, st, id, body, user); refused {
		return err
	}
	err = st.insertDraftClass(c.Request().Context(), id, raw, actorName(user))
	if errors.Is(err, errClassExists) {
		handlerLog.Info("WPI element class already exists", "id", id, "user", actorName(user))
		return errorJSON(c, http.StatusConflict, fmt.Sprintf("element class %q already exists", id))
	}
	if err != nil {
		handlerLog.Error("WPI element class insert failed", "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store element class")
	}
	warnings := classWarnings(id, body)
	handlerLog.Info("WPI element class drafted", "id", id, "user", actorName(user), "warnings", len(warnings))
	return c.JSON(http.StatusCreated, map[string]any{"descriptor_id": id, "version": 1, "status": "draft", "warnings": nonNil(warnings)})
}

// ListElementClasses returns the latest draft or active version of every
// element class, for the editor's palette.
// Endpoint: GET /api/v1/wpi/classes/element
func (h *Handler) ListElementClasses(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	classes, err := store{h.DB}.elementClasses(c.Request().Context())
	if err != nil {
		handlerLog.Error("WPI element class list failed", "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to list element classes")
	}
	handlerLog.Info("WPI element classes listed", "user", actorName(user), "found", len(classes))
	return c.JSON(http.StatusOK, map[string]any{"classes": classes})
}

// GetElementClass returns the latest draft or active version of an element
// class, for the editor's edit form.
// Endpoint: GET /api/v1/wpi/classes/element/:classId
func (h *Handler) GetElementClass(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	id := c.Param("classId")
	if !idPattern.MatchString(id) {
		return errorJSON(c, http.StatusNotFound, "Element class not found")
	}
	row, found, err := store{h.DB}.elementClass(c.Request().Context(), id)
	if err != nil {
		handlerLog.Error("WPI element class read failed", "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to read element class")
	}
	if !found {
		return errorJSON(c, http.StatusNotFound, "Element class not found")
	}
	handlerLog.Info("WPI element class read", "id", id, "version", row.Version, "status", row.Status, "user", actorName(user))
	return c.JSON(http.StatusOK, row)
}

// UpdateElementClass modifies the latest draft or active element class row in
// place, preserving its version and status. Authoring validates its structure, allowing
// attributes to change independently of interpreter code; an active class must
// still name a registered code module, and attributes that module does not
// support are returned as warnings. The class ID cannot change (OpenSpec
// wpi-create-element-class, wpi-code-modules).
// Endpoint: PUT /api/v1/wpi/classes/element/:classId (admins only)
func (h *Handler) UpdateElementClass(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	if !isAdmin(user) {
		return errorJSON(c, http.StatusForbidden, "Admin access required")
	}
	id := c.Param("classId")
	if !idPattern.MatchString(id) {
		return errorJSON(c, http.StatusNotFound, "Element class not found")
	}
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, maxBodyBytes))
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Unable to read request body")
	}
	body, err := decodeBody(raw)
	if err != nil {
		return errorJSON(c, http.StatusBadRequest, "Body must be a JSON object")
	}
	if got, _ := body["descriptor_id"].(string); got != id {
		return errorJSON(c, http.StatusBadRequest, fmt.Sprintf("descriptor_id %q does not match %q", got, id))
	}
	st := store{h.DB}
	ctx := c.Request().Context()
	cur, found, err := st.elementClass(ctx, id)
	if err != nil {
		handlerLog.Error("WPI element class read failed", "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to read element class")
	}
	if !found {
		return errorJSON(c, http.StatusNotFound, "Element class not found")
	}
	issues := validateElementClass(body, cur.Status == "draft")
	if len(issues) > 0 {
		return c.JSON(http.StatusBadRequest, map[string]any{"errors": issues})
	}
	if refused, err := checkModule(c, st, id, body, user); refused {
		return err
	}
	version, err := st.updateClass(ctx, id, cur, raw, actorName(user))
	if errors.Is(err, sql.ErrNoRows) {
		handlerLog.Warn("WPI element class missing during update", "id", id, "version", cur.Version, "user", actorName(user))
		return errorJSON(c, http.StatusNotFound, "Element class not found")
	}
	if err != nil {
		handlerLog.Error("WPI element class update failed", "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store element class")
	}
	warnings := classWarnings(id, body)
	handlerLog.Info("WPI element class updated", "id", id, "version", version, "status", cur.Status, "user", actorName(user), "warnings", len(warnings))
	return c.JSON(http.StatusOK, map[string]any{"descriptor_id": id, "version": version, "status": cur.Status, "warnings": nonNil(warnings)})
}

// checkModule refuses an element class row whose code_module_id does not name
// an active element module in kb.code_modules (OpenSpec wpi-code-modules, design
// D9). When refused is true, the response has been written and err is its error.
func checkModule(c echo.Context, st store, id string, body map[string]any, user *ApiTypes.UserInfo) (refused bool, err error) {
	issues, err := moduleErrors(c.Request().Context(), st.registered, map[string]map[string]any{id: body})
	if err != nil {
		handlerLog.Error("WPI code module check failed", "id", id, "user", actorName(user), "error", err)
		return true, errorJSON(c, http.StatusInternalServerError, "Unable to check code module")
	}
	if len(issues) > 0 {
		return true, c.JSON(http.StatusBadRequest, map[string]any{"errors": issues})
	}
	return false, nil
}

// ListCodeModules returns every code module in kb.code_modules, for the
// editor's class form (devdoc 2026102601 §3.5.2).
// Endpoint: GET /api/v1/wpi/code-modules
func (h *Handler) ListCodeModules(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	modules, err := store{h.DB}.listCodeModules(c.Request().Context())
	if err != nil {
		handlerLog.Error("WPI code module list failed", "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to list code modules")
	}
	handlerLog.Info("WPI code modules listed", "user", actorName(user), "found", len(modules))
	return c.JSON(http.StatusOK, map[string]any{"code_modules": modules})
}

// Limits of a page search (design D10).
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// SearchPages lists the latest active version of every page whose ID, name or
// description contains q, ignoring case, so an author can find one to reuse
// (devdoc 2026102601 §3.2.3).
// Endpoint: GET /api/v1/wpi/pages?q=&descriptor_id=&description=&limit=&offset=
// created_from/updated_from are inclusive; created_to/updated_to are exclusive RFC3339 timestamps.
func (h *Handler) SearchPages(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	limit := defaultPageLimit
	if s := c.QueryParam("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return errorJSON(c, http.StatusBadRequest, "limit must be a positive integer")
		}
		limit = min(n, maxPageLimit)
	}
	f := pageSearch{Limit: limit, Query: strings.ToLower(strings.TrimSpace(c.QueryParam("q"))), ID: strings.ToLower(strings.TrimSpace(c.QueryParam("descriptor_id"))), Description: strings.ToLower(strings.TrimSpace(c.QueryParam("description")))}
	if raw := c.QueryParam("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return errorJSON(c, http.StatusBadRequest, "offset must be a non-negative integer")
		}
		f.Offset = n
	}
	for key, dest := range map[string]**time.Time{"created_from": &f.CreatedFrom, "created_to": &f.CreatedTo, "updated_from": &f.UpdatedFrom, "updated_to": &f.UpdatedTo} {
		if raw := c.QueryParam(key); raw != "" {
			at, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return errorJSON(c, http.StatusBadRequest, key+" must be an RFC3339 timestamp")
			}
			*dest = &at
		}
	}
	for _, bounds := range [][2]*time.Time{{f.CreatedFrom, f.CreatedTo}, {f.UpdatedFrom, f.UpdatedTo}} {
		if bounds[0] != nil && bounds[1] != nil && !bounds[0].Before(*bounds[1]) {
			return errorJSON(c, http.StatusBadRequest, "date range start must precede its end")
		}
	}
	pages, err := store{h.DB}.searchPages(c.Request().Context(), f)
	if err != nil {
		handlerLog.Error("WPI page search failed", "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to search pages")
	}
	hasMore := len(pages) > limit
	if hasMore {
		pages = pages[:limit]
	}
	handlerLog.Info("WPI pages searched", "user", actorName(user), "q", f.Query, "offset", f.Offset, "found", len(pages))
	return c.JSON(http.StatusOK, map[string]any{"pages": pages, "has_more": hasMore})
}

// GetPage returns the latest active version of a page, so an author can copy
// it and save the copy under a new ID.
// Endpoint: GET /api/v1/wpi/pages/:pageId
func (h *Handler) GetPage(c echo.Context) error {
	user := h.User(c)
	if user == nil {
		return errorJSON(c, http.StatusUnauthorized, "Authentication required")
	}
	id := c.Param("pageId")
	if !idPattern.MatchString(id) {
		return errorJSON(c, http.StatusNotFound, "Page not found")
	}
	found, err := store{h.DB}.latest(c.Request().Context(), KindPage, []string{id})
	if err != nil {
		handlerLog.Error("WPI page read failed", "page", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to read page")
	}
	d, ok := found[id]
	if !ok {
		return errorJSON(c, http.StatusNotFound, "Page not found")
	}
	handlerLog.Info("WPI page read", "page", id, "version", d.Version, "user", actorName(user))
	return c.JSON(http.StatusOK, map[string]any{"version": d.Version, "page": d.Raw})
}
