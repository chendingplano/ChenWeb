package docreviewhandler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/docreview"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// ListAspects returns all review aspects.
func ListAspects(c echo.Context) error {
	aspects := docreview.ListAspects()
	return c.JSON(http.StatusOK, map[string]any{
		"status":  true,
		"aspects": aspects,
	})
}

// ListTiers returns tier definitions with aspect mappings.
func ListTiers(c echo.Context) error {
	tiers := docreview.ListTiers()
	return c.JSON(http.StatusOK, map[string]any{
		"status": true,
		"tiers":  tiers,
	})
}

func submitRequestHelper(c echo.Context, synchronous bool) error {
	ctrl := docreview.NewDocReviewController()

	var input docreview.SubmitRequestInput
	if err := c.Bind(&input); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status":    false,
			"error_msg": "Invalid request body: " + err.Error(),
		})
	}

	// Extract authenticated user info for requester fields.
	rc := EchoFactory.NewFromEcho(c, "CWB_DRH_001")
	userInfo := rc.IsAuthenticated()
	if userInfo != nil {
		if input.RequesterName == "" {
			input.RequesterName = userInfo.UserName
		}
	}

	ctx := c.Request().Context()

	// Accept the request (validates and persists).
	result, err := ctrl.AcceptRequest(ctx, input)
	if err != nil {
		if re, ok := err.(*docreview.RequestError); ok {
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

	// Run the review synchronously (the controller handles the lifecycle).
	if err := ctrl.RunReview(ctx, result.RequestID); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status":    false,
			"error_msg": err.Error(),
		})
	}

	// Generate report.
	req, _ := ctrl.GetRequest(ctx, result.RequestID)
	if req != nil && req.Status == "completed" {
		wf, _ := ctrl.GetRequestWithFindings(ctx, result.RequestID)
		gen := docreview.NewDocReviewReportGenerator()
		report, buildErr := gen.Build(ctx, &wf.Request, wf.Findings)
		if buildErr == nil {
			reportID, persistErr := gen.Persist(ctx, &wf.Request, report)
			if persistErr == nil {
				result.ReviewRunID = req.ReviewRunID
				return c.JSON(http.StatusOK, map[string]any{
					"status":     true,
					"request_id": result.RequestID,
					"report_id":  reportID,
				})
			}
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":     true,
		"request_id": result.RequestID,
	})
}

// SubmitRequest creates and runs a review request synchronously.
func SubmitRequest(c echo.Context) error {
	return submitRequestHelper(c, true)
}

// parseID extracts an int64 path parameter.
func parseID(c echo.Context, name string) (int64, error) {
	return strconv.ParseInt(c.Param(name), 10, 64)
}

// GetRequest returns request status + findings.
func GetRequest(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := docreview.NewDocReviewController()
	result, err := ctrl.GetRequestWithFindings(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "request": result.Request, "findings": result.Findings})
}

// GetReport returns the full report JSON.
func GetReport(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	gen := docreview.NewDocReviewReportGenerator()
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
	gen := docreview.NewDocReviewReportGenerator()
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
	gen := docreview.NewDocReviewReportGenerator()
	report, err := gen.GetReport(c.Request().Context(), id)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]any{"status": false, "error_msg": err.Error()})
	}

	format := c.QueryParam("format")
	switch format {
	case "md", "markdown":
		return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(report.ReportMarkdown))
	case "pdf":
		// Deferred — return markdown for now.
		return c.Blob(http.StatusOK, "text/markdown; charset=utf-8", []byte(report.ReportMarkdown))
	default:
		// Return JSON.
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
	ctrl := docreview.NewDocReviewController()
	if err := ctrl.UpdateFinding(c.Request().Context(), id, body.ReviewStatus, body.ReviewedBy); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// StopRequest stops a running review.
func StopRequest(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": "Invalid ID"})
	}
	ctrl := docreview.NewDocReviewController()
	if err := ctrl.StopRequest(c.Request().Context(), id); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{"status": false, "error_msg": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
