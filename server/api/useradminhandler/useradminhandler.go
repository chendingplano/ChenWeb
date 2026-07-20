package useradminhandler

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/auth"
	"github.com/labstack/echo/v4"
)

type listUsersResponse struct {
	Status       string               `json:"status"`
	Users        []*ApiTypes.UserInfo `json:"users"`
	Scope        string               `json:"scope"`
	CanManageAll bool                 `json:"can_manage_all"`
}

type roleCatalogEntry struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type updateUserRequest struct {
	FirstName string   `json:"first_name"`
	LastName  string   `json:"last_name"`
	Status    string   `json:"status"`
	Admin     bool     `json:"admin"`
	Roles     []string `json:"roles"`
}

var defaultRoleCatalog = []roleCatalogEntry{
	{Key: "admin", Label: "Administrator", Description: "Full administrative access.", Status: "active"},
	{Key: "root", Label: "Root", Description: "Reserved role; not intended for active use yet.", Status: "reserved"},
	{Key: "guest", Label: "Guest", Description: "Limited guest access.", Status: "active"},
	{Key: "dev", Label: "Developer", Description: "Engineering and development workflows.", Status: "active"},
	{Key: "k_engineer", Label: "Knowledge Engineer", Description: "Knowledge-system and taxonomy operations.", Status: "active"},
	{Key: "trial", Label: "Trial", Description: "Trial user access.", Status: "active"},
}

func getRoleCatalog() []roleCatalogEntry {
	roleKeys := appconfig.GetAccessRoles(roleCatalogKeys(defaultRoleCatalog))
	defaultByKey := make(map[string]roleCatalogEntry, len(defaultRoleCatalog))
	for _, role := range defaultRoleCatalog {
		defaultByKey[role.Key] = role
	}

	catalog := make([]roleCatalogEntry, 0, len(roleKeys))
	for _, key := range roleKeys {
		if role, ok := defaultByKey[key]; ok {
			catalog = append(catalog, role)
			continue
		}
		catalog = append(catalog, roleCatalogEntry{
			Key:         key,
			Label:       roleLabelFromKey(key),
			Description: "Project-configured access role.",
			Status:      "active",
		})
	}
	return catalog
}

func roleCatalogKeys(catalog []roleCatalogEntry) []string {
	keys := make([]string, 0, len(catalog))
	for _, role := range catalog {
		keys = append(keys, role.Key)
	}
	return keys
}

func roleLabelFromKey(key string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(key), func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, part := range parts {
		part = strings.ToLower(part)
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
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

func requireAuthenticatedUser(c echo.Context, loc string) (*ApiTypes.UserInfo, ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	currentUser := rc.IsAuthenticated()
	if currentUser == nil {
		return nil, rc, c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "Authentication required",
			"loc":   "CWB_USR_010",
		})
	}
	return currentUser, rc, nil
}

func canManageAllUsers(user *ApiTypes.UserInfo) bool {
	if user == nil {
		return false
	}
	if user.Admin || user.IsOwner {
		return true
	}
	for _, role := range user.Roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "admin" || role == "root" {
			return true
		}
	}
	return false
}

func userManagementStatus(user *ApiTypes.UserInfo) string {
	if user == nil {
		return "active"
	}
	if containsRole(user.Roles, "trial") {
		return "trial"
	}
	if strings.TrimSpace(strings.ToLower(user.UserStatus)) == "inactive" {
		return "inactive"
	}
	return "active"
}

func containsRole(roles []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, role := range roles {
		if strings.ToLower(strings.TrimSpace(role)) == target {
			return true
		}
	}
	return false
}

func ListRoles(c echo.Context) error {
	_, rc, err := requireAuthenticatedUser(c, "CWB_USR_001R")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "ok",
		"roles":  getRoleCatalog(),
	})
}

