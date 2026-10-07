package kbhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/documentgeometry"
	"github.com/chendingplano/deepdoc/server/api/pathutil"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// GetTableGeometry uses the same authenticated input-record lookup and artifact
// paths as GetRawLines/GetInputFile. Callers never provide filesystem paths.
func GetTableGeometry(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261007-645")
	defer rc.Close()
	logger := rc.GetLogger()
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (20261007-646)"})
	}
	db := ApiTypes.ProjectDBHandle
	table, err := resolveInputTable(db)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to resolve inputs (20261007-647)"})
	}
	var result, pdf sql.NullString
	if err = db.QueryRowContext(c.Request().Context(), fmt.Sprintf("SELECT i.result_filename, i.file_name FROM %s i WHERE i.id = $1", table), id).Scan(&result, &pdf); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "record not found (20261007-648)"})
		}
		logger.Error("query table geometry input failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve record (20261007-649)"})
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	empty := &documentgeometry.Document{Version: documentgeometry.Version, Algorithm: documentgeometry.Algorithm, CoordinateSpace: "page-normalized-1000", Tables: []documentgeometry.Table{}}
	if strings.TrimSpace(result.String) == "" {
		return c.JSON(http.StatusOK, empty)
	}
	geometry, err := documentgeometry.Ensure(rawLinePathFor(result.String), pathutil.ResolveDataHomePath(pdf.String))
	if os.IsNotExist(err) {
		return c.JSON(http.StatusOK, empty)
	}
	if err != nil {
		logger.Error("load table geometry companion failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to retrieve table geometry (20261007-650)"})
	}
	logger.Info("retrieved table geometry", "id", id, "tables", len(geometry.Tables))
	return c.JSON(http.StatusOK, geometry)
}
