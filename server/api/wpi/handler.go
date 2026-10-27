package wpi

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	version, err := store{h.DB}.insertVersion(c.Request().Context(), Kind(kind), id, raw, actorName(user))
	if err != nil {
		handlerLog.Error("WPI descriptor insert failed", "kind", kind, "id", id, "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to store descriptor")
	}
	handlerLog.Info("WPI descriptor stored", "kind", kind, "id", id, "version", version, "user", actorName(user))
	return c.JSON(http.StatusOK, map[string]any{"kind": kind, "descriptor_id": id, "version": version})
}

// Limits of a page search (design D10).
const (
	defaultPageLimit = 50
	maxPageLimit     = 200
)

// SearchPages lists the latest active version of every page whose ID, name or
// description contains q, ignoring case, so an author can find one to reuse
// (devdoc 2026102601 §3.2.3).
// Endpoint: GET /api/v1/wpi/pages?q=&limit=
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
	all, err := store{h.DB}.pageSummaries(c.Request().Context())
	if err != nil {
		handlerLog.Error("WPI page search failed", "user", actorName(user), "error", err)
		return errorJSON(c, http.StatusInternalServerError, "Unable to search pages")
	}
	q := strings.ToLower(strings.TrimSpace(c.QueryParam("q")))
	pages := []PageSummary{}
	for _, p := range all {
		if len(pages) < limit && (q == "" || p.matches(q)) {
			pages = append(pages, p)
		}
	}
	handlerLog.Info("WPI pages searched", "user", actorName(user), "q", q, "found", len(pages))
	return c.JSON(http.StatusOK, map[string]any{"pages": pages})
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
