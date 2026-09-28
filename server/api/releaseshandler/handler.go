package releaseshandler

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
	rc.GetLogger().Error("release operation failed", "err", err)
	switch {
	case errors.Is(err, ErrInvalid):
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: err.Error()})
	case errors.Is(err, ErrNotFound):
		return c.JSON(http.StatusNotFound, errorResponse{ErrorMsg: err.Error()})
	case strings.Contains(err.Error(), "23505"):
		return c.JSON(http.StatusConflict, errorResponse{ErrorMsg: "this version already exists"})
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse{ErrorMsg: "release operation failed"})
	}
}

func parseID(c echo.Context) (int64, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, ErrInvalid
	}
	return id, nil
}

func ListReleases(c echo.Context) error {
	rc, ok := authorize(c, "CWB_REL_001")
	defer rc.Close()
	if !ok {
		return nil
	}
	releases, err := List(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("listed releases", "count", len(releases))
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": releases})
}

func CreateRelease(c echo.Context) error {
	rc, ok := authorize(c, "CWB_REL_002")
	defer rc.Close()
	if !ok {
		return nil
	}
	var r Release
	if err := json.NewDecoder(c.Request().Body).Decode(&r); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid request body"})
	}
	id, err := Save(c.Request().Context(), ApiTypes.ProjectDBHandle, 0, r)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("created release", "id", id)
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "id": id})
}

func UpdateRelease(c echo.Context) error {
	rc, ok := authorize(c, "CWB_REL_003")
	defer rc.Close()
	if !ok {
		return nil
	}
	id, err := parseID(c)
	if err != nil {
		return respondError(c, rc, err)
	}
	var r Release
	if err := json.NewDecoder(c.Request().Body).Decode(&r); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{ErrorMsg: "invalid request body"})
	}
	_, err = Save(c.Request().Context(), ApiTypes.ProjectDBHandle, id, r)
	if err != nil {
		return respondError(c, rc, err)
	}
	rc.GetLogger().Info("updated release", "id", id)
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

func DeleteRelease(c echo.Context) error {
	rc, ok := authorize(c, "CWB_REL_004")
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
	rc.GetLogger().Info("deleted release", "id", id)
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
