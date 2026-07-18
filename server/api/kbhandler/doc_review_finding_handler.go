package kbhandler

import (
	"encoding/json"
	"net/http"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type docReviewFindingRow struct {
	ID            int64           `json:"id"`
	InputRecordID int64           `json:"input_record_id"`
	RunID         int64           `json:"run_id"`
	Pass          string          `json:"pass"`
	Aspect        string          `json:"aspect"`
	Severity      string          `json:"severity"`
	FindingType   string          `json:"finding_type"`
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Evidence      string          `json:"evidence"`
	Location      string          `json:"location"`
	Suggestion    string          `json:"suggestion"`
	Confidence    float64         `json:"confidence"`
	ReviewStatus  string          `json:"review_status"`
	ArtifactID    string          `json:"artifact_id"`
	Metadata      json.RawMessage `json:"metadata"`
	ReferenceDoc  json.RawMessage `json:"reference_doc"`
}

type listDocReviewFindingsResponse struct {
	Status   bool                  `json:"status"`
	Results  []docReviewFindingRow `json:"results"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	Total    int64                 `json:"total"`
}

// ListDocReviewFindings handles GET /api/v1/kb/doc-review-findings.
func ListDocReviewFindings(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_DRF_001")
	defer rc.Close()
	logger := rc.GetLogger()

	page, err := parseDocReviewLogPage(c.QueryParam("page"), 1)
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'page' must be an integer (CWB_DRF_001)")
	}
	pageSize, err := parseDocReviewLogPage(c.QueryParam("page_size"), defaultPageSize)
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'page_size' must be an integer (CWB_DRF_002)")
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	inputRecordID, err := parseOptionalInt64(c.QueryParam("input_record_id"))
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'input_record_id' must be an integer (CWB_DRF_003)")
	}
	runID, err := parseOptionalInt64(c.QueryParam("run_id"))
	if err != nil {
		return docReviewLogBadRequest(c, "query param 'run_id' must be an integer (CWB_DRF_004)")
	}

	filter := docprocessing.DocReviewFindingFilter{
		InputRecordID: inputRecordID,
		RunID:         runID,
		Pass:          c.QueryParam("pass"),
		Aspect:        c.QueryParam("aspect"),
		Severity:      c.QueryParam("severity"),
		ReviewStatus:  c.QueryParam("review_status"),
		FindingType:   c.QueryParam("finding_type"),
		ArtifactID:    c.QueryParam("artifact_id"),
		Title:         c.QueryParam("title"),
		Page:          page,
		PageSize:      pageSize,
	}

	logger.Info("list doc review findings",
		"input_record_id", filter.InputRecordID,
		"run_id", filter.RunID,
		"pass", filter.Pass,
		"aspect", filter.Aspect,
		"severity", filter.Severity,
		"review_status", filter.ReviewStatus,
		"finding_type", filter.FindingType,
		"artifact_id", filter.ArtifactID,
		"title", filter.Title,
		"page", page,
		"page_size", pageSize,
	)

	rows, total, err := (docprocessing.DocReviewFindingSQLStore{DB: ApiTypes.ProjectDBHandle}).ListDocReviewFindings(c.Request().Context(), filter)
	if err != nil {
		logger.Error("list doc review findings failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list doc review findings (CWB_DRF_010)"})
	}

	results := make([]docReviewFindingRow, 0, len(rows))
	for _, row := range rows {
		results = append(results, docReviewFindingRow{
			ID:            row.ID,
			InputRecordID: row.InputRecordID,
			RunID:         row.RunID,
			Pass:          row.Pass,
			Aspect:        row.Aspect,
			Severity:      row.Severity,
			FindingType:   row.FindingType,
			Title:         row.Title,
			Description:   row.Description,
			Evidence:      row.Evidence,
			Location:      row.Location,
			Suggestion:    row.Suggestion,
			Confidence:    row.Confidence,
			ReviewStatus:  row.ReviewStatus,
			ArtifactID:    row.ArtifactID,
			Metadata:      row.Metadata,
			ReferenceDoc:  row.ReferenceDoc,
		})
	}

	return c.JSON(http.StatusOK, listDocReviewFindingsResponse{
		Status:   true,
		Results:  results,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	})
}
