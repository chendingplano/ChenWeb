package useradminhandler

import (
	"encoding/json"
	"net/http"
	"net/mail"
	"slices"
	"strings"

	"github.com/chendingplano/shared/go/api/auth"
	"github.com/labstack/echo/v4"
)

// CreateUser provisions a password identity using the same access policy as
// listing and deleting managed users. The password is never returned or logged.
func CreateUser(c echo.Context) error {
	current, rc, err := requireAuthenticatedUser(c, "20261027-581")
	defer rc.Close()
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	} // Authentication helper already wrote 401.
	if !canManageAllUsers(current) {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "Admin or root access required"})
	}
	logger := rc.GetLogger()
	var req struct {
		updateUserRequest
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "Invalid request body"})
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	address, err := mail.ParseAddress(req.Email)
	if err != nil || address.Address != req.Email || strings.TrimSpace(req.Password) == "" {
		return c.JSON(400, map[string]string{"error": "A valid email and initial password are required"})
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "inactive" && status != "trial" {
		return c.JSON(400, map[string]string{"error": "status must be active, inactive, or trial"})
	}
	roles := normalizeRoles(req.Roles, req.Admin)
	if status == "trial" && !slices.Contains(roles, "trial") {
		roles = append(roles, "trial")
	}
	if status == "active" {
		roles = removeRole(roles, "trial")
	}
	user, err := auth.KratosCreateIdentityWithPassword(logger, req.Email, strings.TrimSpace(req.FirstName), strings.TrimSpace(req.LastName), req.Password)
	if err != nil {
		logger.Warn("failed to create managed user", "email", req.Email, "error", err)
		code := http.StatusBadGateway
		message := "Failed to create user. Check the email and password requirements."
		if strings.Contains(err.Error(), "SHD_KAH_073") {
			code = http.StatusConflict
			message = "An account with this email already exists"
		}
		return c.JSON(code, map[string]string{"error": message})
	}
	state := status
	if state == "trial" {
		state = "active"
	}
	if err := auth.KratosUpdateIdentity(logger, user.UserId, auth.KratosIdentityUpdate{MetadataPublic: map[string]interface{}{"admin": req.Admin, "roles": roles}, State: &state}); err != nil {
		logger.Error("failed to configure created user", "user_id", user.UserId, "error", err)
		message := "Failed to configure user; please try again"
		if rollbackErr := auth.KratosDeleteIdentity(logger, user.UserId); rollbackErr != nil {
			logger.Error("failed to roll back created user", "user_id", user.UserId, "error", rollbackErr)
			message = "User created but configuration failed. Refresh the list and edit the account."
		}
		return c.JSON(502, map[string]string{"error": message})
	}
	user.Admin = req.Admin
	user.Roles = roles
	user.UserStatus = state
	logger.Info("created managed user", "user_id", user.UserId, "actor_id", current.UserId)
	return c.JSON(http.StatusCreated, map[string]interface{}{"status": "ok", "user": user})
}
