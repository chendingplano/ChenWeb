package peakhourshandler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// errAdminCheckFailed is returned by requireAdmin once it has already
// written the 401/403 response, purely to signal callers to stop — its text
// is never sent to the client.
var errAdminCheckFailed = errors.New("admin check failed")

// requireAdmin authorizes peak-hours CRUD operations: an authenticated user
// who is admin/owner or holds the "admin"/"root" role. On failure it writes
// the error response itself and returns errAdminCheckFailed; callers must
// treat any non-nil error as "response already sent, stop here" (c.JSON
// returns nil on a successful write, so its result cannot be used as that
// signal).
func requireAdmin(c echo.Context, loc string) (ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, errorResponse{Status: false, ErrorMsg: "authentication required (" + loc + ")"})
		return rc, errAdminCheckFailed
	}
	admin := user.IsOwner || user.Admin
	if !admin {
		for _, role := range user.Roles {
			role = strings.ToLower(strings.TrimSpace(role))
			if role == "admin" || role == "root" {
				admin = true
				break
			}
		}
	}
	if !admin {
		_ = c.JSON(http.StatusForbidden, errorResponse{Status: false, ErrorMsg: "admin access required (" + loc + ")"})
		return rc, errAdminCheckFailed
	}
	return rc, nil
}

func decodeJSON(c echo.Context, target any) error {
	return json.NewDecoder(c.Request().Body).Decode(target)
}

func peakHoursError(c echo.Context, rc ApiTypes.RequestContext, op, loc string, err error) error {
	rc.GetLogger().Error(op, "err", err)
	switch {
	case errors.Is(err, ErrPeakHoursNotFound):
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "peak hours not found (" + loc + ")"})
	case errors.Is(err, ErrPeakHoursInvalid):
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error() + " (" + loc + ")"})
	case strings.Contains(err.Error(), "23505"):
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "a peak hours definition with this name already exists (" + loc + ")"})
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: op + " (" + loc + ")"})
	}
}

// List handles GET /api/v1/peak-hours
func List(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_PKH_001")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	list, err := ListPeakHours(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		rc.GetLogger().Error("list peak hours failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list peak hours (CWB_PKH_002)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": list, "total": len(list)})
}

// Create handles POST /api/v1/peak-hours
func Create(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_PKH_010")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	var payload PeakHours
	if err := decodeJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_PKH_011)"})
	}
	created, err := CreatePeakHours(c.Request().Context(), ApiTypes.ProjectDBHandle, payload)
	if err != nil {
		return peakHoursError(c, rc, "create peak hours failed", "CWB_PKH_012", err)
	}
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "record": created})
}

// Update handles PUT /api/v1/peak-hours/:name
func Update(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_PKH_020")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid name (CWB_PKH_021)"})
	}
	var payload PeakHours
	if err := decodeJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_PKH_022)"})
	}
	updated, err := UpdatePeakHours(c.Request().Context(), ApiTypes.ProjectDBHandle, name, payload)
	if err != nil {
		return peakHoursError(c, rc, "update peak hours failed", "CWB_PKH_023", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": updated})
}

// Delete handles DELETE /api/v1/peak-hours/:name
func Delete(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_PKH_030")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid name (CWB_PKH_031)"})
	}
	if err := DeletePeakHours(c.Request().Context(), ApiTypes.ProjectDBHandle, name); err != nil {
		return peakHoursError(c, rc, "delete peak hours failed", "CWB_PKH_032", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// IsActive handles GET /api/v1/peak-hours/:name/is-active?at=<RFC3339>
// It is not admin-gated: other backend code needs to query it.
func IsActive(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_PKH_040")
	defer rc.Close()

	name := strings.TrimSpace(c.Param("name"))
	if name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid name (CWB_PKH_041)"})
	}

	at := time.Now()
	if raw := strings.TrimSpace(c.QueryParam("at")); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid at (must be RFC3339) (CWB_PKH_042)"})
		}
		at = parsed
	}

	ctx := c.Request().Context()
	p, err := GetPeakHoursByName(ctx, ApiTypes.ProjectDBHandle, name)
	if err != nil {
		return peakHoursError(c, rc, "load peak hours failed", "CWB_PKH_043", err)
	}

	active, err := EvaluateActive(ctx, ApiTypes.ProjectDBHandle, p, at)
	if err != nil {
		rc.GetLogger().Error("evaluate peak hours failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to evaluate peak hours (CWB_PKH_044)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "active": active})
}
