package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// droppedProvision is one candidate a gold provision run excluded or merged as
// a duplicate (testbed.provisions_dropped), with the fields of its ledger entry.
type droppedProvision struct {
	ID              int64    `json:"id"`
	CandidateID     string   `json:"candidate_id"`
	Decision        string   `json:"decision"`
	DropReason      string   `json:"drop_reason"`
	Reason          string   `json:"reason,omitempty"`
	Text            string   `json:"text,omitempty"`
	SourceLineSpans []string `json:"source_line_spans"`
	ProvIDs         []string `json:"prov_ids"`
}

// listGoldProvisions answers GET /kb/provisions?source=testbed: the accepted
// and dropped provisions of one gold run, identified by skill_version,
// model_name and run_id, written by the extract-provisions-benchmark skill.
func listGoldProvisions(c echo.Context, logger ApiTypes.JimoLogger, id int64) error {
	skillVersion := strings.TrimSpace(c.QueryParam("skill_version"))
	modelName := strings.TrimSpace(c.QueryParam("model_name"))
	runID := strings.TrimSpace(c.QueryParam("run_id"))
	if skillVersion == "" || modelName == "" || runID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "skill_version, model_name and run_id are required for source=testbed (CWB_KB_PROV_G_011)"})
	}
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle
	rows, err := db.QueryContext(ctx, `
SELECT p.id, p.input_record_id, p.prov_id, p.prov_name, p.prov_name_en, p.provision_type, p.provision_kind,
       p.source_line_spans::text, p.source_table_rows::text, p.provision, p.provision_en,
       p.provision_subject, p.provision_subject_en, p.location_type, p.confidence, p.is_explicit,
       p.section, p.evidence, p.model_name,
       COALESCE(to_char(p.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '')
FROM testbed.provisions p
WHERE p.input_record_id = $1 AND p.skill_version = $2 AND p.model_name = $3 AND p.run_id = $4
ORDER BY (substring(p.prov_id from '\d+$'))::int`, id, skillVersion, modelName, runID)
	if err != nil {
		logger.Error("list gold provisions failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list gold provisions (CWB_KB_PROV_G_012)"})
	}
	defer rows.Close()
	results := make([]provisionRecordJSON, 0)
	for rows.Next() {
		var r provisionRecordJSON
		var name, nameEn, provisionType, provision, provisionEn, subject, subjectEn, location, rowModel sql.NullString
		var spans, tableRows string
		var confidence float64
		var explicit bool
		if err := rows.Scan(&r.ID, &r.InputRecordID, &r.ProvID, &name, &nameEn, &provisionType, &r.ProvisionKind,
			&spans, &tableRows, &provision, &provisionEn, &subject, &subjectEn, &location, &confidence, &explicit,
			&r.Section, &r.Evidence, &rowModel, &r.CreatedAt); err != nil {
			logger.Error("scan gold provisions failed", "input_record_id", id, "run_id", runID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold provisions (CWB_KB_PROV_G_013)"})
		}
		r.ProvName, r.ProvNameEn, r.ProvisionType = nullableString(name), nullableString(nameEn), nullableString(provisionType)
		r.Provision, r.ProvisionEn = nullableString(provision), nullableString(provisionEn)
		r.ProvisionSubject, r.ProvisionSubjectEn = nullableString(subject), nullableString(subjectEn)
		r.LocationType, r.ModelName = nullableString(location), nullableString(rowModel)
		r.SourceLineSpans, r.SourceTableRows = json.RawMessage(spans), json.RawMessage(tableRows)
		r.Confidence, r.IsExplicit = &confidence, &explicit
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate gold provisions failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold provisions (CWB_KB_PROV_G_013)"})
	}

	dropped, err := listDroppedGoldProvisions(ctx, db, id, skillVersion, modelName, runID)
	if err != nil {
		logger.Error("list dropped gold provisions failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list dropped gold provisions (CWB_KB_PROV_G_014)"})
	}
	logger.Info("listed gold provisions", "input_record_id", id, "skill_version", skillVersion,
		"model_name", modelName, "run_id", runID, "accepted", len(results), "dropped", len(dropped))
	return c.JSON(http.StatusOK, listProvisionsResponse{Status: true, Results: results, Total: len(results), Dropped: dropped})
}

// nullableString maps SQL NULL to nil and anything else to a pointer.
func nullableString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}

// listDroppedGoldProvisions reads one gold run's dropped candidates; the
// candidate's reason, text, lines and merged prov IDs come from its ledger entry.
func listDroppedGoldProvisions(ctx context.Context, db *sql.DB, id int64, skillVersion, modelName, runID string) ([]droppedProvision, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, candidate_id, decision, drop_reason, candidate_data::text
FROM testbed.provisions_dropped
WHERE input_record_id = $1 AND skill_version = $2 AND model_name = $3 AND run_id = $4
ORDER BY id`, id, skillVersion, modelName, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dropped := make([]droppedProvision, 0)
	for rows.Next() {
		var d droppedProvision
		var candidateJSON string
		if err := rows.Scan(&d.ID, &d.CandidateID, &d.Decision, &d.DropReason, &candidateJSON); err != nil {
			return nil, err
		}
		var candidate struct {
			Reason          string   `json:"reason"`
			Text            string   `json:"text"`
			SourceLineSpans []string `json:"source_line_spans"`
			ProvIDs         []string `json:"prov_ids"`
		}
		if err := json.Unmarshal([]byte(candidateJSON), &candidate); err != nil {
			return nil, fmt.Errorf("decode candidate %s: %w", d.CandidateID, err)
		}
		d.Reason, d.Text = candidate.Reason, candidate.Text
		d.SourceLineSpans, d.ProvIDs = candidate.SourceLineSpans, candidate.ProvIDs
		if d.SourceLineSpans == nil {
			d.SourceLineSpans = []string{}
		}
		if d.ProvIDs == nil {
			d.ProvIDs = []string{}
		}
		dropped = append(dropped, d)
	}
	return dropped, rows.Err()
}
