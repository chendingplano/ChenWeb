package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

// droppedProduct is one candidate a gold product run excluded or merged as a
// duplicate (testbed.products_dropped), with the fields of its ledger entry.
type droppedProduct struct {
	ID              int64    `json:"id"`
	CandidateID     string   `json:"candidate_id"`
	Decision        string   `json:"decision"`
	DropReason      string   `json:"drop_reason"`
	Reason          string   `json:"reason,omitempty"`
	Text            string   `json:"text,omitempty"`
	SourceLineSpans []string `json:"source_line_spans"`
	ProductKeys     []string `json:"product_keys"`
}

// listGoldProducts answers GET /kb/products?source=testbed: the gold product
// relations and dropped candidates of one gold run, identified by skill_version,
// model_name and run_id, written by the extract-products-benchmark skill. Gold
// rows use the kb.products shape so the Products view renders them unchanged.
func listGoldProducts(c echo.Context, logger ApiTypes.JimoLogger, id int64, fileName string) error {
	skillVersion := strings.TrimSpace(c.QueryParam("skill_version"))
	modelName := strings.TrimSpace(c.QueryParam("model_name"))
	runID := strings.TrimSpace(c.QueryParam("run_id"))
	if skillVersion == "" || modelName == "" || runID == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "skill_version, model_name and run_id are required for source=testbed (CWB_KB_PR_G_011)"})
	}
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle
	rows, err := db.QueryContext(ctx, `
SELECT p.id, p.product_rel_id, p.product_key, p.section, p.rule_id, p.product_name,
       COALESCE(p.product_name_en, ''), p.canonical_name, COALESCE(p.canonical_name_en, ''),
       p.product_type, p.relation_type, p.relation_summary, p.relation_summary_en, p.evidence_quote,
       p.evidence_lines::text, p.obligation_level, p.requirement_text, p.requirement_text_en,
       p.conditions::text, p.exceptions::text, p.parameters::text, p.related_products::text,
       p.responsible_actor, p.confidence, p.model_name, p.created_at
FROM testbed.products p
WHERE p.input_record_id = $1 AND p.skill_version = $2 AND p.model_name = $3 AND p.run_id = $4
ORDER BY (substring(p.product_rel_id from '\d+$'))::int`, id, skillVersion, modelName, runID)
	if err != nil {
		logger.Error("list gold products failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list gold products (CWB_KB_PR_G_012)"})
	}
	defer rows.Close()
	results := make([]productRecord, 0)
	for rows.Next() {
		var r productRecord
		var lines, conditions, exceptions, parameters, related string
		var created time.Time
		if err := rows.Scan(&r.ID, &r.ProductRelID, &r.ProductKey, &r.Section, &r.RuleID, &r.ProductName,
			&r.ProductNameEn, &r.CanonicalName, &r.CanonicalNameEn, &r.ProductType, &r.RelationType,
			&r.RelationSummary, &r.RelationSummaryEn, &r.EvidenceQuote, &lines, &r.ObligationLevel,
			&r.RequirementText, &r.RequirementTextEn, &conditions, &exceptions, &parameters, &related,
			&r.ResponsibleActor, &r.Confidence, &r.ModelName, &created); err != nil {
			logger.Error("scan gold products failed", "input_record_id", id, "run_id", runID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold products (CWB_KB_PR_G_013)"})
		}
		r.EvidenceLines = jsonArrayOrEmpty([]byte(lines))
		r.Conditions = jsonArrayOrEmpty([]byte(conditions))
		r.Exceptions = jsonArrayOrEmpty([]byte(exceptions))
		r.Parameters = jsonArrayOrEmpty([]byte(parameters))
		r.RelatedProducts = jsonArrayOrEmpty([]byte(related))
		r.CreateTime = created.Format(time.RFC3339)
		r.ModifyTime = r.CreateTime
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate gold products failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold products (CWB_KB_PR_G_013)"})
	}

	dropped, err := listDroppedGoldProducts(ctx, db, id, skillVersion, modelName, runID)
	if err != nil {
		logger.Error("list dropped gold products failed", "input_record_id", id, "run_id", runID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list dropped gold products (CWB_KB_PR_G_014)"})
	}
	logger.Info("listed gold products", "input_record_id", id, "skill_version", skillVersion,
		"model_name", modelName, "run_id", runID, "rows", len(results), "dropped", len(dropped))
	return c.JSON(http.StatusOK, listProductsResponse{Status: true, InputID: id, FileName: fileName,
		Results: results, Total: len(results), Dropped: dropped})
}

// listDroppedGoldProducts reads one gold run's dropped candidates; the
// candidate's reason, text, lines and product keys come from its ledger entry.
func listDroppedGoldProducts(ctx context.Context, db *sql.DB, id int64, skillVersion, modelName, runID string) ([]droppedProduct, error) {
	rows, err := db.QueryContext(ctx, `
SELECT id, candidate_id, decision, drop_reason, candidate_data::text
FROM testbed.products_dropped
WHERE input_record_id = $1 AND skill_version = $2 AND model_name = $3 AND run_id = $4
ORDER BY id`, id, skillVersion, modelName, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dropped := make([]droppedProduct, 0)
	for rows.Next() {
		var d droppedProduct
		var candidateJSON string
		if err := rows.Scan(&d.ID, &d.CandidateID, &d.Decision, &d.DropReason, &candidateJSON); err != nil {
			return nil, err
		}
		var candidate struct {
			Reason          string   `json:"reason"`
			Text            string   `json:"text"`
			SourceLineSpans []string `json:"source_line_spans"`
			ProductKeys     []string `json:"product_keys"`
		}
		if err := json.Unmarshal([]byte(candidateJSON), &candidate); err != nil {
			return nil, fmt.Errorf("decode candidate %s: %w", d.CandidateID, err)
		}
		d.Reason, d.Text = candidate.Reason, candidate.Text
		d.SourceLineSpans, d.ProductKeys = candidate.SourceLineSpans, candidate.ProductKeys
		if d.SourceLineSpans == nil {
			d.SourceLineSpans = []string{}
		}
		if d.ProductKeys == nil {
			d.ProductKeys = []string{}
		}
		dropped = append(dropped, d)
	}
	return dropped, rows.Err()
}
