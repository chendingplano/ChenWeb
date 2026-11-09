package kbhandler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type objectTypeReviewStore interface {
	List(context.Context) ([]docprocessing.ObjectType, error)
	Approve(context.Context, int64, string) (docprocessing.ObjectType, error)
}

// objectTypeReviewAdmin writes authorization failures and returns false so
// callers stop even when c.JSON successfully returns nil.
func objectTypeReviewAdmin(c echo.Context, rc ApiTypes.RequestContext) (*ApiTypes.UserInfo, bool) {
	user := rc.IsAuthenticated()
	if user == nil {
		_ = c.JSON(http.StatusUnauthorized, errorResponse{Status: false, ErrorMsg: "authentication required"})
		return nil, false
	}
	admin := user.Admin || user.IsOwner
	for _, role := range user.Roles {
		switch strings.ToLower(strings.TrimSpace(role)) {
		case "admin", "root":
			admin = true
		}
	}
	if !admin {
		_ = c.JSON(http.StatusForbidden, errorResponse{Status: false, ErrorMsg: "admin access required"})
		return nil, false
	}
	return user, true
}

// ListObjectTypes handles GET /api/v1/kb/object-types. Registry review is
// available to administrators; extraction discovers proposed entries separately.
func ListObjectTypes(c echo.Context) error {
	return listObjectTypes(c, docprocessing.ObjectTypeSQLStore{DB: ApiTypes.ProjectDBHandle})
}

func listObjectTypes(c echo.Context, store objectTypeReviewStore) error {
	rc := EchoFactory.NewFromEcho(c, "20261108-571")
	defer rc.Close()
	if _, ok := objectTypeReviewAdmin(c, rc); !ok {
		return nil
	}
	rows, err := store.List(c.Request().Context())
	if err != nil {
		rc.GetLogger().Error("object types: list failed", "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list object types"})
	}
	if rows == nil {
		rows = []docprocessing.ObjectType{}
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "rows": rows})
}

// ApproveObjectType handles POST /api/v1/kb/object-types/:id/approve. The
// registry keeps the same entry and first approval attribution on repeated calls.
func ApproveObjectType(c echo.Context) error {
	return approveObjectType(c, docprocessing.ObjectTypeSQLStore{DB: ApiTypes.ProjectDBHandle})
}

func approveObjectType(c echo.Context, store objectTypeReviewStore) error {
	rc := EchoFactory.NewFromEcho(c, "20261108-572")
	defer rc.Close()
	user, ok := objectTypeReviewAdmin(c, rc)
	if !ok {
		return nil
	}
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid object type id"})
	}
	row, err := store.Approve(c.Request().Context(), id, user.UserId)
	if errors.Is(err, sql.ErrNoRows) {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "object type not found"})
	}
	if err != nil {
		rc.GetLogger().Error("object types: approval failed", "object_type_id", id, "actor", user.UserId, "error", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to approve object type"})
	}
	rc.GetLogger().Info("object types: approved", "object_type_id", id, "actor", user.UserId)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "row": row})
}
