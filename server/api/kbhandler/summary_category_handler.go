package kbhandler

import (
	"bufio"
	"database/sql"
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

type summaryCategoryRecord struct {
	ID          string   `json:"id"`
	PdfFileName string   `json:"pdfFileName"`
	Keywords    []string `json:"keywords"`
	SummaryText string   `json:"summaryText"`
	InputID     int64    `json:"inputId"`
	Page        int      `json:"page"`
}

type getSummaryCategoryResponse struct {
	Status       bool                    `json:"status"`
	CategoryPath string                  `json:"categoryPath"`
	Summaries    []summaryCategoryRecord `json:"summaries"`
}

type summaryArtifactMeta struct {
	recordID int64
	staging  string
	parser   string
	fileName string
}

type parsedSummaryFile struct {
	summaryID   string
	recordID    int64
	level       int
	seqNo       int
	keywords    []string
	summaryText string
	lines       []string
}

func GetSummaryCategory(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_SCAT_001")
	defer rc.Close()
	logger := rc.GetLogger()

	summaryTreeDir := strings.TrimSpace(os.Getenv("SUMMARY_TREE_DIR"))
	if summaryTreeDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "missing SUMMARY_TREE_DIR (CWB_KB_SCAT_010)",
		})
	}

	categoryPath := strings.TrimSpace(c.QueryParam("category_path"))
	if categoryPath == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "missing category_path (CWB_KB_SCAT_011)",
		})
	}

	results, err := readSummaryCategoryRecords(summaryTreeDir, categoryPath)
	if err != nil {
		logger.Error("read summary category failed", "category_path", categoryPath, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to read summary category (CWB_KB_SCAT_012)",
		})
	}

	return c.JSON(http.StatusOK, getSummaryCategoryResponse{
		Status:       true,
		CategoryPath: categoryPath,
		Summaries:    results,
	})
}

