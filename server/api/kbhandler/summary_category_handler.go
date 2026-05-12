package kbhandler

import (
	"bufio"
	"database/sql"
	"encoding/json"
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
	ID            string                `json:"id"`
	PdfFileName   string                `json:"pdfFileName"`
	Keywords      []string              `json:"keywords"`
	SummaryText   string                `json:"summaryText"`
	CategoryPaths []string              `json:"categoryPaths,omitempty"`
	InputID       int64                 `json:"inputId"`
	Page          int                   `json:"page"`
	Coords        []float64             `json:"coords"`
	Targets       []summaryRecordTarget `json:"targets"`
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

type summaryLineTarget struct {
	page     int
	coords   []float64
	lineType string
}

type summaryRecordTarget struct {
	Page   int       `json:"page"`
	Coords []float64 `json:"coords"`
}

type parsedSummaryFile struct {
	summaryID     string
	recordID      int64
	level         int
	seqNo         int
	keywords      []string
	categoryPaths []string
	summaryText   string
	lines         []string
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
	pageCache := map[int64]map[int]summaryLineTarget{}
	results := make([]summaryCategoryRecord, 0, len(summaryIDs))
	for _, summaryID := range summaryIDs {
		recordID, level, seqNo, ok := parseSummaryIDParts(summaryID)
		if !ok {
			continue
		}

		recordDir, err := resolveRecordArtifactDir(artifactDir, recordID)
		if err != nil {
			return nil, err
		}
		summaryPath := filepath.Join(recordDir, fmt.Sprintf("summary_%d_%04d.txt", level, seqNo))
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

		lineTargets, ok := pageCache[recordID]
		if !ok {
			lineTargets, err = readLineTargetMapForRecord(artifactDir, meta)
			if err != nil {
				return nil, err
			}
			pageCache[recordID] = lineTargets
		}

		targets := expandSummaryTargets(parsed.lines, lineTargets)
		page, coords := firstSummaryTarget(targets)

		results = append(results, summaryCategoryRecord{
			ID:            parsed.summaryID,
			PdfFileName:   filepath.Base(strings.TrimSpace(meta.fileName)),
			Keywords:      append([]string(nil), parsed.keywords...),
			SummaryText:   parsed.summaryText,
			CategoryPaths: append([]string(nil), parsed.categoryPaths...),
			InputID:       recordID,
			Page:          page,
			Coords:        coords,
			Targets:       targets,
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

	out := parsedSummaryFile{keywords: []string{}, categoryPaths: []string{}, lines: []string{}}
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
		case strings.HasPrefix(trimmed, "category_paths:"):
			out.categoryPaths = parseQuotedStringArray(strings.TrimSpace(strings.TrimPrefix(trimmed, "category_paths:")))
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
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var parsed []string
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			out := make([]string, 0, len(parsed))
			for _, part := range parsed {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				out = append(out, part)
			}
			return out
		}
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

func readLineTargetMapForRecord(artifactDir string, meta summaryArtifactMeta) (map[int]summaryLineTarget, error) {
	if meta.recordID <= 0 {
		return map[int]summaryLineTarget{}, nil
	}
	stagingBase := filepath.Base(meta.staging)
	stagingRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if stagingRoot == "" || meta.parser == "" {
		return map[int]summaryLineTarget{}, nil
	}
	recordDir, err := resolveRecordArtifactDir(artifactDir, meta.recordID)
	if err != nil {
		return nil, err
	}
	base := filepath.Join(recordDir, stagingRoot+"_"+meta.parser)
	lines, _, err := readCorrectedLinesFile(base + ".txt")
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		lines, _, err = readCorrectedLinesFile(base + ".corrected")
		if err != nil {
			if os.IsNotExist(err) {
				return map[int]summaryLineTarget{}, nil
			}
			return nil, err
		}
	}
	out := make(map[int]summaryLineTarget, len(lines))
	for _, line := range lines {
		out[line.LineNumber] = summaryLineTarget{
			page:     line.PageNumber,
			coords:   append([]float64(nil), line.Coords...),
			lineType: strings.TrimSpace(line.LineType),
		}
	}
	return out, nil
}

func expandSummaryTargets(lineRanges []string, lineTargets map[int]summaryLineTarget) []summaryRecordTarget {
	lineNos := make([]int, 0)
	seen := map[int]struct{}{}
	for _, lineRange := range lineRanges {
		for _, lineNo := range expandSummaryLineRange(lineRange) {
			if _, ok := lineTargets[lineNo]; !ok {
				continue
			}
			if _, dup := seen[lineNo]; dup {
				continue
			}
			seen[lineNo] = struct{}{}
			lineNos = append(lineNos, lineNo)
		}
	}
	sort.Ints(lineNos)
	out := make([]summaryRecordTarget, 0, len(lineNos))
	var current *summaryRecordTarget
	lastLineNo := 0
	for _, lineNo := range lineNos {
		target := lineTargets[lineNo]
		if isSummaryErrorLine(target) {
			lastLineNo = lineNo
			continue
		}
		if len(target.coords) < 4 {
			current = nil
			lastLineNo = 0
			continue
		}
		box := normalizeBBox(target.coords)
		if current != nil && target.page == current.Page && lastLineNo > 0 && lineNo == lastLineNo+1 {
			current.Coords = mergeBBox(current.Coords, box)
		} else {
			next := summaryRecordTarget{
				Page:   target.page,
				Coords: box,
			}
			out = append(out, next)
			current = &out[len(out)-1]
		}
		lastLineNo = lineNo
	}
	return out
}

func isSummaryErrorLine(target summaryLineTarget) bool {
	if len(target.coords) < 4 {
		return false
	}
	if target.coords[0] != 0 || target.coords[1] != 0 {
		return target.coords[0] == 0 && target.coords[2] >= 600
	}
	if strings.EqualFold(strings.TrimSpace(target.lineType), "image") {
		return true
	}
	return target.coords[2] >= 600 && target.coords[3] >= 800
}

func normalizeBBox(coords []float64) []float64 {
	if len(coords) < 4 {
		return []float64{}
	}
	x1, y1, x2, y2 := coords[0], coords[1], coords[2], coords[3]
	left := summaryMinFloat(x1, x2)
	top := summaryMinFloat(y1, y2)
	right := summaryMaxFloat(x1, x2)
	bottom := summaryMaxFloat(y1, y2)
	return []float64{left, top, right, bottom}
}

func mergeBBox(a []float64, b []float64) []float64 {
	if len(a) < 4 {
		return append([]float64(nil), b...)
	}
	if len(b) < 4 {
		return append([]float64(nil), a...)
	}
	return []float64{
		summaryMinFloat(a[0], b[0]),
		summaryMinFloat(a[1], b[1]),
		summaryMaxFloat(a[2], b[2]),
		summaryMaxFloat(a[3], b[3]),
	}
}

func summaryMinFloat(a float64, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func summaryMaxFloat(a float64, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func firstSummaryTarget(targets []summaryRecordTarget) (int, []float64) {
	if len(targets) == 0 {
		return 1, []float64{}
	}
	return targets[0].Page, append([]float64(nil), targets[0].Coords...)
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
