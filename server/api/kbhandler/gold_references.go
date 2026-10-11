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

// droppedReference is one candidate a gold reference run excluded or merged as
// a duplicate (testbed.ref_mentions_dropped), with the fields of its ledger entry.
type droppedReference struct {
	ID              int64    `json:"id"`
	CandidateID     string   `json:"candidate_id"`
	Decision        string   `json:"decision"`
	DropReason      string   `json:"drop_reason"`
	Reason          string   `json:"reason,omitempty"`
	CitedRaw        string   `json:"cited_raw,omitempty"`
	SourceLineSpans []string `json:"source_line_spans"`
	RefIDs          []string `json:"ref_ids"`
}

// goldReferenceIdentifiers renders testbed identifiers ({kind, value, edition})
// as the "kind: value" strings the production list uses, edition in brackets.
const goldReferenceIdentifiers = `COALESCE((SELECT jsonb_agg((e->>'kind') || ': ' || (e->>'value') ||
	COALESCE(' (' || NULLIF(e->>'edition', '') || ')', '') ORDER BY ord)
	FROM jsonb_array_elements(m.identifiers) WITH ORDINALITY AS x(e, ord)), '[]')::text`

// listGoldReferences answers GET /kb/references?source=testbed: the accepted
// and dropped references of one gold run, identified by skill_version,
// model_name and run_id, written by the extract-references-benchmark skill.
func listGoldReferences(c echo.Context, logger ApiTypes.JimoLogger, id int64) error {
	skillVersion := strings.TrimSpace(c.QueryParam("skill_version"))
	modelName := strings.TrimSpace(c.QueryParam("model_name"))
	runID := strings.TrimSpace(c.QueryParam("run_id"))
	if skillVersion == "" || modelName == "" || runID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "skill_version, model_name and run_id are required for source=testbed (CWB_KB_REF_011)"})
	}
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle
	rows, err := db.QueryContext(ctx, `
SELECT m.id, m.ref_id, m.section, m.source_line_spans::text, m.evidence, m.cited_raw, m.source_table_rows,
       m.target_type, m.target_subtype, m.target_title, m.target_version, m.target_content,
       m.normativity, m.list_kind, m.list_mismatch, `+goldReferenceIdentifiers+`,
       m.ref_form, m.mechanism, m.in_requirement, m.internal, m.confidence
FROM testbed.ref_mentions m
WHERE m.input_record_id = $1 AND m.skill_version = $2 AND m.model_name = $3 AND m.run_id = $4
ORDER BY m.id`, id, skillVersion, modelName, runID)
	if err != nil {
		logger.Error("list gold references failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list gold references (CWB_KB_REF_012)"})
	}
	defer rows.Close()
	results := make([]referenceMention, 0)
	for rows.Next() {
		var m referenceMention
		var sourceLineSpansJSON, identifiersJSON string
		var sourceTableRowsJSON []byte
		var inRequirement, internal bool
		var confidence float64
		if err := rows.Scan(&m.ID, &m.RefID, &m.Section, &sourceLineSpansJSON, &m.Evidence, &m.CitedRaw, &sourceTableRowsJSON,
			&m.TargetType, &m.TargetSubtype, &m.TargetTitle, &m.TargetVersion, &m.TargetContent,
			&m.Normativity, &m.ListKind, &m.ListMismatch, &identifiersJSON,
			&m.RefForm, &m.Mechanism, &inRequirement, &internal, &confidence); err != nil {
			logger.Error("scan gold references failed", "input_record_id", id, "run_id", runID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold references (CWB_KB_REF_013)"})
		}
		m.SourceTableRows = sourceTableRowsJSON
		m.InRequirement, m.Internal, m.Confidence = &inRequirement, &internal, &confidence
		if err := json.Unmarshal([]byte(sourceLineSpansJSON), &m.SourceLineSpans); err != nil {
			logger.Error("decode gold reference source lines failed", "input_record_id", id, "ref_id", m.RefID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold reference source lines (CWB_KB_REF_014)"})
		}
		if err := json.Unmarshal([]byte(identifiersJSON), &m.Identifiers); err != nil {
			logger.Error("decode gold reference identifiers failed", "input_record_id", id, "ref_id", m.RefID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold reference identifiers (CWB_KB_REF_015)"})
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate gold references failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold references (CWB_KB_REF_013)"})
	}

	dropped, err := listDroppedGoldReferences(ctx, db, id, skillVersion, modelName, runID)
	if err != nil {
		logger.Error("list dropped gold references failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list dropped gold references (CWB_KB_REF_016)"})
	}
	logger.Info("listed gold references", "input_record_id", id, "skill_version", skillVersion,
		"model_name", modelName, "run_id", runID, "accepted", len(results), "dropped", len(dropped))
	return c.JSON(http.StatusOK, listReferencesResponse{Status: true, Results: results, Dropped: dropped})
}

// listDroppedGoldReferences reads one gold run's dropped candidates; the
// candidate's reason, cited text, lines and merged ref IDs come from its ledger entry.
func listDroppedGoldReferences(ctx context.Context, db *sql.DB, id int64, skillVersion, modelName, runID string) ([]droppedReference, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, candidate_id, decision, drop_reason, candidate_data::text
FROM testbed.ref_mentions_dropped
WHERE input_record_id = $1 AND skill_version = $2 AND model_name = $3 AND run_id = $4
ORDER BY id`, id, skillVersion, modelName, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dropped := make([]droppedReference, 0)
	for rows.Next() {
		var d droppedReference
		var candidateJSON string
		if err := rows.Scan(&d.ID, &d.CandidateID, &d.Decision, &d.DropReason, &candidateJSON); err != nil {
			return nil, err
		}
		var candidate struct {
			Reason          string   `json:"reason"`
			CitedRaw        string   `json:"cited_raw"`
			SourceLineSpans []string `json:"source_line_spans"`
			RefIDs          []string `json:"ref_ids"`
		}
		if err := json.Unmarshal([]byte(candidateJSON), &candidate); err != nil {
			return nil, fmt.Errorf("decode candidate %s: %w", d.CandidateID, err)
		}
		d.Reason, d.CitedRaw = candidate.Reason, candidate.CitedRaw
		d.SourceLineSpans, d.RefIDs = candidate.SourceLineSpans, candidate.RefIDs
		if d.SourceLineSpans == nil {
			d.SourceLineSpans = []string{}
		}
		if d.RefIDs == nil {
			d.RefIDs = []string{}
		}
		dropped = append(dropped, d)
	}
	return dropped, rows.Err()
}
