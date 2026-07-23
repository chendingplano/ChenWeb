package kbhandler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/scheduler"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// Schedules admin page (System Admin -> Schedules): CRUD for
// kb.scheduled_jobs plus run history from kb.scheduled_job_runs. The actual
// execution engine lives in server/api/scheduler and is started once at
// process startup (see cmd/deepdoc/main.go); these handlers only manage the
// schedule rows it reads.

type jobTypeDTO struct {
	JobType string `json:"job_type"`
	Label   string `json:"label"`
}

// ListScheduleJobTypes returns the registered job types for the "add
// schedule" form's dropdown.
//
// GET /kb/schedule-job-types
func ListScheduleJobTypes(c echo.Context) error {
	registry := DefaultSchedulerRegistry()
	out := make([]jobTypeDTO, 0, len(registry))
	for jobType, descriptor := range registry {
		out = append(out, jobTypeDTO{JobType: jobType, Label: descriptor.Label})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].JobType < out[j].JobType })
	return c.JSON(http.StatusOK, map[string]any{"status": true, "job_types": out})
}

type scheduleDTO struct {
	ID              int64          `json:"id"`
	Name            string         `json:"name"`
	JobType         string         `json:"job_type"`
	IntervalSeconds int            `json:"interval_seconds"`
	Params          map[string]any `json:"params"`
	Enabled         bool           `json:"enabled"`
	NextRunAt       string         `json:"next_run_at"`
	LastRunAt       string         `json:"last_run_at,omitempty"`
	LastRunStatus   string         `json:"last_run_status,omitempty"`
}

func toScheduleDTO(s scheduler.Schedule) scheduleDTO {
	dto := scheduleDTO{
		ID: s.ID, Name: s.Name, JobType: s.JobType, IntervalSeconds: s.IntervalSeconds,
		Params: s.Params, Enabled: s.Enabled, NextRunAt: s.NextRunAt.Format(rfc3339Layout),
		LastRunStatus: s.LastRunStatus,
	}
	if s.LastRunAt != nil {
		dto.LastRunAt = s.LastRunAt.Format(rfc3339Layout)
	}
	if dto.Params == nil {
		dto.Params = map[string]any{}
	}
	return dto
}

const rfc3339Layout = "2006-01-02T15:04:05Z07:00"

type createScheduleRequest struct {
	Name            string         `json:"name"`
	JobType         string         `json:"job_type"`
	IntervalSeconds int            `json:"interval_seconds"`
	Params          map[string]any `json:"params"`
	Enabled         *bool          `json:"enabled"`
}

// CreateSchedule adds a new schedule, due immediately.
//
// POST /kb/schedules
func CreateSchedule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCH_001")
	defer rc.Close()
	logger := rc.GetLogger()

	var req createScheduleRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_SCH_002)"})
	}
	req.Name = strings.TrimSpace(req.Name)
	req.JobType = strings.TrimSpace(req.JobType)
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_SCH_003)"})
	}
	if _, ok := DefaultSchedulerRegistry()[req.JobType]; !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "unknown job_type (CWB_KB_SCH_004)"})
	}
	if req.IntervalSeconds <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "interval_seconds must be positive (CWB_KB_SCH_005)"})
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_SCH_010)"})
	}
	store := scheduler.SQLStore{DB: db}
	sched, err := store.CreateSchedule(c.Request().Context(), scheduler.CreateScheduleInput{
		Name: req.Name, JobType: req.JobType, IntervalSeconds: req.IntervalSeconds,
		Params: req.Params, Enabled: enabled,
	})
	if err != nil {
		logger.Error("create schedule failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "create schedule failed (CWB_KB_SCH_011)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "schedule": toScheduleDTO(sched)})
}

// ListSchedules returns every schedule, for the "active schedules" view.
//
// GET /kb/schedules
func ListSchedules(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCH_020")
	defer rc.Close()
	logger := rc.GetLogger()

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_SCH_021)"})
	}
	store := scheduler.SQLStore{DB: db}
	schedules, err := store.ListSchedules(c.Request().Context())
	if err != nil {
		logger.Error("list schedules failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "list schedules failed (CWB_KB_SCH_022)"})
	}
	dtos := make([]scheduleDTO, 0, len(schedules))
	for _, s := range schedules {
		dtos = append(dtos, toScheduleDTO(s))
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "schedules": dtos})
}

type updateScheduleRequest struct {
	Name            string         `json:"name"`
	IntervalSeconds int            `json:"interval_seconds"`
	Params          map[string]any `json:"params"`
	Enabled         bool           `json:"enabled"`
}

// UpdateSchedule edits an existing schedule (name/interval/params/enabled).
//
// PATCH /kb/schedules/:id
func UpdateSchedule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCH_030")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_SCH_031)"})
	}
	var req updateScheduleRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_SCH_032)"})
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "name is required (CWB_KB_SCH_033)"})
	}
	if req.IntervalSeconds <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "interval_seconds must be positive (CWB_KB_SCH_034)"})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_SCH_040)"})
	}
	store := scheduler.SQLStore{DB: db}
	if err := store.UpdateSchedule(c.Request().Context(), id, scheduler.UpdateScheduleInput{
		Name: req.Name, IntervalSeconds: req.IntervalSeconds, Params: req.Params, Enabled: req.Enabled,
	}); err != nil {
		logger.Error("update schedule failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "update schedule failed (CWB_KB_SCH_041)"})
	}
	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}

// DeleteSchedule removes a schedule; its run history cascades.
//
// DELETE /kb/schedules/:id
func DeleteSchedule(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCH_050")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_SCH_051)"})
	}
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_SCH_052)"})
	}
	store := scheduler.SQLStore{DB: db}
	if err := store.DeleteSchedule(c.Request().Context(), id); err != nil {
		logger.Error("delete schedule failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "delete schedule failed (CWB_KB_SCH_053)"})
	}
	return c.JSON(http.StatusOK, map[string]bool{"status": true})
}

type runDTO struct {
	ID         int64          `json:"id"`
	ScheduleID int64          `json:"schedule_id"`
	JobType    string         `json:"job_type"`
	Status     string         `json:"status"`
	StartedAt  string         `json:"started_at"`
	FinishedAt string         `json:"finished_at,omitempty"`
	Result     map[string]any `json:"result"`
	Error      string         `json:"error,omitempty"`
}

// ListScheduleRuns returns run history for one schedule, most recent first.
//
// GET /kb/schedules/:id/runs
func ListScheduleRuns(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCH_060")
	defer rc.Close()
	logger := rc.GetLogger()

	id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_KB_SCH_061)"})
	}
	limit := parsePositiveInt(c.QueryParam("limit"), 50)

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "db not initialized (CWB_KB_SCH_062)"})
	}
	store := scheduler.SQLStore{DB: db}
	runs, err := store.ListRuns(c.Request().Context(), id, limit)
	if err != nil {
		logger.Error("list schedule runs failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "list schedule runs failed (CWB_KB_SCH_063)"})
	}
	dtos := make([]runDTO, 0, len(runs))
	for _, r := range runs {
		dto := runDTO{
			ID: r.ID, ScheduleID: r.ScheduleID, JobType: r.JobType, Status: r.Status,
			StartedAt: r.StartedAt.Format(rfc3339Layout), Result: r.Result, Error: r.Error,
		}
		if r.FinishedAt != nil {
			dto.FinishedAt = r.FinishedAt.Format(rfc3339Layout)
		}
		if dto.Result == nil {
			dto.Result = map[string]any{}
		}
		dtos = append(dtos, dto)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "runs": dtos})
}
