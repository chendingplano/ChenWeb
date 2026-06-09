package kbhandler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type checkMD5Request struct {
	MD5s []string `json:"md5s"`
}

type checkMD5Response struct {
	Status   bool     `json:"status"`
	Existing []string `json:"existing"`
}

// CheckMD5s handles POST /api/v1/kb/inputs/check-md5.
// It accepts a list of MD5 hex strings and returns which ones already exist in kb.inputs.
func CheckMD5s(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_CMD5_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req checkMD5Request
	if err := c.Bind(&req); err != nil || len(req.MD5s) == 0 {
		return c.JSON(http.StatusOK, checkMD5Response{Status: true, Existing: []string{}})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to resolve kb input table (CWB_KB_CMD5_010)",
		})
	}

	placeholders := make([]string, len(req.MD5s))
	args := make([]any, len(req.MD5s))
	for i, h := range req.MD5s {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = strings.ToLower(strings.TrimSpace(h))
	}

	query := fmt.Sprintf(
		`SELECT md5 FROM %s WHERE md5 IN (%s)`,
		inputTable,
		strings.Join(placeholders, ","),
	)

	rows, err := db.QueryContext(c.Request().Context(), query, args...)
	if err != nil {
		logger.Error("check md5 query failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to query existing md5s (CWB_KB_CMD5_011)",
		})
	}
	defer rows.Close()

	existing := make([]string, 0)
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err == nil {
			existing = append(existing, h)
		}
	}

	return c.JSON(http.StatusOK, checkMD5Response{Status: true, Existing: existing})
}
