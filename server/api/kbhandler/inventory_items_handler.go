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

type inventoryItemRecord struct {
	ID                   int64           `json:"id"`
	InventoryItemID      string          `json:"inventory_item_id"`
	ItemName             string          `json:"item_name"`
	CanonicalName        string          `json:"canonical_name"`
	ItemCategory         string          `json:"item_category"`
	Manufacturer         string          `json:"manufacturer,omitempty"`
	Brand                string          `json:"brand,omitempty"`
	ModelNumber          string          `json:"model_number,omitempty"`
	PartNumber           string          `json:"part_number,omitempty"`
	NormalizedSpecs      json.RawMessage `json:"normalized_specs"`
	RawSpecs             json.RawMessage `json:"raw_specs"`
	Standards            json.RawMessage `json:"standards"`
	Aliases              json.RawMessage `json:"aliases"`
	EvidenceQuote        string          `json:"evidence_quote,omitempty"`
	SourceLineSpans      json.RawMessage `json:"source_line_spans"`
	ValidationFlags      json.RawMessage `json:"validation_flags"`
	MissingRequiredAttrs json.RawMessage `json:"missing_required_attrs"`
	DedupeKey            string          `json:"dedupe_key,omitempty"`
	SchemaVersion        string          `json:"schema_version,omitempty"`
	DictionaryVersion    string          `json:"dictionary_version,omitempty"`
	Confidence           float64         `json:"confidence"`
	ConfidenceReason     string          `json:"confidence_reason,omitempty"`
	ModelName            string          `json:"model_name"`
	PromptName           string          `json:"prompt_name"`
	CreateTime           string          `json:"create_time"`
	ModifyTime           string          `json:"modify_time"`
}

type listInventoryItemsResponse struct {
	Status   bool                  `json:"status"`
	InputID  int64                 `json:"input_id"`
	FileName string                `json:"file_name,omitempty"`
	Results  []inventoryItemRecord `json:"results"`
	Total    int                   `json:"total"`
}

func ListInventoryItems(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_INV_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid input_record_id (CWB_KB_INV_010)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "database unavailable (CWB_KB_INV_011)",
		})
	}

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
	id, inventory_item_id, item_name, canonical_name, item_category,
	manufacturer, brand, model_number, part_number,
	normalized_specs, raw_specs, standards, aliases,
	evidence_quote, source_line_spans, validation_flags, missing_required_attrs,
	dedupe_key, schema_version, dictionary_version, confidence, confidence_reason,
	model_name, prompt_name, create_time, modify_time
FROM kb.inventory_items
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.Query(q, inputID)
	if err != nil {
		logger.Error("query kb.inventory_items failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to load inventory items (CWB_KB_INV_020)",
		})
	}
	defer rows.Close()

	results := make([]inventoryItemRecord, 0, 16)
	for rows.Next() {
		var (
			r                    inventoryItemRecord
			normalizedSpecs      []byte
			rawSpecs             []byte
			standards            []byte
			aliases              []byte
			sourceLineSpans      []byte
			validationFlags      []byte
			missingRequiredAttrs []byte
			createTime           time.Time
			modifyTime           time.Time
		)
		if err := rows.Scan(
			&r.ID, &r.InventoryItemID, &r.ItemName, &r.CanonicalName, &r.ItemCategory,
			&r.Manufacturer, &r.Brand, &r.ModelNumber, &r.PartNumber,
			&normalizedSpecs, &rawSpecs, &standards, &aliases,
			&r.EvidenceQuote, &sourceLineSpans, &validationFlags, &missingRequiredAttrs,
			&r.DedupeKey, &r.SchemaVersion, &r.DictionaryVersion, &r.Confidence, &r.ConfidenceReason,
			&r.ModelName, &r.PromptName, &createTime, &modifyTime,
		); err != nil {
			logger.Error("scan kb.inventory_items row failed", "err", err, "input_id", inputID)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to read inventory items (CWB_KB_INV_021)",
			})
		}
		r.NormalizedSpecs = jsonArrayOrEmpty(normalizedSpecs)
		r.RawSpecs = jsonArrayOrEmpty(rawSpecs)
		r.Standards = jsonArrayOrEmpty(standards)
		r.Aliases = jsonArrayOrEmpty(aliases)
		r.SourceLineSpans = jsonArrayOrEmpty(sourceLineSpans)
		r.ValidationFlags = jsonArrayOrEmpty(validationFlags)
		r.MissingRequiredAttrs = jsonArrayOrEmpty(missingRequiredAttrs)
		r.CreateTime = createTime.Format(time.RFC3339)
		r.ModifyTime = modifyTime.Format(time.RFC3339)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.inventory_items rows failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read inventory items (CWB_KB_INV_022)",
		})
	}

	return c.JSON(http.StatusOK, listInventoryItemsResponse{
		Status:   true,
		InputID:  inputID,
		FileName: fileName,
		Results:  results,
		Total:    len(results),
	})
}

func SearchInventoryItems(c echo.Context) error {
	return searchRegistryArtifacts(c, "inventory_item")
}
