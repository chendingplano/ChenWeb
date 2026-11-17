package attention

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type itemStore interface {
	Get(ctx context.Context, id int64) (Item, error)
	List(ctx context.Context, f ListFilter) ([]Item, int, error)
	Close(ctx context.Context, id int64, status, actor, resolution string) (Item, error)
}

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

func projectStore() SQLStore { return SQLStore{DB: ApiTypes.ProjectDBHandle} }

// ListItems handles GET /api/v1/kb/attention-items?type=&status=&limit=&offset=.
func ListItems(c echo.Context) error { return listItems(c, projectStore()) }

// GetItem handles GET /api/v1/kb/attention-items/:id.
func GetItem(c echo.Context) error { return getItem(c, projectStore()) }

// ResolveItem handles POST /api/v1/kb/attention-items/:id/resolve.
func ResolveItem(c echo.Context) error { return closeItem(c, projectStore(), StatusResolved) }

// DismissItem handles POST /api/v1/kb/attention-items/:id/dismiss.
func DismissItem(c echo.Context) error { return closeItem(c, projectStore(), StatusDismissed) }

func requireUser(c echo.Context, rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, bool) {
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, errorResponse{ErrorMsg: "authentication required"})
		return nil, false
	}
	return user, true
}

func listItems(c echo.Context, store itemStore) error {
	rc := EchoFactory.NewFromEcho(c, "20261117-410")
	defer rc.Close()
	if _, ok := requireUser(c, rc); !ok {
		return nil
	}
	f := ListFilter{
		ItemType: strings.TrimSpace(c.QueryParam("type")),
		Status:   strings.TrimSpace(c.QueryParam("status")),
	}
	if f.ItemType != "" && !KnownType(f.ItemType) {
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "unknown attention item type"})
	}
	switch f.Status {
	case "", StatusOpen, StatusResolved, StatusDismissed:
	default:
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid status"})
	}
	f.Limit, _ = strconv.Atoi(c.QueryParam("limit"))
	f.Offset, _ = strconv.Atoi(c.QueryParam("offset"))
	items, total, err := store.List(c.Request().Context(), f)
	if err != nil {
		rc.GetLogger().Error("attention items: list failed", "type", f.ItemType, "status", f.Status, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{ErrorMsg: "failed to list attention items"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "items": items, "total": total})
}

func parseID(c echo.Context) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		_ = c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid attention item id"})
		return 0, false
	}
	return id, true
}

func getItem(c echo.Context, store itemStore) error {
	rc := EchoFactory.NewFromEcho(c, "20261117-421")
	defer rc.Close()
	if _, ok := requireUser(c, rc); !ok {
		return nil
	}
	id, ok := parseID(c)
	if !ok {
		return nil
	}
	it, err := store.Get(c.Request().Context(), id)
	if errors.Is(err, ErrNotFound) {
		return c.JSON(http.StatusNotFound, errorResponse{ErrorMsg: "attention item not found"})
	}
	if err != nil {
		rc.GetLogger().Error("attention items: get failed", "id", id, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{ErrorMsg: "failed to load attention item"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "item": it})
}

type closeRequest struct {
	Resolution string `json:"resolution"`
}

func closeItem(c echo.Context, store itemStore, status string) error {
	rc := EchoFactory.NewFromEcho(c, "20261117-437")
	defer rc.Close()
	user, ok := requireUser(c, rc)
	if !ok {
		return nil
	}
	id, ok := parseID(c)
	if !ok {
		return nil
	}
	var req closeRequest
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&req); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid request body"})
		}
	}
	actor := user.UserName
	if actor == "" {
		actor = user.UserId
	}
	it, err := store.Close(c.Request().Context(), id, status, actor, req.Resolution)
	switch {
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, errorResponse{ErrorMsg: "attention item not found"})
	case errors.Is(err, ErrNotOpen):
		return c.JSON(http.StatusConflict, errorResponse{ErrorMsg: "attention item is not open"})
	case err != nil:
		rc.GetLogger().Error("attention items: close failed", "id", id, "status", status, "actor", actor, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{ErrorMsg: "failed to update attention item"})
	}
	rc.GetLogger().Info("attention items: closed", "id", id, "item_type", it.ItemType, "status", status, "actor", actor)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "item": it})
}
