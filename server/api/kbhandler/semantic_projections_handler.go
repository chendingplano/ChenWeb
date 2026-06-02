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

type semanticProjectionRecord struct {
	ID                   int64           `json:"id"`
	SemanticProjID       string          `json:"semantic_proj_id"`
	InputRecordID        int64           `json:"input_record_id"`
	EventID              string          `json:"event_id,omitempty"`
	Language             string          `json:"language,omitempty"`
	DescriptiveName      string          `json:"descriptive_name,omitempty"`
	DescriptiveNameEn    string          `json:"descriptive_name_en,omitempty"`
	Keywords             json.RawMessage `json:"keywords"`
	KeywordsEn           json.RawMessage `json:"keywords_en"`
	SemanticProjection   string          `json:"semantic_projection,omitempty"`
	SemanticProjectionEn string          `json:"semantic_projection_en,omitempty"`
	CategoryPaths        json.RawMessage `json:"category_paths"`
	CategoryPathsEn      json.RawMessage `json:"category_paths_en"`
	LineSpans            json.RawMessage `json:"line_spans"`
	ModelName            string          `json:"model_name,omitempty"`
	PromptName           string          `json:"prompt_name,omitempty"`
	CreateTime           string          `json:"create_time"`
}

type listSemanticProjectionsResponse struct {
	Status   bool                       `json:"status"`
	InputID  int64                      `json:"input_id"`
	FileName string                     `json:"file_name,omitempty"`
	Results  []semanticProjectionRecord `json:"results"`
	Total    int                        `json:"total"`
}

// ListSemanticProjections returns the persisted semantic-projection rows for a
// single kb.inputs record. The view is read-only: rows are produced by the
// extract_semantic_projections doc processor.
func ListSemanticProjections(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SP_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "invalid input_record_id (CWB_KB_SP_010)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "database unavailable (CWB_KB_SP_011)",
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
	id, semantic_proj_id, input_record_id, event_id, language,
	descriptive_name, descriptive_name_en, keywords, keywords_en,
	semantic_projection, semantic_projection_en, category_paths, category_paths_en,
	line_spans,
	model_name, prompt_name, create_time
FROM kb.semantic_projections
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.Query(q, inputID)
	if err != nil {
		logger.Error("query kb.semantic_projections failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to load semantic projections (CWB_KB_SP_020)",
		})
	}
	defer rows.Close()

	results := make([]semanticProjectionRecord, 0, 16)
	for rows.Next() {
		var (
			r                    semanticProjectionRecord
			eventID              sql.NullString
			language             sql.NullString
			descriptiveName      sql.NullString
			descriptiveNameEn    sql.NullString
			keywords             []byte
			keywordsEn           []byte
			semanticProjection   sql.NullString
			semanticProjectionEn sql.NullString
			categoryPaths        []byte
			categoryPathsEn      []byte
			lineSpans            []byte
			modelName            sql.NullString
			promptName           sql.NullString
			createTime           time.Time
		)
		if err := rows.Scan(
			&r.ID, &r.SemanticProjID, &r.InputRecordID, &eventID, &language,
			&descriptiveName, &descriptiveNameEn, &keywords, &keywordsEn,
			&semanticProjection, &semanticProjectionEn, &categoryPaths, &categoryPathsEn,
			&lineSpans,
			&modelName, &promptName, &createTime,
		); err != nil {
			logger.Error("scan kb.semantic_projections row failed", "err", err, "input_id", inputID)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to read semantic projections (CWB_KB_SP_021)",
			})
		}
		r.EventID = strings.TrimSpace(eventID.String)
		r.Language = strings.TrimSpace(language.String)
		r.DescriptiveName = strings.TrimSpace(descriptiveName.String)
		r.DescriptiveNameEn = strings.TrimSpace(descriptiveNameEn.String)
		r.SemanticProjection = semanticProjection.String
		r.SemanticProjectionEn = semanticProjectionEn.String
		r.ModelName = strings.TrimSpace(modelName.String)
		r.PromptName = strings.TrimSpace(promptName.String)
		r.Keywords = jsonArrayOrEmpty(keywords)
		r.KeywordsEn = jsonArrayOrEmpty(keywordsEn)
		r.CategoryPaths = jsonArrayOrEmpty(categoryPaths)
		r.CategoryPathsEn = jsonArrayOrEmpty(categoryPathsEn)
		r.LineSpans = jsonArrayOrEmpty(lineSpans)
		r.CreateTime = createTime.Format(time.RFC3339)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate kb.semantic_projections rows failed", "err", err, "input_id", inputID)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read semantic projections (CWB_KB_SP_022)",
		})
	}

	return c.JSON(http.StatusOK, listSemanticProjectionsResponse{
		Status:   true,
		InputID:  inputID,
		FileName: fileName,
		Results:  results,
		Total:    len(results),
	})
}
