package kbhandler

import (
	"bufio"
	"database/sql"
	"fmt"
	"io/fs"
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

// ListTopicChunks handles GET /api/v1/kb/topic-chunks?input_record_id=N.
func ListTopicChunks(c echo.Context) error {
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

	topicsRootPath := topicRootPathFor(chunkDir, inputID)
	topicEntries, err := readTopicChunkEntries(topicsRootPath, inputID)
	if err != nil {
		logger.Error("read topics file failed", "path", topicsRootPath, "err", err)
		return c.JSON(http.StatusNotFound, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("topics file not found for input %d (CWB_KB_C_024)", inputID),
		})
	}

	chunks := buildTopicChunkRecords(topicEntries, rawLines)
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

func readTopicChunkEntries(root string, inputRecordID int64) ([]topicChunkEntry, error) {
	out := make([]topicChunkEntry, 0, 256)
	leafFiles, err := listTopicLeafFiles(root)
	if err != nil {
		return nil, err
	}
	for _, path := range leafFiles {
		entries, err := readTopicChunkEntriesFromFile(path, inputRecordID)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}
	for i := range out {
		out[i].SeqNo = i + 1
	}
	if len(out) == 0 {
		return nil, os.ErrNotExist
	}
	return out, nil
}

func listTopicLeafFiles(root string) ([]string, error) {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), "topics.txt") {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".txt") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func readTopicChunkEntriesFromFile(path string, inputRecordID int64) ([]topicChunkEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := make([]topicChunkEntry, 0, 64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		entry, ok := parseTopicChunkLine(scanner.Text(), inputRecordID)
		if !ok {
			continue
		}
		out = append(out, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func parseTopicChunkLine(line string, inputRecordID int64) (topicChunkEntry, bool) {
	s := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	if s == "" {
		return topicChunkEntry{}, false
	}
	parts := strings.Split(s, "\t")
	if len(parts) != 5 {
		return topicChunkEntry{}, false
	}
	idCol, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || idCol <= 0 {
		return topicChunkEntry{}, false
	}
	topicType := strings.TrimSpace(parts[1])
	if topicType == "" {
		topicType = "general"
	}
	lineTokens := parseTopicArrayItems(parts[2])
	topic := strings.TrimSpace(parts[4])
	if topic == "" {
		return topicChunkEntry{}, false
	}
	return topicChunkEntry{
		RecordID:   idCol,
		SeqNo:      0,
		TopicType:  topicType,
		LineTokens: lineTokens,
		Keywords:   parseTopicArrayItems(parts[3]),
		Topic:      topic,
	}, idCol == inputRecordID
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