func readSummaryCategoryRecords(summaryTreeDir string, categoryPath string) ([]summaryCategoryRecord, error) {
	artifactDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if artifactDir == "" {
		return nil, fmt.Errorf("missing ARTIFACT_DIR")
	}

	summaryIDs, err := readSummaryIDsForCategory(summaryTreeDir, categoryPath)
	if err != nil {
		return nil, err
	}
	if len(summaryIDs) == 0 {
		return []summaryCategoryRecord{}, nil
	}

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

	metaCache := map[int64]summaryArtifactMeta{}
	pageCache := map[int64]map[int]int{}
	results := make([]summaryCategoryRecord, 0, len(summaryIDs))
	for _, summaryID := range summaryIDs {
		recordID, level, seqNo, ok := parseSummaryIDParts(summaryID)
		if !ok {
			continue
		}

		summaryPath := filepath.Join(artifactDir, strconv.FormatInt(recordID/1000, 10), strconv.FormatInt(recordID, 10), fmt.Sprintf("summary_%d_%04d.txt", level, seqNo))
		parsed, err := readSummaryArtifactFile(summaryPath)
		if err != nil {
			return nil, err
		}

		meta, ok := metaCache[recordID]
		if !ok {
			meta, err = fetchSummaryArtifactMeta(db, inputTable, stagingExpr, parserExpr, recordID)
			if err != nil {
				return nil, err
			}
			metaCache[recordID] = meta
		}

		linePages, ok := pageCache[recordID]
		if !ok {
			linePages, err = readLinePageMapForRecord(artifactDir, meta)
			if err != nil {
				return nil, err
			}
			pageCache[recordID] = linePages
		}

		results = append(results, summaryCategoryRecord{
			ID:          parsed.summaryID,
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

func readSummaryIDsForCategory(summaryTreeDir string, categoryPath string) ([]string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(categoryPath))
	if cleanPath == "." || cleanPath == "" {
		return nil, fmt.Errorf("invalid category path")
	}
	if strings.HasPrefix(cleanPath, "..") {
		return nil, fmt.Errorf("invalid category path")
	}
	path := filepath.Join(summaryTreeDir, filepath.FromSlash(cleanPath), "summaries.txt")
	body, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	rows := make([]string, 0)
	for _, row := range strings.Split(string(body), "\n") {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		rows = append(rows, row)
	}
	sort.Strings(rows)
	return rows, nil
}

func readSummaryArtifactFile(path string) (parsedSummaryFile, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return parsedSummaryFile{}, err
	}

	out := parsedSummaryFile{keywords: []string{}, lines: []string{}}
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)

	inSummary := false
	summaryLines := make([]string, 0)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "summary_begin:":
			inSummary = true
			continue
		case trimmed == "summary_end":
			inSummary = false
			continue
		}
		if inSummary {
			summaryLines = append(summaryLines, line)
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "summary_id:"):
			out.summaryID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "summary_id:")), `"`)
		case strings.HasPrefix(trimmed, "record_id:"):
			out.recordID, _ = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(trimmed, "record_id:")), 10, 64)
		case strings.HasPrefix(trimmed, "level:"):
			out.level, _ = strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(trimmed, "level:")))
		case strings.HasPrefix(trimmed, "lines:"):
			out.lines = parseQuotedStringArray(strings.TrimSpace(strings.TrimPrefix(trimmed, "lines:")))
		case strings.HasPrefix(trimmed, "keywords:"):
			out.keywords = parseQuotedStringArray(strings.TrimSpace(strings.TrimPrefix(trimmed, "keywords:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return parsedSummaryFile{}, err
	}
	out.summaryText = strings.TrimSpace(strings.Join(summaryLines, "\n"))
	if out.keywords == nil {
		out.keywords = []string{}
	}
	if out.lines == nil {
		out.lines = []string{}
	}
	return out, nil
}

func parseQuotedStringArray(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return []string{}
	}
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.Trim(part, `"`))
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func fetchSummaryArtifactMeta(db *sql.DB, inputTable string, stagingExpr string, parserExpr string, recordID int64) (summaryArtifactMeta, error) {
	query := fmt.Sprintf(`SELECT %s AS staging_filename, %s AS parser_name, i.file_name FROM %s i WHERE i.id = $1`, stagingExpr, parserExpr, inputTable)
	var stagingFilename sql.NullString
	var parserName sql.NullString
	var fileName sql.NullString
	if err := db.QueryRow(query, recordID).Scan(&stagingFilename, &parserName, &fileName); err != nil {
		return summaryArtifactMeta{}, err
	}
	return summaryArtifactMeta{
		recordID: recordID,
		staging:  strings.TrimSpace(stagingFilename.String),
		parser:   strings.TrimSpace(parserName.String),
		fileName: strings.TrimSpace(fileName.String),
	}, nil
}

func readLinePageMapForRecord(artifactDir string, meta summaryArtifactMeta) (map[int]int, error) {
	if meta.recordID <= 0 {
		return map[int]int{}, nil
	}
	stagingBase := filepath.Base(meta.staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if stagingRoot == "" || meta.parser == "" {
		return map[int]int{}, nil
	}
	path := filepath.Join(
		artifactDir,
		strconv.FormatInt(meta.recordID/1000, 10),
		strconv.FormatInt(meta.recordID, 10),
		stagingRoot+"_"+meta.parser+".corrected",
	)
	lines, _, err := readCorrectedLinesFile(path)
	if err != nil {
		return nil, err
	}
	out := make(map[int]int, len(lines))
	for _, line := range lines {
		out[line.LineNumber] = line.PageNumber
	}
	return out, nil
}

func firstSummaryPage(lineRanges []string, linePages map[int]int) int {
	bestLine := 0
	bestPage := 1
	for _, lineRange := range lineRanges {
		for _, lineNo := range expandSummaryLineRange(lineRange) {
			page, ok := linePages[lineNo]
			if !ok {
				continue
			}
			if bestLine == 0 || lineNo < bestLine {
				bestLine = lineNo
				bestPage = page
			}
		}
	}
	return bestPage
}

func expandSummaryLineRange(span string) []int {
	span = strings.Trim(strings.TrimSpace(span), "[]")
	if span == "" {
		return nil
	}
	if !strings.Contains(span, "-") {
		n, err := strconv.Atoi(span)
		if err != nil || n <= 0 {
			return nil
		}
		return []int{n}
	}
	parts := strings.SplitN(span, "-", 2)
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || start <= 0 || end < start {
		return nil
	}
	out := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		out = append(out, i)
	}
	return out
}
