package useradminhandler

import (
	"net/http"
	"slices"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/labstack/echo/v4"
)

type listUsersResponse struct {
	Status string               `json:"status"`
	Users  []*ApiTypes.UserInfo `json:"users"`
}

func ListUsers(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_USR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	currentUser := rc.IsAuthenticated()
	if currentUser == nil {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
			"loc":   "CWB_USR_010",
		})
	}

	if !currentUser.Admin && !currentUser.IsOwner && !slices.Contains(currentUser.Roles, "admin") {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Admin access required",
			"loc":   "CWB_USR_018",
		})
	}

	users, err := auth.KratosListAllIdentities(logger)
	if err != nil {
		logger.Error("failed to list kratos users", "error", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve users",
			"loc":   "CWB_USR_026",
		})
	}

	return c.JSON(http.StatusOK, listUsersResponse{
		Status: "ok",
		Users:  users,
	})
}
