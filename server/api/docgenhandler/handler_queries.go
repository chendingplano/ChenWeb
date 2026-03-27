package docgenhandler

import (
	"net/http"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ListQueries handles GET /api/v1/docgen/queries?q=<search>
func ListQueries(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_100")
	defer rc.Close()
	logger := rc.GetLogger()

	search := c.QueryParam("q")
	queries, err := appdatastores.ListDocGenQueries(ApiTypes.ProjectDBHandle, search)
	if err != nil {
		logger.Error("list queries failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to list queries (CWB_DGH_101)"})
	}
	return c.JSON(http.StatusOK, QueryListResponse{Status: true, Queries: queries})
}

// CreateQuery handles POST /api/v1/docgen/queries — admin only
func CreateQuery(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_110")
	defer rc.Close()
	logger := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil || !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin only (CWB_DGH_111)"})
	}

	var req CreateQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid request body (CWB_DGH_112)"})
	}
	if req.Name == "" || req.SQLStatement == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "name and sql_statement are required (CWB_DGH_113)"})
	}

	id, err := appdatastores.InsertDocGenQuery(ApiTypes.ProjectDBHandle, appdatastores.DocGenQuery{
		Name:         req.Name,
		Description:  req.Description,
		SQLStatement: req.SQLStatement,
		CreatedBy:    userInfo.UserName,
	})
	if err != nil {
		logger.Error("create query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to create query (CWB_DGH_114)"})
	}
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "id": id})
}

// UpdateQuery handles PUT /api/v1/docgen/queries/:id — admin only
func UpdateQuery(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_120")
	defer rc.Close()
	logger := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil || !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin only (CWB_DGH_121)"})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid id (CWB_DGH_122)"})
	}

	var req UpdateQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid request body (CWB_DGH_123)"})
	}

	if err := appdatastores.UpdateDocGenQuery(ApiTypes.ProjectDBHandle, id, req.Name, req.Description, req.SQLStatement); err != nil {
		logger.Error("update query failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to update query (CWB_DGH_124)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// DeleteQuery handles DELETE /api/v1/docgen/queries/:id — admin only
func DeleteQuery(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_130")
	defer rc.Close()
	logger := rc.GetLogger()

	userInfo := rc.IsAuthenticated()
	if userInfo == nil || !userInfo.Admin {
		return c.JSON(http.StatusForbidden, ErrorResponse{Status: false, ErrorMsg: "admin only (CWB_DGH_131)"})
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid id (CWB_DGH_132)"})
	}

	if err := appdatastores.DeleteDocGenQuery(ApiTypes.ProjectDBHandle, id); err != nil {
		logger.Error("delete query failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to delete query (CWB_DGH_133)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
