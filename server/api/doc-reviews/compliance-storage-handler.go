package docreviews

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
)

// HTTP handlers of compliance review storage (openspec compliance-review-storage). Each
// returns one bounded page or one record; none calls a model.

func complianceError(c echo.Context, op string, err error) error {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrComplianceNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrComplianceBadRequest):
		status = http.StatusBadRequest
	}
	logger.Warn("doc review compliance request failed", "operation", op, "path", c.Request().URL.Path, "status", status, "error", err)
	return c.JSON(status, map[string]any{"status": false, "error_msg": err.Error()})
}

func queryInt(c echo.Context, name string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(c.QueryParam(name)))
	return n
}

func pathID(c echo.Context, name string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: invalid %s %q", ErrComplianceBadRequest, name, c.Param(name))
	}
	return id, nil
}

// ListRunFindings handles GET /doc-review/runs/:run_id/findings?aspect=&limit=&cursor=&exclude_status=.
func ListRunFindings(c echo.Context) error {
	runID, err := pathID(c, "run_id")
	if err != nil {
		return complianceError(c, "list run findings", err)
	}
	var exclude []string
	for _, s := range strings.Split(c.QueryParam("exclude_status"), ",") {
		if s = strings.TrimSpace(s); s != "" {
			exclude = append(exclude, s)
		}
	}
	page, err := NewDocReviewController().ListRunFindingSummaries(c.Request().Context(), runID, c.QueryParam("aspect"), exclude,
		c.QueryParam("cursor"), queryInt(c, "limit"))
	if err != nil {
		return complianceError(c, "list run findings", err)
	}
	logger.Info("doc review finding summaries listed", "run_id", runID, "aspect", c.QueryParam("aspect"), "items", len(page.Items), "total", page.Total)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "items": page.Items, "total": page.Total, "next_cursor": page.NextCursor})
}

// GetFindingDetails handles GET /doc-review/findings/:id/details.
func GetFindingDetails(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "finding details", err)
	}
	d, err := NewDocReviewController().GetFindingDetails(c.Request().Context(), id, FindingDetailsOptions{
		ActivityLimit: queryInt(c, "activity_limit"), CandidateLimit: queryInt(c, "candidate_limit"),
		CandidateFilter: c.QueryParam("filter"), Language: c.QueryParam("language")})
	if err != nil {
		return complianceError(c, "finding details", err)
	}
	logger.Info("doc review finding details loaded", "finding_id", id, "legacy", d.Legacy != nil)
	return c.JSON(http.StatusOK, map[string]any{"status": true, "details": d})
}

// ListFindingActivities handles GET /doc-review/findings/:id/activities?cursor=&limit=.
func ListFindingActivities(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "finding activities", err)
	}
	page, err := NewDocReviewController().ListFindingActivities(c.Request().Context(), id, c.QueryParam("cursor"), queryInt(c, "limit"))
	if err != nil {
		return complianceError(c, "finding activities", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "items": page.Items, "next_cursor": page.NextCursor})
}

// ListFindingCandidates handles GET /doc-review/findings/:id/candidates?filter=&cursor=&limit=.
func ListFindingCandidates(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "finding candidates", err)
	}
	page, err := NewDocReviewController().ListFindingCandidates(c.Request().Context(), id, c.QueryParam("filter"), c.QueryParam("cursor"), queryInt(c, "limit"))
	if err != nil {
		return complianceError(c, "finding candidates", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "items": page.Items, "total": page.Total, "next_cursor": page.NextCursor})
}

// GetComplianceCandidate handles GET /doc-review/compliance/candidates/:id.
func GetComplianceCandidate(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "compliance candidate", err)
	}
	d, err := NewDocReviewController().GetComplianceCandidate(c.Request().Context(), id)
	if err != nil {
		return complianceError(c, "compliance candidate", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "candidate": d})
}

// ListFindingSearches handles GET /doc-review/findings/:id/searches.
func ListFindingSearches(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "finding searches", err)
	}
	searches, err := NewDocReviewController().ListFindingSearches(c.Request().Context(), id)
	if err != nil {
		return complianceError(c, "finding searches", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "items": searches})
}

// ListComplianceSearchResults handles GET /doc-review/compliance/searches/:id/results?cursor=&limit=.
func ListComplianceSearchResults(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "search results", err)
	}
	page, err := NewDocReviewController().ListSearchResults(c.Request().Context(), id, c.QueryParam("cursor"), queryInt(c, "limit"))
	if err != nil {
		return complianceError(c, "search results", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "items": page.Items, "next_cursor": page.NextCursor})
}

// ExportFinding handles GET /doc-review/findings/:id/export: the complete audit record.
func ExportFinding(c echo.Context) error {
	id, err := pathID(c, "id")
	if err != nil {
		return complianceError(c, "export finding", err)
	}
	record, err := NewDocReviewController().ExportFinding(c.Request().Context(), id)
	if err != nil {
		return complianceError(c, "export finding", err)
	}
	logger.Info("doc review finding exported", "finding_id", id)
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="finding-%d.json"`, id))
	return c.JSON(http.StatusOK, record)
}