func ListUsers(c echo.Context) error {
	currentUser, rc, err := requireAuthenticatedUser(c, "CWB_USR_001")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()
	logger := rc.GetLogger()
	canManageAll := canManageAllUsers(currentUser)

	if !canManageAll {
		user, err := findIdentityByEmail(logger, currentUser.Email)
		if err != nil {
			logger.Warn("failed to reload self identity for self-scoped user management", "email", currentUser.Email, "error", err)
			user = currentUser
		}
		return c.JSON(http.StatusOK, listUsersResponse{
			Status:       "ok",
			Users:        []*ApiTypes.UserInfo{user},
			Scope:        "self",
			CanManageAll: false,
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
		Status:       "ok",
		Users:        users,
		Scope:        "all",
		CanManageAll: true,
	})
}

func findIdentityByEmail(logger ApiTypes.JimoLogger, email string) (*ApiTypes.UserInfo, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return nil, echo.NewHTTPError(http.StatusBadRequest, "email is required")
	}

	user, err := auth.KratosGetIdentityByEmail(logger, email)
	if err == nil {
		return user, nil
	}

	users, listErr := auth.KratosListAllIdentities(logger)
	if listErr != nil {
		return nil, err
	}
	for _, candidate := range users {
		if strings.EqualFold(strings.TrimSpace(candidate.Email), email) {
			return candidate, nil
		}
	}
	return nil, err
}

func normalizeEmailParam(raw string) (string, error) {
	decoded, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.ToLower(decoded)), nil
}

func UpdateUser(c echo.Context) error {
	currentUser, rc, err := requireAuthenticatedUser(c, "CWB_USR_030")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()
	logger := rc.GetLogger()
	canManageAll := canManageAllUsers(currentUser)

	targetEmail, err := normalizeEmailParam(c.Param("email"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid email path parameter",
			"loc":   "CWB_USR_039A",
		})
	}
	if targetEmail == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Email is required",
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

	existingUser, err := findIdentityByEmail(logger, targetEmail)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
			"loc":   "CWB_USR_072",
		})
	}

	if !canManageAll && !strings.EqualFold(currentUser.Email, targetEmail) {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "You may only manage your own account",
			"loc":   "CWB_USR_077",
		})
	}

	if !canManageAll {
		req.Admin = existingUser.Admin
		req.Roles = append([]string(nil), existingUser.Roles...)
		status = userManagementStatus(existingUser)
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

	if currentUser.Email == targetEmail && !req.Admin {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "You cannot remove your own admin access",
			"loc":   "CWB_USR_099",
		})
	}

	if err := auth.KratosUpdateIdentity(logger, existingUser.UserId, auth.KratosIdentityUpdate{
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
		logger.Error("failed to update kratos user", "error", err, "user_email", targetEmail)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to update user",
			"loc":   "CWB_USR_117",
		})
	}

	updatedUser, err := findIdentityByEmail(logger, targetEmail)
	if err != nil {
		logger.Error("failed to load updated kratos user", "error", err, "user_email", targetEmail)
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
	currentUser, rc, err := requireAuthenticatedUser(c, "CWB_USR_140")
	if err != nil {
		defer rc.Close()
		return err
	}
	defer rc.Close()
	logger := rc.GetLogger()
	if !canManageAllUsers(currentUser) {
		return c.JSON(http.StatusForbidden, map[string]string{
			"error": "Admin or root access required",
			"loc":   "CWB_USR_141",
		})
	}

	targetEmail, err := normalizeEmailParam(c.Param("email"))
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid email path parameter",
			"loc":   "CWB_USR_149A",
		})
	}
	if targetEmail == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Email is required",
			"loc":   "CWB_USR_149",
		})
	}
	if currentUser.Email == targetEmail {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "You cannot delete your own account",
			"loc":   "CWB_USR_156",
		})
	}

	existingUser, err := findIdentityByEmail(logger, targetEmail)
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

	if err := auth.KratosDeleteIdentity(logger, existingUser.UserId); err != nil {
		logger.Error("failed to delete kratos user", "error", err, "user_email", targetEmail)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to delete user",
			"loc":   "CWB_USR_179",
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"deleted": true,
		"email":   targetEmail,
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
