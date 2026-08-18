package kbhandler

import (
	"net/http"
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// retryQueueJob is the API shape for one kb.semantic_retry_queue row, per
// ADR 2026081801 task 5.2's "retry tooling" diagnostic reader. It is a plain
// read model over semantic.RetryJob: this queue has no accepted/represented
// distinction, so unlike semantic-assertions there is no governance filter to
// preserve here.
type retryQueueJob struct {
	ID                          int64   `json:"id"`
	OutcomeID                   int64   `json:"outcome_id"`
	FindingID                   *int64  `json:"finding_id,omitempty"`
	TargetDependencyFingerprint string  `json:"target_dependency_fingerprint"`
	SourceInputFingerprint      string  `json:"source_input_fingerprint,omitempty"`
	State                       string  `json:"state"`
	Attempts                    int     `json:"attempts"`
	LeaseToken                  string  `json:"lease_token,omitempty"`
	LeaseExpiresAt              *string `json:"lease_expires_at,omitempty"`
	LastError                   string  `json:"last_error,omitempty"`
	CreateTime                  string  `json:"create_time"`
	ModifyTime                  string  `json:"modify_time"`
	OutcomeInputRecordID        *int64  `json:"outcome_input_record_id,omitempty"`
	OutcomeArtifactType         string  `json:"outcome_artifact_type,omitempty"`
	OutcomeArtifactID           string  `json:"outcome_artifact_id,omitempty"`
	OutcomeStageTermID          string  `json:"outcome_stage_term_id,omitempty"`
}

func toRetryQueueJob(j semantic.RetryJob) retryQueueJob {
	out := retryQueueJob{
		ID: j.ID, OutcomeID: j.OutcomeID, FindingID: j.FindingID,
		TargetDependencyFingerprint: j.TargetDependencyFingerprint,
		SourceInputFingerprint:      j.SourceInputFingerprint,
		State:                       j.State, Attempts: j.Attempts,
		LeaseToken: j.LeaseToken, LastError: j.LastError,
		CreateTime: j.CreateTime.Format(timeLayout), ModifyTime: j.ModifyTime.Format(timeLayout),
		OutcomeInputRecordID: j.OutcomeInputRecordID,
		OutcomeArtifactType:  j.OutcomeArtifactType,
		OutcomeArtifactID:    j.OutcomeArtifactID,
		OutcomeStageTermID:   j.OutcomeStageTermID,
	}
	if j.LeaseExpiresAt != nil {
		s := j.LeaseExpiresAt.Format(timeLayout)
		out.LeaseExpiresAt = &s
	}
	return out
}

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

type semanticRetryQueueListResponse struct {
	Status   bool            `json:"status"`
	Results  []retryQueueJob `json:"results"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int64           `json:"total"`
}

// ListSemanticRetryQueue serves GET /kb/semantic-retry-queue, the read-only
// diagnostic reader over kb.semantic_retry_queue (task 5.2 "retry tooling").
func ListSemanticRetryQueue(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SRQ_001")
	defer rc.Close()
	page, _ := strconv.Atoi(c.QueryParam("page"))
	size, _ := strconv.Atoi(c.QueryParam("page_size"))
	var outcomeID int64
	if value, err := strconv.ParseInt(c.QueryParam("outcome_id"), 10, 64); err == nil && value > 0 {
		outcomeID = value
	}
	queue := semantic.RetryQueue{DB: ApiTypes.ProjectDBHandle}
	rows, total, err := queue.List(c.Request().Context(), semantic.RetryJobFilter{
		State: c.QueryParam("state"), OutcomeID: outcomeID, Page: page, PageSize: size,
	})
	if err != nil {
		rc.GetLogger().Error("list semantic retry queue failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list semantic retry queue"})
	}
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	results := make([]retryQueueJob, 0, len(rows))
	for _, j := range rows {
		results = append(results, toRetryQueueJob(j))
	}
	return c.JSON(http.StatusOK, semanticRetryQueueListResponse{Status: true, Results: results, Page: page, PageSize: size, Total: total})
}
