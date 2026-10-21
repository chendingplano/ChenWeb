package kbhandler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type goldSummaryRecord struct {
	ID                 int64           `json:"id"`
	SkillName          string          `json:"skill_name"`
	SkillVersion       string          `json:"skill_version"`
	ModelName          string          `json:"model_name"`
	RunID              string          `json:"run_id"`
	InputRecordID      int64           `json:"input_record_id"`
	SummaryID          string          `json:"summary_id"`
	SummaryLevel       int             `json:"summary_level"`
	SummarySeqNo       int             `json:"summary_seq_no"`
	SourceLineSpans    json.RawMessage `json:"source_line_spans"`
	OverlapLines       json.RawMessage `json:"overlap_lines"`
	RegularLines       json.RawMessage `json:"regular_lines"`
	SummaryText        string          `json:"summary_text"`
	SummaryTextEn      string          `json:"summary_text_en"`
	Language           string          `json:"language"`
	Keywords           json.RawMessage `json:"keywords"`
	KeywordsEn         json.RawMessage `json:"keywords_en"`
	Units              json.RawMessage `json:"units"`
	EmptyReason        string          `json:"empty_reason"`
	CompletenessReview json.RawMessage `json:"completeness_review"`
	CorrectnessReview  json.RawMessage `json:"correctness_review"`
	CreatedAt          time.Time       `json:"created_at"`
}

type listGoldSummariesResponse struct {
	Status    bool                `json:"status"`
	RecordID  int64               `json:"record_id"`
	Summaries []goldSummaryRecord `json:"summaries"`
}

// ListGoldSummaries returns the testbed summary rows for one document and
// optional benchmark run filters.
func ListGoldSummaries(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SUM_002")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := strconv.ParseInt(strings.TrimSpace(c.QueryParam("input_record_id")), 10, 64)
	if err != nil || recordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "missing or invalid input_record_id (CWB_KB_SUM_010)"})
	}

	query := `SELECT id, skill_name, skill_version, model_name, run_id, input_record_id,
		summary_id, summary_level, summary_seq_no, source_line_spans::text,
		overlap_lines::text, regular_lines::text, summary_text, summary_text_en,
		language, keywords::text, keywords_en::text, units::text, empty_reason,
		completeness_review::text, correctness_review::text, created_at
	FROM testbed.summaries
	WHERE input_record_id = $1
	  AND ($2 = '' OR skill_version = $2)
	  AND ($3 = '' OR model_name = $3)
	  AND ($4 = '' OR run_id = $4)
	ORDER BY summary_seq_no, id`
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), query,
		recordID,
		strings.TrimSpace(c.QueryParam("skill_version")),
		strings.TrimSpace(c.QueryParam("model_name")),
		strings.TrimSpace(c.QueryParam("run_id")),
	)
	if err != nil {
		logger.Error("query gold summaries failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list gold summaries (CWB_KB_SUM_011)"})
	}
	defer rows.Close()

	results := make([]goldSummaryRecord, 0)
	for rows.Next() {
		var item goldSummaryRecord
		var spans, overlap, regular, keywords, keywordsEn, units, completeness, correctness string
		if err := rows.Scan(
			&item.ID, &item.SkillName, &item.SkillVersion, &item.ModelName, &item.RunID, &item.InputRecordID,
			&item.SummaryID, &item.SummaryLevel, &item.SummarySeqNo, &spans,
			&overlap, &regular, &item.SummaryText, &item.SummaryTextEn,
			&item.Language, &keywords, &keywordsEn, &units, &item.EmptyReason,
			&completeness, &correctness, &item.CreatedAt,
		); err != nil {
			logger.Error("scan gold summary failed", "record_id", recordID, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold summary (CWB_KB_SUM_012)"})
		}
		item.SourceLineSpans = json.RawMessage(spans)
		item.OverlapLines = json.RawMessage(overlap)
		item.RegularLines = json.RawMessage(regular)
		item.Keywords = json.RawMessage(keywords)
		item.KeywordsEn = json.RawMessage(keywordsEn)
		item.Units = json.RawMessage(units)
		item.CompletenessReview = json.RawMessage(completeness)
		item.CorrectnessReview = json.RawMessage(correctness)
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate gold summaries failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to read gold summaries (CWB_KB_SUM_013)"})
	}

	return c.JSON(http.StatusOK, listGoldSummariesResponse{Status: true, RecordID: recordID, Summaries: results})
}
