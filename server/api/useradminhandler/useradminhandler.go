package useradminhandler

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/labstack/echo/v4"
)

type listUsersResponse struct {
	Status string               `json:"status"`
	Users  []*ApiTypes.UserInfo `json:"users"`
}

type updateUserRequest struct {
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Status    string   `json:"status"`
	Admin     bool     `json:"admin"`
	Roles     []string `json:"roles"`
}

func requireAdmin(c echo.Context, loc string) (*ApiTypes.UserInfo, ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	currentUser := rc.IsAuthenticated()
	if currentUser == nil {
		return nil, rc, c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
			"loc":   "CWB_USR_010",
		})
	}

	if !currentUser.Admin && !currentUser.IsOwner && !slices.Contains(currentUser.Roles, "admin") {
		return nil, rc, c.JSON(http.StatusForbidden, map[string]string{
			"error": "Admin access required",
			"loc":   "CWB_USR_018",
		})
	}

	return currentUser, rc, nil
}

func ListUsers(c echo.Context) error {
	_, rc, err := requireAdmin(c, "CWB_USR_001")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()
	logger := rc.GetLogger()

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

func UpdateUser(c echo.Context) error {
	currentUser, rc, err := requireAdmin(c, "CWB_USR_030")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()
	logger := rc.GetLogger()

	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
			"loc":   "CWB_USR_039",
		})
	}

	var req updateUserRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
			"loc":   "CWB_USR_047",
		})
	}

	if strings.TrimSpace(req.FirstName) == "" && strings.TrimSpace(req.LastName) == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "At least one name field is required",
			"loc":   "CWB_USR_054",
		})
	}

	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" && status != "trial" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "status must be active, inactive, or trial",
			"loc":   "CWB_USR_064",
		})
	}

	existingUser, err := auth.KratosGetIdentityByID(logger, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
			"loc":   "CWB_USR_072",
		})
	}

	roles := normalizeRoles(req.Roles, req.Admin)
	switch status {
	case "trial":
		if !slices.Contains(roles, "trial") {
			roles = append(roles, "trial")
		}
	case "active":
		roles = removeRole(roles, "trial")
	}

	identityState := status
	if status == "trial" {
		identityState = "active"
	}

	if existingUser.IsOwner && !req.Admin {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Owner accounts must remain admin-enabled",
			"loc":   "CWB_USR_091",
		})
	}

	if currentUser.UserId == userID && !req.Admin {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "You cannot remove your own admin access",
			"loc":   "CWB_USR_099",
		})
	}

	if err := auth.KratosUpdateIdentity(logger, userID, auth.KratosIdentityUpdate{
		Traits: map[string]interface{}{
			"name": map[string]interface{}{
				"first": strings.TrimSpace(req.FirstName),
				"last":  strings.TrimSpace(req.LastName),
			},
		},
		MetadataPublic: map[string]interface{}{
			"admin": req.Admin,
			"roles": roles,
		},
		State: &identityState,
	}); err != nil {
		logger.Error("failed to update kratos user", "error", err, "user_id", userID)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user",
			"loc":   "CWB_USR_117",
		})
	}

	updatedUser, err := auth.KratosGetIdentityByID(logger, userID)
	if err != nil {
		logger.Error("failed to load updated kratos user", "error", err, "user_id", userID)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "User updated but could not be reloaded",
			"loc":   "CWB_USR_126",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"user":   updatedUser,
	})
}

func DeleteUser(c echo.Context) error {
	currentUser, rc, err := requireAdmin(c, "CWB_USR_140")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()
	logger := rc.GetLogger()

	userID := strings.TrimSpace(c.Param("id"))
	if userID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "User ID is required",
			"loc":   "CWB_USR_149",
		})
	}
	if currentUser.UserId == userID {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "You cannot delete your own account",
			"loc":   "CWB_USR_156",
		})
	}

	existingUser, err := auth.KratosGetIdentityByID(logger, userID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
			"loc":   "CWB_USR_164",
		})
	}
	if existingUser.IsOwner {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Owner accounts cannot be deleted",
			"loc":   "CWB_USR_171",
		})
	}

	if err := auth.KratosDeleteIdentity(logger, userID); err != nil {
		logger.Error("failed to delete kratos user", "error", err, "user_id", userID)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete user",
			"loc":   "CWB_USR_179",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"deleted": true,
		"user_id": userID,
	})
}

func normalizeRoles(roles []string, admin bool) []string {
	seen := make(map[string]struct{}, len(roles)+1)
	normalized := make([]string, 0, len(roles)+1)
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	if admin {
		if _, ok := seen["admin"]; !ok {
			normalized = append([]string{"admin"}, normalized...)
		}
	} else {
		normalized = removeRole(normalized, "admin")
	}
	return normalized
}

func removeRole(roles []string, target string) []string {
	filtered := make([]string, 0, len(roles))
	for _, role := range roles {
		if role == target {
			continue
		}
		filtered = append(filtered, role)
	}
	return filtered
}
