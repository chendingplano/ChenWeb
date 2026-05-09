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

type topicChunkRecord struct {
	SeqNo           int             `json:"seqno"`
	TopicType       string          `json:"topic_type"`
	Topic           string          `json:"topic"`
	Keywords        []string        `json:"keywords"`
	LineTokens      []string        `json:"line_tokens"`
	SourceLineNos   []int           `json:"source_line_numbers"`
	SourceLineSpans []sourceLineRef `json:"source_line_spans"`
	ContentLines    []rawLine       `json:"content_lines"`
	BoundingBoxes   []pageBBox      `json:"bounding_boxes"`
}

type sourceLineRef struct {
	PageNumber int `json:"page_number"`
	LineNumber int `json:"line_number"`
}

type pageBBox struct {
	PageNumber int       `json:"page_number"`
	Coords     []float64 `json:"coords"`
}

type listTopicChunksResponse struct {
	Status   bool               `json:"status"`
	InputID  int64              `json:"input_id"`
	FileName string             `json:"file_name,omitempty"`
	Results  []topicChunkRecord `json:"results"`
	Total    int                `json:"total"`
}

type topicChunkEntry struct {
	RecordID   int64
	SeqNo      int
	TopicType  string
	LineTokens []string
	Keywords   []string
	Topic      string
}

// ListChunks handles GET /api/v1/kb/chunks?input_record_id=N.
func ListChunks(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_C_001")
	defer rc.Close()
	logger := rc.GetLogger()

	idStr := strings.TrimSpace(c.QueryParam("input_record_id"))
	inputID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || inputID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status: false, ErrorMsg: "invalid input_record_id (CWB_KB_C_010)",
		})
	}

	chunkDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	if chunkDir == "" {
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "missing ARTIFACT_DIR (CWB_KB_C_011)",
		})
	}

	db := ApiTypes.ProjectDBHandle
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to resolve table (CWB_KB_C_012)",
		})
	}

	q := fmt.Sprintf(`SELECT i.result_filename, i.file_name FROM %s i WHERE i.id = $1`, inputTable)
	var resultFile sql.NullString
	var fileName sql.NullString
	if err := db.QueryRow(q, inputID).Scan(&resultFile, &fileName); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, errorResponse{
				Status: false, ErrorMsg: "record not found (CWB_KB_C_020)",
			})
		}
		logger.Error("query kb input failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status: false, ErrorMsg: "failed to retrieve record (CWB_KB_C_021)",
		})
	}

	if !resultFile.Valid || strings.TrimSpace(resultFile.String) == "" {
		return c.JSON(http.StatusNotFound, errorResponse{
			Status: false, ErrorMsg: "result_filename is empty (CWB_KB_C_022)",
		})
	}

	rawPath := rawLinePathFor(resultFile.String)
	rawLines, err := readRawLinesFile(rawPath)
	if err != nil {
		logger.Error("read raw_line file failed", "path", rawPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("raw_line file not found: %s (CWB_KB_C_023)", filepath.Base(rawPath)),
		})
	}

	chunkRootPath := topicRootPathFor(chunkDir, inputID)
	chunkEntries, err := readChunkEntries(chunkRootPath, inputID)
	if err != nil {
		logger.Error("read chunk file failed", "path", chunkRootPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("chunks file not found for input %d (CWB_KB_C_024)", inputID),
		})
	}

	chunks := buildTopicChunkRecords(chunkEntries, rawLines)
	logger.Info("kb chunks response",
		"input_id", inputID,
		"chunk_root", chunkRootPath,
		"chunk_count", len(chunks),
		"chunks", summarizeChunkRecordsForLog(chunks, 24),
	)
	resp := listTopicChunksResponse{
		Status:  true,
		InputID: inputID,
		Results: chunks,
		Total:   len(chunks),
	}
	if fileName.Valid {
		resp.FileName = fileName.String
	}
	return c.JSON(http.StatusOK, resp)
}

// ListTopicChunks is kept as a compatibility alias while the frontend migrates
// to the clearer /kb/chunks naming.
func ListTopicChunks(c echo.Context) error {
	return ListChunks(c)
}

func topicRootPathFor(artifactDir string, recordID int64) string {
	groupID := recordID / 1000
	return filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
}

