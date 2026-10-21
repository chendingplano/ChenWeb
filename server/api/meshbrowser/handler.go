package meshbrowser

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// Handler serves the MeSH browser API. The store is created on first use, after the
// project database is open.
type Handler struct {
	once  sync.Once
	store *Store
}

// NewHandler returns a handler over ApiTypes.ProjectDBHandle.
func NewHandler() *Handler { return &Handler{} }

func (h *Handler) s() *Store {
	h.once.Do(func() { h.store = &Store{DB: ApiTypes.ProjectDBHandle} })
	return h.store
}

// Register adds the routes to an authenticated group.
func (h *Handler) Register(g *echo.Group) {
	g.GET("/mesh/overview", h.Overview)
	g.GET("/mesh/entities", h.Entities)
	g.GET("/mesh/tree", h.Tree)
	g.GET("/mesh/entity", h.Entity)
	g.GET("/mesh/neighborhood", h.Neighborhood)
}

func (h *Handler) Overview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_MESH_101")
	defer rc.Close()
	ov, err := h.s().Overview(c.Request().Context())
	if err != nil {
		return fail(c, rc, "mesh overview failed", err)
	}
	return c.JSON(http.StatusOK, ov)
}

func (h *Handler) Entities(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_MESH_102")
	defer rc.Close()
	kind := c.QueryParam("kind")
	if !validKind(kind) {
		return badRequest(c, rc, "kind must be a MeSH vocabulary class name")
	}
	limit, offset, err := paging(c)
	if err != nil {
		return badRequest(c, rc, err.Error())
	}
	rows, err := h.s().ListKind(c.Request().Context(), kind, strings.TrimSpace(c.QueryParam("q")), limit, offset)
	if err != nil {
		return fail(c, rc, "mesh entity list failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": rows, "has_more": len(rows) == limit})
}

func (h *Handler) Tree(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_MESH_103")
	defer rc.Close()
	parent := c.QueryParam("parent")
	if parent != "" && !validID(parent) {
		return badRequest(c, rc, "parent must be a MeSH tree number id")
	}
	rows, err := h.s().Children(c.Request().Context(), parent)
	if err != nil {
		return fail(c, rc, "mesh tree failed", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": rows})
}

func (h *Handler) Neighborhood(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_MESH_105")
	defer rc.Close()
	id := c.QueryParam("id")
	if !validID(id) {
		return badRequest(c, rc, "id must be a MeSH record id such as A01.111")
	}
	node, err := h.s().Neighborhood(c.Request().Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]any{"message": "mesh entity not found"})
	}
	if err != nil {
		return fail(c, rc, "mesh neighborhood failed", err)
	}
	return c.JSON(http.StatusOK, node)
}

func (h *Handler) Entity(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_MESH_104")
	defer rc.Close()
	id := c.QueryParam("id")
	if !validID(id) {
		return badRequest(c, rc, "id must be a MeSH record id such as D000001")
	}
	ent, err := h.s().Entity(c.Request().Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, map[string]any{"message": "mesh entity not found"})
	}
	if err != nil {
		return fail(c, rc, "mesh entity failed", err)
	}
	return c.JSON(http.StatusOK, ent)
}

const maxPage = 200

func paging(c echo.Context) (limit, offset int, err error) {
	limit, offset = 50, 0
	if v := c.QueryParam("limit"); v != "" {
		if limit, err = strconv.Atoi(v); err != nil || limit < 1 || limit > maxPage {
			return 0, 0, errors.New("limit must be between 1 and 200")
		}
	}
	if v := c.QueryParam("offset"); v != "" {
		if offset, err = strconv.Atoi(v); err != nil || offset < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
	}
	return limit, offset, nil
}

// validKind accepts a bare vocabulary class name such as TopicalDescriptor.
func validKind(kind string) bool {
	if kind == "" {
		return false
	}
	for _, r := range kind {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

// validID accepts a MeSH record id: letters, digits and dots (D000001, M0000001, A01.111).
func validID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		if !(r == '.' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func fail(c echo.Context, rc ApiTypes.RequestContext, msg string, err error) error {
	rc.GetLogger().Error(msg, "path", c.Path(), "error", err)
	return c.JSON(http.StatusInternalServerError, map[string]any{"message": msg, "error": err.Error()})
}

func badRequest(c echo.Context, rc ApiTypes.RequestContext, msg string) error {
	rc.GetLogger().Warn("mesh bad request", "path", c.Path(), "error", msg)
	return c.JSON(http.StatusBadRequest, map[string]any{"message": msg})
}
