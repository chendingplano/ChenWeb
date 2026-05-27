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

type deleteOldLogsResponse struct {
	Status  bool   `json:"status"`
	Deleted int64  `json:"deleted"`
	Message string `json:"message"`
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
		EntryType:   c.QueryParam("entry_type"),
		DocProcName: c.QueryParam("doc_proc_name"),
		LLMCallID:   c.QueryParam("llm_call_id"),
		Page:        page,
		PageSize:    pageSize,
	}

	logger.Info("list doc proc logs",
		"entry_type", f.EntryType,
		"doc_proc_name", f.DocProcName,
		"page", f.Page,
		"page_size", f.PageSize,
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