func readRawLinesFile(path string) ([]rawLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make([]rawLine, 0, 1024)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		ln, ok := parseRawLine(scanner.Text())
		if !ok {
			continue
		}
		out = append(out, ln)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// readChunkEntries reads the .chunks file from root and returns pure chunk
// entries for the Chunks page. Topic artifacts belong to Semantic Web and are
// intentionally ignored here.
func readChunkEntries(root string, inputRecordID int64) ([]topicChunkEntry, error) {
	chunksPath := findChunksFilePath(root)
	if chunksPath == "" {
		return nil, os.ErrNotExist
	}

	entries, err := parseChunksFile(chunksPath, inputRecordID)
	if err != nil {
		return nil, err
	}

	summaries, err := readChunkSummaryEnrichment(root)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if len(summaries) == 0 {
		return entries, nil
	}

	for i := range entries {
		summary, ok := summaries[entries[i].SeqNo]
		if !ok {
			continue
		}
		if text := strings.TrimSpace(summary.summaryText); text != "" {
			entries[i].Topic = text
		}
		if len(summary.keywords) > 0 {
			entries[i].Keywords = append([]string(nil), summary.keywords...)
		}
		if strings.TrimSpace(entries[i].TopicType) == "" || strings.EqualFold(strings.TrimSpace(entries[i].TopicType), "general") {
			entries[i].TopicType = "summary"
		}
	}

	return entries, nil
}

func findChunksFilePath(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".chunks") {
			return filepath.Join(root, e.Name())
		}
	}
	return ""
}

func readChunkSummaryEnrichment(root string) (map[int]parsedSummaryFile, error) {
	paths, err := filepath.Glob(filepath.Join(root, "summary_0_*.txt"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, os.ErrNotExist
	}

	sort.Strings(paths)
	out := make(map[int]parsedSummaryFile, len(paths))
	for _, path := range paths {
		parsed, err := readSummaryArtifactFile(path)
		if err != nil {
			return nil, err
		}
		seqNo := parsed.seqNo
		if seqNo <= 0 {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			parts := strings.Split(base, "_")
			if len(parts) >= 3 {
				seqNo, _ = strconv.Atoi(parts[len(parts)-1])
			}
		}
		if seqNo <= 0 {
			continue
		}
		out[seqNo] = parsed
	}
	if len(out) == 0 {
		return nil, os.ErrNotExist
	}
	return out, nil
}

// parseChunksFile reads a .chunks file where each chunk occupies two lines:
//
//	overlap: [ddd, ddd-ddd, ...]
//	lines: [ddd, ddd-ddd, ...]
func parseChunksFile(path string, inputRecordID int64) ([]topicChunkEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)

	var out []topicChunkEntry
	seqNo := 1
	var pendingOverlap string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "overlap:") {
			pendingOverlap = line
			continue
		}
		if !strings.HasPrefix(line, "lines:") {
			continue
		}

		val := strings.TrimSpace(line[len("lines:"):])
		lineTokens := parseTopicArrayItems(val)
		out = append(out, topicChunkEntry{
			RecordID:   inputRecordID,
			SeqNo:      seqNo,
			TopicType:  "general",
			LineTokens: lineTokens,
			Keywords:   []string{},
			Topic:      "",
		})
		seqNo++
		_ = pendingOverlap
		pendingOverlap = ""
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, os.ErrNotExist
	}
	return out, nil
}

func summarizeChunkRecordsForLog(records []topicChunkRecord, limit int) []string {
	if limit <= 0 || len(records) == 0 {
		return []string{}
	}
	if len(records) > limit {
		records = records[:limit]
	}
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, fmt.Sprintf("#%d lines=%v spans=%d keywords=%d topic=%q",
			record.SeqNo,
			record.LineTokens,
			len(record.SourceLineSpans),
			len(record.Keywords),
			record.Topic,
		))
	}
	return out
}

// readTopicEnrichment reads a .topics file and returns a map from seq_no to entry.
// The .topics format is multi-line records separated by topic_id boundaries:
//
//	topic_id: 1
//	topic_type: "table"
//	lines: [38-45, 47]
//	topic_keywords: ["risk", "scoring"]
//	topic: "Scoring table"
//	category_paths: [...]
func readTopicEnrichment(path string) (map[int]topicChunkEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	out := make(map[int]topicChunkEntry)
	var current topicChunkEntry
	hasRecord := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "topic_id:") {
			if hasRecord && current.SeqNo > 0 {
				out[current.SeqNo] = current
			}
			current = topicChunkEntry{Keywords: []string{}}
			hasRecord = true
			n, err := strconv.Atoi(strings.TrimSpace(line[len("topic_id:"):]))
			if err == nil && n > 0 {
				current.SeqNo = n
			}
			continue
		}

		if !hasRecord {
			continue
		}

		if strings.HasPrefix(line, "topic_type:") {
			val := strings.TrimSpace(line[len("topic_type:"):])
			current.TopicType = strings.Trim(val, "\"")
		} else if strings.HasPrefix(line, "topic_keywords:") {
			kw := parseTopicArrayItems(line[len("topic_keywords:"):])
			for i := range kw {
				kw[i] = strings.Trim(kw[i], `"`)
			}
			current.Keywords = kw
		} else if strings.HasPrefix(line, "topic:") {
			val := strings.TrimSpace(line[len("topic:"):])
			current.Topic = strings.Trim(val, "\"")
		}
	}

	if hasRecord && current.SeqNo > 0 {
		out[current.SeqNo] = current
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, os.ErrNotExist
	}
	return out, nil
}

