package docreviews

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// HandleListAspects returns all review aspects.
func HandleListAspects(c echo.Context) error {
	aspects := ListAspects()
	return c.JSON(http.StatusOK, map[string]any{
		"status":  true,
		"aspects": aspects,
	})
}

// HandleListTiers returns tier definitions with aspect mappings.
func HandleListTiers(c echo.Context) error {
	tiers := ListTiers()
	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"tiers":  tiers,
	})
}

// SubmitRequest accepts a review request and publishes a JetStream event so
// doc-processor runs the review asynchronously.
func SubmitRequest(c echo.Context) error {
	ctrl := NewDocReviewController()

	var input SubmitRequestInput
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":    false,
			"error_msg": "Invalid request body: " + err.Error(),
		})
	}

	rc := EchoFactory.NewFromEcho(c, "CWB_DRH_001")
	userInfo := rc.IsAuthenticated()
	if userInfo != nil {
		if input.RequesterName == "" {
			input.RequesterName = userInfo.UserName
		}
	}

	ctx := c.Request().Context()

	result, err := ctrl.AcceptRequest(ctx, input)
	if err != nil {
		if re, ok := err.(*RequestError); ok {
			return c.JSON(re.Status, map[string]any{
				"status":    false,
				"error_msg": re.Message,
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":    false,
			"error_msg": err.Error(),
		})
	}

	// Publish a JetStream event so doc-processor runs the review. If the
	// publisher is unavailable, fall back to an inline goroutine so the service
	// degrades gracefully rather than failing hard.
	if pubErr := PublishReviewEvent(ctx, result.RequestID); pubErr != nil {
		logger.Warn("JetStream publish failed; falling back to inline goroutine",
			"request_id", result.RequestID, "error", pubErr)
		requestID := result.RequestID
		go func() {
			bgCtrl := NewDocReviewController()
			bgCtrl.RunReviewAndReport(c.Request().Context(), requestID)
		}()
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":     true,
		"request_id": result.RequestID,
		"status_str": "accepted",
	})
}

// ListActiveJobs returns all review jobs with ≥1 unfinished aspect (DR15).
func ListActiveJobs(c echo.Context) error {
	ctrl := NewDocReviewController()
	jobs, err := ctrl.ListActiveJobs(c.Request().Context())
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": err.Error()})
	}
	if jobs == nil {
		jobs = []ActiveJob{}
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "jobs": jobs})
}

// ListRequests returns document-review requests matching the query filters,
// newest first. Drives the request list (with its Search dialog) in the GUI.
func ListRequests(c echo.Context) error {
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	filter := RequestListFilter{
		RequestID:     c.QueryParam("request_id"),
		DocTitle:      c.QueryParam("title"),
		RequesterName: c.QueryParam("requester"),
		Tier:          c.QueryParam("tier"),
		Status:        c.QueryParam("status"),
		CreateStart:   c.QueryParam("create_start"),
		CreateEnd:     c.QueryParam("create_end"),
		Limit:         limit,
	}
	ctrl := NewDocReviewController()
	items, err := ctrl.ListRequests(c.Request().Context(), filter)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": err.Error()})
	}
	if items == nil {
		items = []RequestListItem{}
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "requests": items})
}

func parseID(c echo.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

// GetRequest returns request status + findings.
func GetRequest(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := NewDocReviewController()
	result, err := ctrl.GetRequestWithFindings(c.Request().Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		return c.JSON(status, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{
		"status":          true,
		"request":         result.Request,
		"findings":        result.Findings,
		"aspect_statuses": result.AspectStatuses,
	})
}

// GetReport returns the full report JSON.
func GetReport(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	gen := NewDocReviewReportGenerator()
	report, err := gen.GetReport(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "report": report})
}

// GetReportHTML returns the HTML-rendered report.
func GetReportHTML(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	gen := NewDocReviewReportGenerator()
	html, err := gen.GetReportHTML(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.HTML(http.StatusOK, html)
}

// ExportReport returns the report in the requested format.
func ExportReport(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	gen := NewDocReviewReportGenerator()
	report, err := gen.GetReport(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": err.Error()})
	}

	format := c.QueryParam("format")
	switch format {
	case "md", "markdown":
		return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(report.ReportMarkdown))
	case "pdf":
		return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(report.ReportMarkdown))
	default:
		reportJSON, _ := json.Marshal(report.ReportJSON)
		return c.Blob(http.StatusOK, "application/json", reportJSON)
	}
}

// UpdateFinding updates a finding's review_status.
func UpdateFinding(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	var body struct {
		ReviewStatus string `json:"review_status"`
		ReviewedBy   string `json:"reviewed_by"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid body"})
	}
	ctrl := NewDocReviewController()
	if err := ctrl.UpdateFinding(c.Request().Context(), id, body.ReviewStatus, body.ReviewedBy); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// AutoFixFinding runs the LLM auto-fix for a finding, editing the line-file.
// A 200 with fixable=false (not an HTTP error) means the GUI should prompt the
// user (e.g. unfixable, no location, or no model configured).
func AutoFixFinding(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := NewDocReviewController()
	res, err := ctrl.AutoFixFinding(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "result": res})
}

// GetFindingLines returns the current content of the finding's offending line(s)
// so the Edit Tool dialog can display them.
func GetFindingLines(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := NewDocReviewController()
	lines, err := ctrl.GetFindingLines(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": err.Error()})
	}
	if lines == nil {
		lines = []LineEdit{}
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "lines": lines})
}

// EditFindingLines saves user-edited line content (the Edit Tool "Save" action).
func EditFindingLines(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	var body struct {
		Lines []LineEdit `json:"lines"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid body"})
	}
	ctrl := NewDocReviewController()
	changed, err := ctrl.ApplyFindingEdit(c.Request().Context(), id, body.Lines)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "changed": changed})
}

// RegenerateReport rebuilds a report's JSON/markdown and PDF from current findings.
func RegenerateReport(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := NewDocReviewController()
	if err := ctrl.RegenerateReport(c.Request().Context(), id); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		return c.JSON(status, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// StopRequest stops a running review.
func StopRequest(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := NewDocReviewController()
	if err := ctrl.StopRequest(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
