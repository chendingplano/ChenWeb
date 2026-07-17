package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type docReviewLogRow struct {
	ID            int64           `json:"id"`
	InputRecordID int64           `json:"input_record_id"`
	RunID         int64           `json:"run_id"`
	Pass          string          `json:"pass"`
	Aspect        string          `json:"aspect"`
	UnitType      string          `json:"unit_type"`
	UnitKey       string          `json:"unit_key"`
	UnitLocation  json.RawMessage `json:"unit_location"`
	MatchedUnits  json.RawMessage `json:"matched_units"`
	Findings      json.RawMessage `json:"findings"`
	Outcome       string          `json:"outcome"`
	Detail        json.RawMessage `json:"detail"`
	CreateTime    string          `json:"create_time"`
}

type listDocReviewLogsResponse struct {
	Status   bool              `json:"status"`
	Results  []docReviewLogRow `json:"results"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
}

// ListDocReviewLogs handles GET /api/v1/kb/doc-review-logs.
func ListDocReviewLogs(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DRL_001")
	defer rc.Close()
	logger := rc.GetLogger()

	page := parsePositiveInt(c.QueryParam("page"), 1)
	pageSize := parsePositiveInt(c.QueryParam("page_size"), defaultPageSize)
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	inputRecordID, err := parseOptionalInt64(c.QueryParam("input_record_id"))
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'input_record_id' must be an integer (CWB_DRL_002)")
	}
	runID, err := parseOptionalInt64(c.QueryParam("run_id"))
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'run_id' must be an integer (CWB_DRL_003)")
	}
	createStartTime, err := parseRFC3339Query(c.QueryParam("create_start_time"))
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'create_start_time' must be RFC3339 (CWB_DRL_004)")
	}
	createEndTime, err := parseRFC3339Query(c.QueryParam("create_end_time"))
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'create_end_time' must be RFC3339 (CWB_DRL_005)")
	}
	if createStartTime != nil && createEndTime != nil && createStartTime.After(*createEndTime) {
		return docReviewLogBadRequest(c, "query param 'create_start_time' must not be after 'create_end_time' (CWB_DRL_006)")
	}

	filter := docprocessing.DocReviewLogFilter{
		InputRecordID:   inputRecordID,
		RunID:           runID,
		Pass:            c.QueryParam("pass"),
		Aspect:          c.QueryParam("aspect"),
		UnitType:        c.QueryParam("unit_type"),
		UnitKey:         c.QueryParam("unit_key"),
		Outcome:         c.QueryParam("outcome"),
		CreateTimeStart: createStartTime,
		CreateTimeEnd:   createEndTime,
		Page:            page,
		PageSize:        pageSize,
	}
	logger.Info("list doc review logs",
		"input_record_id", filter.InputRecordID,
		"run_id", filter.RunID,
		"pass", filter.Pass,
		"aspect", filter.Aspect,
		"unit_type", filter.UnitType,
		"unit_key", filter.UnitKey,
		"outcome", filter.Outcome,
		"create_start_time", formatOptionalTime(filter.CreateTimeStart),
		"create_end_time", formatOptionalTime(filter.CreateTimeEnd),
		"page", page,
		"page_size", pageSize,
	)

	rows, total, err := (docprocessing.DocReviewLogSQLStore{DB: ApiTypes.ProjectDBHandle}).ListDocReviewLogs(c.Request().Context(), filter)
	if err != nil {
		logger.Error("list doc review logs failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list doc review logs (CWB_DRL_010)"})
	}
	results := make([]docReviewLogRow, 0, len(rows))
	for _, row := range rows {
		results = append(results, docReviewLogRow{
			ID: row.ID, InputRecordID: row.InputRecordID, RunID: row.RunID,
			Pass: row.Pass, Aspect: row.Aspect, UnitType: row.UnitType, UnitKey: row.UnitKey,
			UnitLocation: row.UnitLocation, MatchedUnits: row.MatchedUnits, Findings: row.Findings,
			Outcome: row.Outcome, Detail: row.Detail, CreateTime: row.CreateTime,
		})
	}
	return c.JSON(http.StatusOK, listDocReviewLogsResponse{Status: true, Results: results, Page: page, PageSize: pageSize, Total: total})
}

func docReviewLogBadRequest(c echo.Context, message string) error {
	return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: message})
}

func parseOptionalInt64(raw string) (*int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseRFC3339Query(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
