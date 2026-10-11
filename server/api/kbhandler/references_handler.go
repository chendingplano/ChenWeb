package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type referenceMention struct {
	ID               int64           `json:"id"`
	RefID            string          `json:"ref_id"`
	Section          string          `json:"section,omitempty"`
	SourceLineSpans  []string        `json:"source_line_spans"`
	SourceTableRows  json.RawMessage `json:"source_table_rows,omitempty"`
	Evidence         string          `json:"evidence,omitempty"`
	CitedRaw         string          `json:"cited_raw,omitempty"`
	TargetType       string          `json:"target_type"`
	TargetSubtype    string          `json:"target_subtype"`
	TargetTitle      string          `json:"target_title,omitempty"`
	TargetVersion    string          `json:"target_version,omitempty"`
	TargetContent    string          `json:"target_content"`
	Normativity      string          `json:"normativity"`
	ListKind         string          `json:"list_kind,omitempty"`
	ListMismatch     string          `json:"list_mismatch,omitempty"`
	R1State          string          `json:"r1_state,omitempty"`
	R2State          string          `json:"r2_state,omitempty"`
	TargetInputID    *int64          `json:"target_input_id,omitempty"`
	TargetInputTitle string          `json:"target_input_title,omitempty"`
	MetricState      string          `json:"metric_state,omitempty"`
	ResourceTitle    string          `json:"resource_title,omitempty"`
	IdentityKey      string          `json:"identity_key,omitempty"`
	Identifiers      []string        `json:"identifiers"`
	// Gold-only fields (source=testbed); production rows leave them empty.
	RefForm       string   `json:"ref_form,omitempty"`
	Mechanism     string   `json:"mechanism,omitempty"`
	InRequirement *bool    `json:"in_requirement,omitempty"`
	Internal      *bool    `json:"internal,omitempty"`
	Confidence    *float64 `json:"confidence,omitempty"`
}

type listReferencesResponse struct {
	Status  bool               `json:"status"`
	Results []referenceMention `json:"results"`
	// Dropped holds a gold run's excluded/duplicate candidates (source=testbed only).
	Dropped []droppedReference `json:"dropped,omitempty"`
}

// ListReferences returns the references extracted from one input record, or
// with source=testbed the gold references of one benchmark run.
func ListReferences(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_REF_001")
	defer rc.Close()
	id, err := strconv.ParseInt(strings.TrimSpace(c.QueryParam("input_record_id")), 10, 64)
	if err != nil || id <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "valid input_record_id is required"})
	}
	if strings.TrimSpace(c.QueryParam("source")) == "testbed" {
		return listGoldReferences(c, rc.GetLogger(), id)
	}
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
SELECT m.id, m.ref_id, COALESCE(m.section,''), COALESCE(m.source_line_spans, '[]'::jsonb)::text,
       COALESCE(m.evidence,''), COALESCE(m.cited_raw,''), m.source_table_rows,
       m.target_type, m.target_subtype, COALESCE(m.target_title,''), COALESCE(m.target_version,''),
       m.target_content, m.normativity, COALESCE(m.list_kind,''), COALESCE(m.list_mismatch,''),
       COALESCE(m.r1_state,''), COALESCE(m.r2_state,''), m.target_input_id,
       COALESCE(NULLIF(BTRIM(i.doc_metadata->>'title'),''), i.title, i.staging_filename, ''),
       COALESCE(m.metric_state,''), COALESCE(r.title,''), COALESCE(r.identity_key,''),
		COALESCE((SELECT jsonb_agg(ri.kind || ': ' || ri.value ORDER BY ri.kind, ri.value)::text
		          FROM kb.ref_resource_identifiers ri WHERE ri.resource_id = r.id), '[]')
FROM kb.ref_mentions m
LEFT JOIN kb.ref_resources r ON r.id = m.resource_id
LEFT JOIN kb.inputs i ON i.id = m.target_input_id
WHERE m.input_record_id = $1
ORDER BY m.id`, id)
	if err != nil {
		rc.GetLogger().Error("list references failed", "input_record_id", id, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list references"})
	}
	defer rows.Close()
	results := make([]referenceMention, 0)
	for rows.Next() {
		var m referenceMention
		var sourceLineSpansJSON string
		var identifiersJSON string
		var sourceTableRowsJSON []byte // []byte accepts SQL NULL; json.RawMessage does not.
		if err := rows.Scan(&m.ID, &m.RefID, &m.Section, &sourceLineSpansJSON, &m.Evidence, &m.CitedRaw, &sourceTableRowsJSON, &m.TargetType, &m.TargetSubtype,
			&m.TargetTitle, &m.TargetVersion, &m.TargetContent, &m.Normativity, &m.ListKind, &m.ListMismatch,
			&m.R1State, &m.R2State, &m.TargetInputID, &m.TargetInputTitle, &m.MetricState, &m.ResourceTitle,
			&m.IdentityKey, &identifiersJSON); err != nil {
			rc.GetLogger().Error("scan references failed", "input_record_id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read references"})
		}
		m.SourceTableRows = sourceTableRowsJSON
		if err := json.Unmarshal([]byte(sourceLineSpansJSON), &m.SourceLineSpans); err != nil {
			rc.GetLogger().Error("decode reference source lines failed", "input_record_id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read reference source lines"})
		}
		if err := json.Unmarshal([]byte(identifiersJSON), &m.Identifiers); err != nil {
			rc.GetLogger().Error("decode reference identifiers failed", "input_record_id", id, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read reference identifiers"})
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read references"})
	}
	return c.JSON(http.StatusOK, listReferencesResponse{Status: true, Results: results})
}
