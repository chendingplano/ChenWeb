package kbhandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// Candidate display text is the current index text, not a frozen gold quotation.
// The document/run pair is mandatory: a document can have many historical runs.
const goldComplianceQuery = `SELECT jsonb_build_object(
 'run', to_jsonb(r),
 'activities', COALESCE((SELECT jsonb_agg(to_jsonb(a) || jsonb_build_object(
   'judgements', COALESCE((SELECT jsonb_agg(to_jsonb(j) || jsonb_build_object(
     'source_title', COALESCE(sa.source_title, ''),
     'passage', COALESCE(NULLIF(sa.snippet_basis, ''), sa.search_document, ''),
     'source_line_spans', COALESCE(sa.source_line_spans, '[]'::jsonb),
     'source_available', sa.artifact_id IS NOT NULL
   ) ORDER BY j.id)
   FROM testbed.compliance_gold_judgements j
   LEFT JOIN kb.search_artifacts sa ON sa.artifact_type = j.artifact_type AND sa.artifact_id = j.artifact_id
     AND sa.input_record_id = j.candidate_record
   WHERE j.activity_id = a.id), '[]'::jsonb)
 ) ORDER BY a.line_start, a.id)
 FROM testbed.compliance_gold_activities a WHERE a.run_id = r.id AND a.input_record_id = $1), '[]'::jsonb)
) FROM testbed.compliance_gold_runs r WHERE r.run_key = $2
 AND EXISTS (SELECT 1 FROM testbed.compliance_gold_activities a WHERE a.run_id = r.id AND a.input_record_id = $1)`

var complianceRunKeyPattern = regexp.MustCompile(`^[0-9]{8}_[0-9]{6}$`)

// ListGoldCompliance is protected by the benchmark admin middleware in routes.go.
func ListGoldCompliance(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "20261114-683")
	defer rc.Close()
	recordID, err := strconv.ParseInt(strings.TrimSpace(c.QueryParam("input_record_id")), 10, 64)
	key := strings.TrimSpace(c.QueryParam("run_id"))
	if err != nil || recordID <= 0 || !complianceRunKeyPattern.MatchString(key) {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "valid input_record_id and run_id are required"})
	}
	var payload []byte
	err = ApiTypes.ProjectDBHandle.QueryRowContext(c.Request().Context(), goldComplianceQuery, recordID, key).Scan(&payload)
	if err == sql.ErrNoRows {
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "gold compliance run not found for this document"})
	}
	if err != nil {
		rc.GetLogger().Error("read compliance gold failed", "record_id", recordID, "run_id", key, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read compliance gold"})
	}
	rc.GetLogger().Info("read compliance gold", "record_id", recordID, "run_id", key)
	return c.JSON(http.StatusOK, struct {
		Status bool            `json:"status"`
		Data   json.RawMessage `json:"data"`
	}{true, payload})
}
