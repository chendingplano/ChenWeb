package kbhandler

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

type getRecordSummariesResponse struct {
	Status    bool                    `json:"status"`
	RecordID  int64                   `json:"recordId"`
	Summaries []summaryCategoryRecord `json:"summaries"`
}

func GetRecordSummaries(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SREC_001")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := parseOptionalPositiveInt64(c.QueryParam("record_id"))
	if err != nil || recordID == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing or invalid record_id (CWB_KB_SREC_010)",
		})
	}

	results, err := readRecordSummaryCards(*recordID)
	if err != nil {
		logger.Error("read record summaries failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read record summaries (CWB_KB_SREC_011)",
		})
	}

	return c.JSON(http.StatusOK, getRecordSummariesResponse{
		Status:    true,
		RecordID:  *recordID,
		Summaries: results,
	})
}

func readRecordSummaryCards(recordID int64) ([]summaryCategoryRecord, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return nil, fmt.Errorf("missing ARTIFACT_DIR")
	}

	recordDir := filepath.Join(artifactDir, strconv.FormatInt(recordID/1000, 10), strconv.FormatInt(recordID, 10))
	entries, err := filepath.Glob(filepath.Join(recordDir, "summary_*_*.txt"))
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return []summaryCategoryRecord{}, nil
	}
	sort.Strings(entries)

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return nil, err
	}
	stagingExpr, err := resolveStagingOrNameExpr(db, inputTable)
	if err != nil {
		return nil, err
	}
	parserExpr, err := resolveParserNameExpr(db, inputTable)
	if err != nil {
		return nil, err
	}
	meta, err := fetchSummaryArtifactMeta(db, inputTable, stagingExpr, parserExpr, recordID)
	if err != nil {
		return nil, err
	}
	linePages, err := readLinePageMapForRecord(artifactDir, meta)
	if err != nil {
		return nil, err
	}

	results := make([]summaryCategoryRecord, 0, len(entries))
	for _, path := range entries {
		parsed, err := readSummaryArtifactFile(path)
		if err != nil {
			return nil, err
		}
		if parsed.recordID != 0 && parsed.recordID != recordID {
			continue
		}
		summaryID := parsed.summaryID
		if summaryID == "" {
			base := filepath.Base(path)
			summaryID = strings.TrimSuffix(base, filepath.Ext(base))
		}
		results = append(results, summaryCategoryRecord{
			ID:          summaryID,
			PdfFileName: filepath.Base(strings.TrimSpace(meta.fileName)),
			Keywords:    append([]string(nil), parsed.keywords...),
			SummaryText: parsed.summaryText,
			InputID:     recordID,
			Page:        firstSummaryPage(parsed.lines, linePages),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ID < results[j].ID
	})
	return results, nil
}