func parseTopicArrayItems(raw string) []string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		tok := strings.TrimSpace(part)
		if tok == "" {
			continue
		}
		if _, ok := seen[tok]; ok {
			continue
		}
		seen[tok] = struct{}{}
		out = append(out, tok)
	}
	return out
}

func expandTopicLineTokens(tokens []string) []int {
	set := map[int]struct{}{}
	for _, token := range tokens {
		t := strings.TrimSpace(token)
		if t == "" {
			continue
		}
		if strings.Contains(t, "-") {
			mm := strings.SplitN(t, "-", 2)
			if len(mm) != 2 {
				continue
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(mm[0]))
			end, err2 := strconv.Atoi(strings.TrimSpace(mm[1]))
			if err1 != nil || err2 != nil || start <= 0 || end <= 0 {
				continue
			}
			if end < start {
				start, end = end, start
			}
			for n := start; n <= end; n++ {
				set[n] = struct{}{}
			}
			continue
		}
		n, err := strconv.Atoi(t)
		if err != nil || n <= 0 {
			continue
		}
		set[n] = struct{}{}
	}

	out := make([]int, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}

func buildTopicChunkRecords(entries []topicChunkEntry, rawLines []rawLine) []topicChunkRecord {
	if len(entries) == 0 {
		return []topicChunkRecord{}
	}
	lineMap := make(map[int][]rawLine, len(rawLines))
	for _, ln := range rawLines {
		lineMap[ln.LineNumber] = append(lineMap[ln.LineNumber], ln)
	}

	out := make([]topicChunkRecord, 0, len(entries))
	for _, e := range entries {
		numbers := expandTopicLineTokens(e.LineTokens)
		contentLines := make([]rawLine, 0, len(numbers))
		spans := make([]sourceLineRef, 0, len(numbers))
		for _, n := range numbers {
			matches := lineMap[n]
			for _, ln := range matches {
				contentLines = append(contentLines, ln)
				spans = append(spans, sourceLineRef{PageNumber: ln.PageNumber, LineNumber: ln.LineNumber})
			}
		}
		sort.SliceStable(contentLines, func(i, j int) bool {
			if contentLines[i].PageNumber != contentLines[j].PageNumber {
				return contentLines[i].PageNumber < contentLines[j].PageNumber
			}
			return contentLines[i].LineNumber < contentLines[j].LineNumber
		})
		sort.SliceStable(spans, func(i, j int) bool {
			if spans[i].PageNumber != spans[j].PageNumber {
				return spans[i].PageNumber < spans[j].PageNumber
			}
			return spans[i].LineNumber < spans[j].LineNumber
		})
		out = append(out, topicChunkRecord{
			SeqNo:           e.SeqNo,
			TopicType:       e.TopicType,
			Topic:           e.Topic,
			Keywords:        e.Keywords,
			LineTokens:      e.LineTokens,
			SourceLineNos:   numbers,
			SourceLineSpans: spans,
			ContentLines:    contentLines,
			BoundingBoxes:   buildChunkBoundingBoxes(contentLines),
		})
	}
	return out
}

func buildChunkBoundingBoxes(lines []rawLine) []pageBBox {
	type bounds struct {
		x1 float64
		y1 float64
		x2 float64
		y2 float64
		ok bool
	}
	pageBounds := map[int]bounds{}
	for _, ln := range lines {
		if len(ln.Coords) < 4 {
			continue
		}
		x1 := minFloat(ln.Coords[0], ln.Coords[2])
		x2 := maxFloat(ln.Coords[0], ln.Coords[2])
		y1 := minFloat(ln.Coords[1], ln.Coords[3])
		y2 := maxFloat(ln.Coords[1], ln.Coords[3])
		b := pageBounds[ln.PageNumber]
		if !b.ok {
			pageBounds[ln.PageNumber] = bounds{x1: x1, y1: y1, x2: x2, y2: y2, ok: true}
			continue
		}
		b.x1 = minFloat(b.x1, x1)
		b.y1 = minFloat(b.y1, y1)
		b.x2 = maxFloat(b.x2, x2)
		b.y2 = maxFloat(b.y2, y2)
		pageBounds[ln.PageNumber] = b
	}

	pages := make([]int, 0, len(pageBounds))
	for p := range pageBounds {
		pages = append(pages, p)
	}
	sort.Ints(pages)
	out := make([]pageBBox, 0, len(pages))
	for _, p := range pages {
		b := pageBounds[p]
		out = append(out, pageBBox{
			PageNumber: p,
			Coords:     []float64{b.x1, b.y1, b.x2, b.y2},
		})
	}
	return out
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
