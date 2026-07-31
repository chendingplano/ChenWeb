package kbhandler

import (
	"net/http"
	"strconv"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type docProcLogRow struct {
	ID            int64    `json:"id"`
	CallReason    string   `json:"call_reason"`
	DocProcName   string   `json:"doc_proc_name"`
	ModelNames    []string `json:"model_names"`
	PromptName    string   `json:"prompt_name"`
	RecordID      *int64   `json:"record_id,omitempty"`
	RunID         *int64   `json:"run_id,omitempty"`
	ProcProgress  *string  `json:"proc_progress,omitempty"`
	EntryType     string   `json:"entry_type"`
	Pass          *int     `json:"pass,omitempty"`
	LLMCallID     *string  `json:"llm_call_id,omitempty"`
	ActivityName  *string  `json:"activity_name,omitempty"`
	ArtifactJSON  *string  `json:"artifact,omitempty"`
	Errors        *string  `json:"errors,omitempty"`
	ExtraInfoJSON *string  `json:"extra_info,omitempty"`
	MSUsed        *int64   `json:"ms_used,omitempty"`
	CreateTime    string   `json:"create_time"`
}

type listDocProcLogsResponse struct {
	Status   bool            `json:"status"`
	Results  []docProcLogRow `json:"results"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

type listDocProcLogFilterOptionsResponse struct {
	Status        bool     `json:"status"`
	EntryTypes    []string `json:"entry_types"`
	DocProcNames  []string `json:"doc_proc_names"`
	ActivityNames []string `json:"activity_names"`
}

type deleteOldLogsResponse struct {
	Status  bool   `json:"status"`
	Deleted int64  `json:"deleted"`
	Message string `json:"message"`
}

type getLatestDocProcessPlanResponse struct {
	Status bool                          `json:"status"`
	Result *latestDocProcessPlanResponse `json:"result,omitempty"`
}

type listDocProcessPlansResponse struct {
	Status   bool                           `json:"status"`
	Results  []latestDocProcessPlanResponse `json:"results"`
	Page     int                            `json:"page"`
	PageSize int                            `json:"page_size"`
	Total    int64                          `json:"total"`
}

type latestDocProcessPlanResponse struct {
	RunID             int64                                             `json:"run_id"`
	RecordID          int64                                             `json:"record_id"`
	Mode              string                                            `json:"mode"`
	Status            string                                            `json:"status"`
	Processors        []string                                          `json:"processors"`
	Parameters        map[string]any                                    `json:"parameters"`
	PlanFacts         docprocessing.ProductionPlanFacts                 `json:"plan_facts"`
	PlanSteps         []docprocessing.ProcessorPlanStep                 `json:"plan_steps"`
	PipelineBinding   docprocessing.ProductionPipelineBindingResolution `json:"pipeline_binding"`
	PipelineSelection docprocessing.ProductionPipelineSelection         `json:"pipeline_selection"`
	CreateTime        string                                            `json:"create_time"`
}

// ListDocProcLogs handles GET /api/v1/kb/doc-proc-logs.
//
// Query params:
//
//	entry_type    - filter by 'llm_call' or 'doc_proc_summary'
//	doc_proc_name - filter by processor name
//	llm_call_id   - filter by LLM call ID
//	page          - 1-based page number (default 1)
//	page_size     - rows per page (default 50, max 500)
//	order_by      - allowed sortable field (default create_time)
//	order_dir     - asc or desc (default desc)
func ListDocProcLogs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DPLG_001")
	defer rc.Close()
	logger := rc.GetLogger()

	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	f := docprocessing.DocProcLogFilter{
		EntryType:    c.QueryParam("entry_type"),
		DocProcName:  c.QueryParam("doc_proc_name"),
		ActivityName: c.QueryParam("activity_name"),
		LLMCallID:    c.QueryParam("llm_call_id"),
		Page:         page,
		PageSize:     pageSize,
		OrderBy:      c.QueryParam("order_by"),
		OrderDir:     c.QueryParam("order_dir"),
	}
	if runIDRaw := c.QueryParam("run_id"); runIDRaw != "" {
		runID, err := parseOptionalPositiveInt64(runIDRaw)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{
				Status:   false,
				ErrorMsg: "query param 'run_id' must be a positive integer (CWB_DPLG_006)",
			})
		}
		f.RunID = runID
	}
	if recordIDRaw := c.QueryParam("record_id"); recordIDRaw != "" {
		recordID, err := strconv.ParseInt(recordIDRaw, 10, 64)
		if err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{
				Status:   false,
				ErrorMsg: "query param 'record_id' must be an integer (CWB_DPLG_005)",
			})
		}
		f.RecordID = &recordID
	}
	createStartTime, err := parseTimeQuery(firstNonEmpty(c.QueryParam("create_start_time"), c.QueryParam("start_time")))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid start_time (CWB_DPLG_007)",
		})
	}
	createEndTime, err := parseTimeQuery(firstNonEmpty(c.QueryParam("create_end_time"), c.QueryParam("end_time")))
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid end_time (CWB_DPLG_008)",
		})
	}
	f.CreateTimeStart = createStartTime
	f.CreateTimeEnd = createEndTime

	logger.Info("list doc proc logs",
		"entry_type", f.EntryType,
		"doc_proc_name", f.DocProcName,
		"activity_name", f.ActivityName,
		"record_id", f.RecordID,
		"run_id", f.RunID,
		"create_start_time", formatOptionalTime(f.CreateTimeStart),
		"create_end_time", formatOptionalTime(f.CreateTimeEnd),
		"page", f.Page,
		"page_size", f.PageSize,
		"order_by", f.OrderBy,
		"order_dir", f.OrderDir,
	)

	store := docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle}
	rows, total, err := store.ListDocProcLogs(c.Request().Context(), f)
	if err != nil {
		logger.Error("list doc proc logs failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to list doc proc logs (CWB_DPLG_010)",
		})
	}

	results := make([]docProcLogRow, 0, len(rows))
	for _, r := range rows {
		names := r.ModelNames
		if names == nil {
			names = []string{}
		}
		results = append(results, docProcLogRow{
			ID:            r.ID,
			CallReason:    r.CallReason,
			DocProcName:   r.DocProcName,
			ModelNames:    names,
			PromptName:    r.PromptName,
			RecordID:      r.RecordID,
			RunID:         r.RunID,
			ProcProgress:  r.ProcProgress,
			EntryType:     r.EntryType,
			Pass:          r.Pass,
			LLMCallID:     r.LLMCallID,
			ActivityName:  r.ActivityName,
			ArtifactJSON:  r.ArtifactJSON,
			Errors:        r.Errors,
			ExtraInfoJSON: r.ExtraInfoJSON,
			MSUsed:        r.MSUsed,
			CreateTime:    r.CreateTime,
		})
	}

	return c.JSON(http.StatusOK, listDocProcLogsResponse{
		Status:   true,
		Results:  results,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

func ListDocProcLogFilterOptions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DPLG_003")
	defer rc.Close()
	logger := rc.GetLogger()

	store := docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle}
	options, err := store.ListDocProcLogFilterOptions(c.Request().Context())
	if err != nil {
		logger.Error("list doc proc log filter options failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to list doc proc log filter options (CWB_DPLG_031)",
		})
	}

	return c.JSON(http.StatusOK, listDocProcLogFilterOptionsResponse{
		Status:        true,
		EntryTypes:    options.EntryTypes,
		DocProcNames:  options.DocProcNames,
		ActivityNames: options.ActivityNames,
	})
}

// GetLatestDocProcessPlan handles GET /api/v1/kb/doc-proc-plans/latest?record_id=N.
func GetLatestDocProcessPlan(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DPLG_041")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil || recordID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "query param 'record_id' must be a positive integer (CWB_DPLG_042)",
		})
	}

	store := docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle}
	view, err := store.GetLatestDocProcessPlan(c.Request().Context(), *recordID)
	if err != nil {
		logger.Error("get latest doc process plan failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to get latest doc process plan (CWB_DPLG_043)",
		})
	}

	return c.JSON(http.StatusOK, getLatestDocProcessPlanResponse{
		Status: true,
		Result: &latestDocProcessPlanResponse{
			RunID:             view.RunID,
			RecordID:          view.RecordID,
			Mode:              view.Mode,
			Status:            view.Status,
			Processors:        view.Processors,
			Parameters:        view.Parameters,
			PlanFacts:         view.PlanFacts,
			PlanSteps:         view.PlanSteps,
			PipelineBinding:   view.PipelineBinding,
			PipelineSelection: view.PipelineSelection,
			CreateTime:        view.CreateTime,
		},
	})
}

// ListDocProcessPlans handles GET /api/v1/kb/doc-proc-plans?record_id=N&page=1&page_size=50.
func ListDocProcessPlans(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DPLG_044")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil || recordID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "query param 'record_id' must be a positive integer (CWB_DPLG_045)",
		})
	}
	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	statusFilter := c.QueryParam("status")
	modeFilter := c.QueryParam("mode")
	pipelineNameFilter := c.QueryParam("pipeline_name")

	store := docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle}
	views, total, err := store.ListDocProcessPlans(c.Request().Context(), *recordID, page, pageSize, statusFilter, modeFilter, pipelineNameFilter)
	if err != nil {
		logger.Error("list doc process plans failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to list doc process plans (CWB_DPLG_046)",
		})
	}

	results := make([]latestDocProcessPlanResponse, 0, len(views))
	for _, view := range views {
		results = append(results, latestDocProcessPlanResponse{
			RunID:             view.RunID,
			RecordID:          view.RecordID,
			Mode:              view.Mode,
			Status:            view.Status,
			Processors:        view.Processors,
			Parameters:        view.Parameters,
			PlanFacts:         view.PlanFacts,
			PlanSteps:         view.PlanSteps,
			PipelineBinding:   view.PipelineBinding,
			PipelineSelection: view.PipelineSelection,
			CreateTime:        view.CreateTime,
		})
	}

	return c.JSON(http.StatusOK, listDocProcessPlansResponse{
		Status:   true,
		Results:  results,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}

// DeleteOldDocProcLogs handles DELETE /api/v1/kb/doc-proc-logs/old.
//
// Query params:
//
//	days - retain logs for this many days; entries older than this are removed (min 1)
func DeleteOldDocProcLogs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DPLG_002")
	defer rc.Close()
	logger := rc.GetLogger()

	daysStr := c.QueryParam("days")
	if daysStr == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "query param 'days' is required (CWB_DPLG_021)",
		})
	}
	days, err := strconv.Atoi(daysStr)
	if err != nil || days < 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "'days' must be a positive integer (CWB_DPLG_022)",
		})
	}

	logger.Info("delete old doc proc logs", "retention_days", days)

	store := docprocessing.SQLStore{DB: ApiTypes.ProjectDBHandle}
	deleted, err := store.DeleteOldDocProcLogs(c.Request().Context(), days)
	if err != nil {
		logger.Error("delete old doc proc logs failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to delete old doc proc logs (CWB_DPLG_030)",
		})
	}

	logger.Info("deleted old doc proc logs", "rows_deleted", deleted, "retention_days", days)
	return c.JSON(http.StatusOK, deleteOldLogsResponse{
		Status:  true,
		Deleted: deleted,
		Message: "deleted " + strconv.FormatInt(deleted, 10) + " log entries older than " + daysStr + " days",
	})
}
