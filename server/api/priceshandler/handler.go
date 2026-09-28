package priceshandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

func authorize(c echo.Context, loc string) (ApiTypes.RequestContext, bool) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, errorResponse{ErrorMsg: "authentication required"})
		return rc, false
	}
	admin := user.Admin || user.IsOwner
	for _, role := range user.Roles {
		if role = strings.ToLower(strings.TrimSpace(role)); role == "admin" || role == "root" {
			admin = true
		}
	}
	if !admin {
		_ = c.JSON(http.StatusForbidden, errorResponse{ErrorMsg: "admin access required"})
		return rc, false
	}
	return rc, true
}

func respondError(c echo.Context, rc ApiTypes.RequestContext, err error) error {
	rc.GetLogger().Error("price operation failed", "err", err)
	switch {
	case errors.Is(err, ErrInvalid):
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, errorResponse{ErrorMsg: err.Error()})
	case strings.Contains(err.Error(), "23505"):
		return c.JSON(http.StatusConflict, errorResponse{ErrorMsg: "this price definition name already exists"})
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse{ErrorMsg: "price operation failed"})
	}
}

func parseID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalid
	}
	return id, nil
}

func ListPrices(c echo.Context) error {
	rc, ok := authorize(c, "CWB_PRC_001")
	defer rc.Close()
	if !ok {
		return nil
	}
	defs, err := List(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("listed price definitions", "count", len(defs))
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": defs})
}

func CreatePrice(c echo.Context) error {
	rc, ok := authorize(c, "CWB_PRC_002")
	defer rc.Close()
	if !ok {
		return nil
	}
	var p PriceDef
	if err := json.NewDecoder(c.Request().Body).Decode(&p); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid request body"})
	}
	id, err := Save(c.Request().Context(), ApiTypes.ProjectDBHandle, 0, p)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("created price definition", "id", id, "name", p.PriceDefName)
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "id": id})
}

func UpdatePrice(c echo.Context) error {
	rc, ok := authorize(c, "CWB_PRC_003")
	defer rc.Close()
	if !ok {
		return nil
	}
	id, err := parseID(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	var p PriceDef
	if err := json.NewDecoder(c.Request().Body).Decode(&p); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid request body"})
	}
	_, err = Save(c.Request().Context(), ApiTypes.ProjectDBHandle, id, p)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("updated price definition", "id", id, "name", p.PriceDefName)
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

func DeletePrice(c echo.Context) error {
	rc, ok := authorize(c, "CWB_PRC_004")
	defer rc.Close()
	if !ok {
		return nil
	}
	id, err := parseID(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	if err := Delete(c.Request().Context(), ApiTypes.ProjectDBHandle, id); err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("deleted price definition", "id", id)
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
