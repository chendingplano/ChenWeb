package kbhandler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// productRecord is one row from kb.products flattened for the UI.
type productRecord struct {
	ID                int64           `json:"id"`
	ProductRelID      string          `json:"product_rel_id"`
	ProductName       string          `json:"product_name"`
	ProductNameEn     string          `json:"product_name_en,omitempty"`
	CanonicalName     string          `json:"canonical_name"`
	CanonicalNameEn   string          `json:"canonical_name_en,omitempty"`
	ProductType       string          `json:"product_type,omitempty"`
	RelationType      string          `json:"relation_type,omitempty"`
	RelationSummary   string          `json:"relation_summary,omitempty"`
	RelationSummaryEn string          `json:"relation_summary_en,omitempty"`
	EvidenceQuote     string          `json:"evidence_quote,omitempty"`
	EvidenceLines     json.RawMessage `json:"evidence_lines,omitempty"`
	ObligationLevel   string          `json:"obligation_level,omitempty"`
	RequirementText   string          `json:"requirement_text,omitempty"`
	RequirementTextEn string          `json:"requirement_text_en,omitempty"`
	Conditions        json.RawMessage `json:"conditions"`
	Exceptions        json.RawMessage `json:"exceptions"`
	Parameters        json.RawMessage `json:"parameters"`
	RelatedProducts   json.RawMessage `json:"related_products"`
	ResponsibleActor  string          `json:"responsible_actor,omitempty"`
	Confidence        float64         `json:"confidence"`
	ConfidenceReason  string          `json:"confidence_reason,omitempty"`
	ModelName         string          `json:"model_name"`
	PromptName        string          `json:"prompt_name"`
	CreateTime        string          `json:"create_time"`
	ModifyTime        string          `json:"modify_time"`
}

type listProductsResponse struct {
	Status   bool            `json:"status"`
	InputID  int64           `json:"input_id"`
	FileName string          `json:"file_name,omitempty"`
	Results  []productRecord `json:"results"`
	Total    int             `json:"total"`
}

// ListProducts handles GET /api/v1/kb/products?input_record_id=N.
// It returns every product relation extracted for the record from kb.products,
// ordered by primary key (extraction order).
func ListProducts(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_PR_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_PR_010)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		logger.Error("project db handle is nil")
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "database unavailable (CWB_KB_PR_011)",
		})
	}

	// file_name lets the UI decide whether the source can render as a PDF.
	var fileName string
	if inputTable, terr := resolveInputTable(db); terr == nil {
		var fn sql.NullString
		q := fmt.Sprintf(`SELECT file_name FROM %s WHERE id = $1`, inputTable)
		if qerr := db.QueryRow(q, inputID).Scan(&fn); qerr == nil && fn.Valid {
			fileName = strings.TrimSpace(fn.String)
		}
	}

	const q = `
SELECT
	id, product_rel_id, product_name, product_name_en,
	canonical_name, canonical_name_en, product_type, relation_type,
	relation_summary, relation_summary_en, evidence_quote, evidence_lines,
	obligation_level, requirement_text, requirement_text_en,
	conditions, exceptions, parameters, related_products,
	responsible_actor, confidence, confidence_reason,
	model_name, prompt_name, create_time, modify_time
FROM kb.products
WHERE input_record_id = $1
ORDER BY id`

	rows, err := db.Query(q, inputID)
	if err != nil {
		logger.Error("query kb.products failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to load products (CWB_KB_PR_020)",
		})
	}
	defer rows.Close()

	results := make([]productRecord, 0, 16)
	for rows.Next() {
		var (
			r             productRecord
			evidenceLines []byte
			conditions    []byte
			exceptions    []byte
			parameters    []byte
			relatedProds  []byte
			createTime    time.Time
			modifyTime    time.Time
		)
		if err := rows.Scan(
			&r.ID, &r.ProductRelID, &r.ProductName, &r.ProductNameEn,
			&r.CanonicalName, &r.CanonicalNameEn, &r.ProductType, &r.RelationType,
			&r.RelationSummary, &r.RelationSummaryEn, &r.EvidenceQuote, &evidenceLines,
			&r.ObligationLevel, &r.RequirementText, &r.RequirementTextEn,
			&conditions, &exceptions, &parameters, &relatedProds,
			&r.ResponsibleActor, &r.Confidence, &r.ConfidenceReason,
			&r.ModelName, &r.PromptName, &createTime, &modifyTime,
		); err != nil {
			logger.Error("scan kb.products row failed", "err", err, "input_id", inputID)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status: false, ErrorMsg: "failed to read products (CWB_KB_PR_021)",
			})
		}
		r.EvidenceLines = jsonArrayOrEmpty(evidenceLines)
		r.Conditions = jsonArrayOrEmpty(conditions)
		r.Exceptions = jsonArrayOrEmpty(exceptions)
		r.Parameters = jsonArrayOrEmpty(parameters)
		r.RelatedProducts = jsonArrayOrEmpty(relatedProds)
		r.CreateTime = createTime.Format(time.RFC3339)
		r.ModifyTime = modifyTime.Format(time.RFC3339)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.products rows failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to read products (CWB_KB_PR_022)",
		})
	}

	logger.Info("kb products response",
		"input_id", inputID,
		"file_name", fileName,
		"product_count", len(results),
	)
	return c.JSON(http.StatusOK, listProductsResponse{
		Status:   true,
		InputID:  inputID,
		FileName: fileName,
		Results:  results,
		Total:    len(results),
	})
}
