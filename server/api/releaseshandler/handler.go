package releaseshandler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
)

// RoleBuild is the [deployment] role of the machine that owns versions and items.
const RoleBuild = "build"

const codeReadOnly = "read_only"

// role is this machine's [deployment] role; a variable so tests can set it.
var role = func() string { return strings.TrimSpace(appconfig.AppConfig.Deployment.Role) }

var (
	pdfOnce     sync.Once
	pdfRenderer *PDFRenderer
)

// renderer is created on first use, after the environment has been loaded.
var renderer = func() *PDFRenderer {
	pdfOnce.Do(func() { pdfRenderer = NewPDFRenderer(".cache/release-notes") })
	return pdfRenderer
}

type errorResponse struct {
	Status   bool   `json:"status"`
	Code     string `json:"code,omitempty"`
	ErrorMsg string `json:"error_msg"`
}

func authorize(c echo.Context, loc string) (ApiTypes.RequestContext, *ApiTypes.UserInfo, bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, errorResponse{ErrorMsg: "authentication required"})
		return rc, nil, false
	}
	admin := user.Admin || user.IsOwner
	for _, r := range user.Roles {
		if r = strings.ToLower(strings.TrimSpace(r)); r == "admin" || r == "root" {
			admin = true
		}
	}
	if !admin {
		_ = c.JSON(http.StatusForbidden, errorResponse{ErrorMsg: "admin access required"})
		return rc, nil, false
	}
	return rc, user, true
}

// authorizeWrite also requires the build machine, which alone owns versions and items.
func authorizeWrite(c echo.Context, loc string) (ApiTypes.RequestContext, *ApiTypes.UserInfo, bool) {
	rc, user, ok := authorize(c, loc)
	if ok && role() != RoleBuild {
		_ = c.JSON(http.StatusForbidden, errorResponse{Code: codeReadOnly,
			ErrorMsg: "releases are changed only on the build machine ([deployment] role = \"build\")"})
		return rc, nil, false
	}
	return rc, user, ok
}

func userName(u *ApiTypes.UserInfo) string {
	if u == nil {
		return ""
	}
	if u.Email != "" {
		return u.Email
	}
	return u.UserName
}

func respondError(c echo.Context, rc ApiTypes.RequestContext, err error) error {
	status := http.StatusInternalServerError
	msg := "release operation failed"
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrLimit):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrNotFound):
		status, msg = http.StatusNotFound, err.Error()
	case errors.Is(err, ErrConflict):
		status, msg = http.StatusConflict, err.Error()
	case errors.Is(err, ErrPDFUnavailable):
		status, msg = http.StatusServiceUnavailable, err.Error()
	}
	if status == http.StatusInternalServerError {
		rc.GetLogger().Error("release operation failed", "error", err)
	} else {
		rc.GetLogger().Warn("release request refused", "status", status, "error", err)
	}
	return c.JSON(status, errorResponse{ErrorMsg: msg})
}

func projectParam(c echo.Context) (string, error) {
	p := strings.TrimSpace(c.QueryParam("project"))
	if p == "" {
		p = ProjectChenWeb
	}
	return p, validProject(p)
}

func parseID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("%w: id must be a positive number", ErrInvalid)
	}
	return id, nil
}

// ListReleases handles GET /releases?project=: current and next versions, the building
// release, pending items and the history.
func ListReleases(c echo.Context) error {
	rc, _, ok := authorize(c, "20261119-101")
	defer rc.Close()
	if !ok {
		return nil
	}
	project, err := projectParam(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle
	cur, build, err := Current(ctx, db, project)
	if err != nil {
		return respondError(c, rc, err)
	}
	history, err := History(ctx, db, project)
	if err != nil {
		return respondError(c, rc, err)
	}
	pending, err := Pending(ctx, db, project)
	if err != nil {
		return respondError(c, rc, err)
	}
	var building *Release
	released := make([]Release, 0, len(history))
	for i := range history {
		if history[i].Status == StatusBuilding {
			building = &history[i]
			continue
		}
		released = append(released, history[i])
	}
	next := ""
	if v, err := cur.NextRevision(); err == nil {
		next = v.String()
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status": true, "project": project, "role": role(), "editable": role() == RoleBuild,
		"current": cur.String(), "current_build_number": build, "next_build": next,
		"revisions_left": MaxPart - cur.Revision, "building": building,
		"pending": pending, "releases": released,
	})
}

type lineRequest struct {
	Project     string `json:"project"`
	Description string `json:"description"`
}

