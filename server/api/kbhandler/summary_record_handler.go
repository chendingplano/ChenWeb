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

	logger.Info("get record summaries request", "record_id", *recordID)

	results, err := readRecordSummaryCards(logger, *recordID)
	if err != nil {
		logger.Error("read record summaries failed", "record_id", *recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read record summaries (CWB_KB_SREC_011)",
		})
	}

	logger.Info("get record summaries result", "record_id", *recordID, "count", len(results), "summary_ids", sampleSummaryIDs(results, 8))

	return c.JSON(http.StatusOK, getRecordSummariesResponse{
		Status:    true,
		RecordID:  *recordID,
		Summaries: results,
	})
}

func readRecordSummaryCards(logger ApiTypes.JimoLogger, recordID int64) ([]summaryCategoryRecord, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return nil, fmt.Errorf("missing ARTIFACT_DIR")
	}

	recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
	if err != nil {
		return nil, err
	}
	entries, err := filepath.Glob(filepath.Join(recordDir, "summary_*_*.txt"))
	if err != nil {
		return nil, err
	}
	if logger != nil {
		logger.Info("record summaries artifact scan",
			"record_id", recordID,
			"artifact_dir", artifactDir,
			"record_dir", recordDir,
			"summary_file_count", len(entries),
			"summary_files", summarizePaths(entries, 12),
		)
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

func summarizePaths(paths []string, limit int) []string {
	if limit <= 0 || len(paths) == 0 {
		return []string{}
	}
	if len(paths) < limit {
		limit = len(paths)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, paths[i])
	}
	return out
}

func sampleSummaryIDs(records []summaryCategoryRecord, limit int) []string {
	if limit <= 0 || len(records) == 0 {
		return []string{}
	}
	if len(records) < limit {
		limit = len(records)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, records[i].ID)
	}
	return out
}

func resolveRecordArtifactDir(artifactDir string, recordID int64) (string, error) {
	if strings.TrimSpace(artifactDir) == "" {
		return "", fmt.Errorf("missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return "", fmt.Errorf("invalid record ID: %d", recordID)
	}

	preferred := filepath.Join(
		artifactDir,
		strconv.FormatInt(recordID/1000, 10),
		strconv.FormatInt(recordID, 10),
	)
	if info, err := os.Stat(preferred); err == nil && info.IsDir() {
		return preferred, nil
	}

	matches, err := filepath.Glob(filepath.Join(artifactDir, "*", strconv.FormatInt(recordID, 10)))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	for _, match := range matches {
		if info, err := os.Stat(match); err == nil && info.IsDir() {
			return match, nil
		}
	}

	return preferred, nil
}
