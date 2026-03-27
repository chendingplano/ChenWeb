package docgenhandler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/deepdoc/server/api/docgenworker"
	"github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// SubmitJob handles POST /api/v1/docgen/jobs
func SubmitJob(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_150")
	defer rc.Close()
	logger := rc.GetLogger()

	var req SubmitJobRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid request body (CWB_DGH_151)"})
	}

	// Validate required fields
	if req.RequestName == "" || req.Purpose == "" || req.TemplateType == "" ||
		req.TemplateName == "" || req.OutputDir == "" || req.OutputFormat == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "request_name, purpose, template_type, template_name, output_dir, output_format are required (CWB_DGH_152)"})
	}

	// Resolve SQL: prefer sql_query_id over sql_statement
	sqlStmt := req.SQLStatement
	if req.SQLQueryID != nil {
		q, err := appdatastores.GetDocGenQuery(ApiTypes.ProjectDBHandle, *req.SQLQueryID)
		if err != nil {
			logger.Error("get query failed", "id", *req.SQLQueryID, "err", err)
			return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to get query (CWB_DGH_153)"})
		}
		if q == nil {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "sql_query_id not found (CWB_DGH_154)"})
		}
		sqlStmt = q.SQLStatement
	}

	// Validate SQL is SELECT
	if err := docgenworker.ValidateSQLStatement(sqlStmt); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_DGH_155)"})
	}

	// Serialize and validate converter
	if req.Converter == nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "converter is required (CWB_DGH_156)"})
	}
	converterBytes, err := json.Marshal(req.Converter)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid converter (CWB_DGH_157)"})
	}
	converterJSON := string(converterBytes)
	if _, err := docgenworker.ValidateConverter(converterJSON); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_DGH_158)"})
	}

	// Resolve template path
	templatePath := filepath.Join(config.GetDocGenConfig().TemplateDir, req.TemplateName)
	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "template file not found (CWB_DGH_159)"})
		}
		logger.Error("stat template failed", "path", templatePath, "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to check template (CWB_DGH_160)"})
	}

	// Determine createdBy from auth context
	createdBy := "unknown"
	userInfo := rc.IsAuthenticated()
	if userInfo != nil {
		createdBy = userInfo.UserName
	}

	// Insert job into DB
	jobID, err := appdatastores.InsertDocGenJob(ApiTypes.ProjectDBHandle, appdatastores.DocGenJob{
		RequestName:  req.RequestName,
		Purpose:      req.Purpose,
		Remarks:      req.Remarks,
		SQLQueryID:   req.SQLQueryID,
		SQLStatement: sqlStmt,
		TemplateType: req.TemplateType,
		TemplatePath: templatePath,
		Converter:    converterJSON,
		OutputDir:    req.OutputDir,
		OutputFormat: req.OutputFormat,
		CreatedBy:    createdBy,
	})
	if err != nil {
		errStr := err.Error()
		if strings.Contains(strings.ToLower(errStr), "unique") || strings.Contains(strings.ToLower(errStr), "duplicate") {
			return c.JSON(http.StatusConflict, ErrorResponse{Status: false, ErrorMsg: "request_name already exists (CWB_DGH_161)"})
		}
		logger.Error("insert job failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to create job (CWB_DGH_162)"})
	}

	// Push job ID onto worker channel (non-blocking)
	if docgenworker.JobChannel != nil {
		select {
		case docgenworker.JobChannel <- jobID:
		default:
			logger.Info("job channel full, job will be picked up on next requeue", "job_id", jobID)
		}
	}

	logger.Info("job submitted", "job_id", jobID, "request_name", req.RequestName, "created_by", createdBy)
	return c.JSON(http.StatusCreated, SubmitJobResponse{Status: true, JobID: jobID})
}

// ListJobs handles GET /api/v1/docgen/jobs
func ListJobs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_170")
	defer rc.Close()
	logger := rc.GetLogger()

	status := c.QueryParam("status")
	requestName := c.QueryParam("request_name")
	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), 20)

	if ApiTypes.ProjectDBHandle == nil {
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "database not available (CWB_DGH_170a)"})
	}
	jobs, total, err := appdatastores.ListDocGenJobs(ApiTypes.ProjectDBHandle, status, requestName, page, pageSize)
	if err != nil {
		logger.Error("list jobs failed", "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to list jobs (CWB_DGH_171)"})
	}
	return c.JSON(http.StatusOK, JobListResponse{
		Status:   true,
		Jobs:     jobs,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// GetJob handles GET /api/v1/docgen/jobs/:id
func GetJob(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DGH_180")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Status: false, ErrorMsg: "invalid id (CWB_DGH_181)"})
	}

	job, err := appdatastores.GetDocGenJob(ApiTypes.ProjectDBHandle, id)
	if err != nil {
		logger.Error("get job failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to get job (CWB_DGH_182)"})
	}
	if job == nil {
		return c.JSON(http.StatusNotFound, ErrorResponse{Status: false, ErrorMsg: "job not found (CWB_DGH_183)"})
	}

	logs, err := appdatastores.ListDocGenLogByJobID(ApiTypes.ProjectDBHandle, id)
	if err != nil {
		logger.Error("list job logs failed", "id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Status: false, ErrorMsg: "failed to get job logs (CWB_DGH_184)"})
	}

	return c.JSON(http.StatusOK, JobDetailResponse{
		Status: true,
		Job:    job,
		Logs:   logs,
	})
}

// parsePositiveInt parses a string to a positive int, returning def on failure or non-positive value.
func parsePositiveInt(raw string, def int) int {
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return def
	}
	return v
}