func createLine(c echo.Context, kind, loc string) error {
	rc, user, ok := authorizeWrite(c, loc)
	defer rc.Close()
	if !ok {
		return nil
	}
	var req lineRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return respondError(c, rc, fmt.Errorf("%w: invalid request body", ErrInvalid))
	}
	if req.Project == "" {
		req.Project = ProjectChenWeb
	}
	r, err := CreateLine(c.Request().Context(), ApiTypes.ProjectDBHandle, req.Project, kind, req.Description, userName(user))
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("release line created", "project", r.Project, "kind", kind, "version", r.Version, "user", userName(user))
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "release": r})
}

// CreateMajor handles POST /releases/majors {project, description}.
func CreateMajor(c echo.Context) error { return createLine(c, KindMajor, "20261119-102") }

// CreateMinor handles POST /releases/minors {project, description}.
func CreateMinor(c echo.Context) error { return createLine(c, KindMinor, "20261119-103") }

type itemRequest struct {
	Project string `json:"project"`
	Item
}

// CreateItem handles POST /releases/items.
func CreateItem(c echo.Context) error {
	rc, user, ok := authorizeWrite(c, "20261119-104")
	defer rc.Close()
	if !ok {
		return nil
	}
	var req itemRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return respondError(c, rc, fmt.Errorf("%w: invalid request body", ErrInvalid))
	}
	if req.Project == "" {
		req.Project = ProjectChenWeb
	}
	it, err := AddItem(c.Request().Context(), ApiTypes.ProjectDBHandle, req.Project, req.Item, userName(user))
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("release item added", "id", it.ID, "type", it.ItemType, "user", userName(user))
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "item": it})
}

// UpdateItemHandler handles PUT /releases/items/:id.
func UpdateItemHandler(c echo.Context) error {
	rc, user, ok := authorizeWrite(c, "20261119-105")
	defer rc.Close()
	if !ok {
		return nil
	}
	id, err := parseID(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	var req itemRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return respondError(c, rc, fmt.Errorf("%w: invalid request body", ErrInvalid))
	}
	it, err := UpdateItem(c.Request().Context(), ApiTypes.ProjectDBHandle, id, req.Item)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("release item updated", "id", id, "user", userName(user))
	return c.JSON(http.StatusOK, map[string]any{"status": true, "item": it})
}

// DeleteItemHandler handles DELETE /releases/items/:id.
func DeleteItemHandler(c echo.Context) error {
	rc, user, ok := authorizeWrite(c, "20261119-106")
	defer rc.Close()
	if !ok {
		return nil
	}
	id, err := parseID(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	if err := DeleteItem(c.Request().Context(), ApiTypes.ProjectDBHandle, id); err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("release item deleted", "id", id, "user", userName(user))
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// GetNotes handles GET /releases/notes?project=&from=&to=&format=text|pdf&lang=en|zh-cn.
// from is exclusive and may be empty (from the first release); to defaults to the newest
// released version.
func GetNotes(c echo.Context) error {
	rc, _, ok := authorize(c, "20261119-107")
	defer rc.Close()
	if !ok {
		return nil
	}
	project, err := projectParam(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	ctx := c.Request().Context()
	history, err := History(ctx, ApiTypes.ProjectDBHandle, project)
	if err != nil {
		return respondError(c, rc, err)
	}
	var from *Version
	if s := strings.TrimSpace(c.QueryParam("from")); s != "" {
		v, err := ParseVersion(s)
		if err != nil {
			return respondError(c, rc, err)
		}
		from = &v
	}
	var to Version
	if s := strings.TrimSpace(c.QueryParam("to")); s != "" {
		if to, err = ParseVersion(s); err != nil {
			return respondError(c, rc, err)
		}
	} else if released := InRange(history, nil, Version{MaxPart, MaxPart, MaxPart}); len(released) > 0 {
		to = released[0].V
	}
	lang := c.QueryParam("lang")
	if _, ok := labels[lang]; !ok {
		lang = "en"
	}
	notes := Render(project, InRange(history, from, to), from, to, lang)
	if c.QueryParam("format") != "pdf" {
		return c.Blob(http.StatusOK, "text/plain; charset=utf-8", []byte(notes.Text))
	}
	path, err := renderer().PDF(ctx, notes.Markdown)
	if err != nil {
		return respondError(c, rc, err)
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s-release-notes-%s.pdf"`, project, to))
	return c.File(path)
}
