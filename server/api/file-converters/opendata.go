package fileconverters

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type openDataDocument struct {
	NumberOfPages int              `json:"number of pages"`
	Kids          []map[string]any `json:"kids"`
}

type extractedOpenDataLine struct {
	Page         string
	Type         string
	HeadingLevel string
	Font         string
	FontSize     string
	Content      string
	BBox         string
	Raw          string
}

type renderedTableRow struct {
	Page    int
	BBox    [4]float64
	HasBBox bool
	Cols    []string
}

type renderedTable struct {
	ID     int
	PrevID int
	NextID int
	Page   int
	Rows   []renderedTableRow
}

var romanNumeralRE = regexp.MustCompile(`^[ivxlcdm]+$`)
var whitespaceRE = regexp.MustCompile(`\s+`)

const (
	defaultLineFont     = "unknown-font"
	defaultLineFontSize = "12"
)

func ConvertOpenDataFile(inputPath string) (string, error) {
	inputPath = strings.TrimSpace(inputPath)
	if inputPath == "" {
		return "", fmt.Errorf("input path is empty")
	}

	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return "", fmt.Errorf("read input file: %w", err)
	}

	var doc openDataDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("parse opendata json: %w", err)
	}

	items := extractOpenDataLineItems(doc.Kids)
	items = filterPageNumberLines(items)
	items = filterRepeatedContentLines(items, doc.NumberOfPages)
	lines := formatOpenDataLines(items)

	outputPath := openDataOutputPath(inputPath)
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write output file: %w", err)
	}

	return outputPath, nil
}

func openDataOutputPath(inputPath string) string {
	root := strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	return root + "_opendata.txt"
}

func extractOpenDataLineItems(nodes []map[string]any) []extractedOpenDataLine {
	lines := make([]extractedOpenDataLine, 0, len(nodes))
	var pendingTable *renderedTable

	flushPendingTable := func() {
		if pendingTable == nil {
			return
		}
		lines = append(lines, tableRowsFromRenderedTable(*pendingTable)...)
		pendingTable = nil
	}

	var walk func(map[string]any)
	walk = func(node map[string]any) {
		if len(node) == 0 {
			return
		}
		typ := strings.ToLower(strings.TrimSpace(asString(node["type"])))

		switch typ {
		case "header":
			// Header blocks are usually page furniture; ignore them entirely.
			return
		case "footer":
			// Footer blocks are page furniture; ignore them entirely.
			return
		case "table":
			tbl, ok := renderTableFromNode(node)
			if !ok {
				return
			}
			if pendingTable != nil && shouldMergeRenderedTables(*pendingTable, tbl) {
				mergeRenderedTables(pendingTable, tbl)
				return
			}
			flushPendingTable()
			pendingTable = &tbl
			return
		case "list":
			flushPendingTable()
			// List is a container; emit list items instead of the parent list node.
			for _, item := range asNodeSlice(node["list items"]) {
				walk(item)
			}
			for _, child := range asNodeSlice(node["kids"]) {
				walk(child)
			}
			return
		}
		flushPendingTable()

		if item, ok := makeOpenDataLineItem(node); ok {
			lines = append(lines, item)
		}

		for _, child := range asNodeSlice(node["kids"]) {
			walk(child)
		}
	}

	for _, node := range nodes {
		walk(node)
	}
	flushPendingTable()
	return lines
}

func asNodeSlice(v any) []map[string]any {
	switch x := v.(type) {
	case []map[string]any:
		return x
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func makeOpenDataLineItem(node map[string]any) (extractedOpenDataLine, bool) {
	if len(node) == 0 {
		return extractedOpenDataLine{}, false
	}

	typ := normalizeTypeToken(asString(node["type"]))
	headingLevel := strings.TrimSpace(asString(node["heading level"]))
	page := formatPage(node["page number"])
	font := strings.TrimSpace(asString(node["font"]))
	fontSize := normalizeFontSize(firstNonEmptyAny(
		node["font size"],
		node["font_size"],
		node["fontsize"],
	))
	content := strings.TrimSpace(asString(node["content"]))
	if content == "" {
		content = strings.TrimSpace(asString(node["source"]))
	}
	bbox := formatBBox(node["bounding box"])

	return extractedOpenDataLine{
		Page:         page,
		Type:         strings.TrimSpace(typ),
		HeadingLevel: headingLevel,
		Font:         font,
		FontSize:     fontSize,
		Content:      content,
		BBox:         bbox,
	}, true
}

func normalizeTypeToken(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	return whitespaceRE.ReplaceAllString(v, "-")
}

func filterPageNumberLines(lines []extractedOpenDataLine) []extractedOpenDataLine {
	if len(lines) == 0 {
		return lines
	}
	lastNonFooterByPage := map[string]int{}
	for idx, line := range lines {
		if line.Raw != "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(line.Type), "footer") {
			lastNonFooterByPage[line.Page] = idx
		}
	}

	drop := map[int]struct{}{}
	for page, idx := range lastNonFooterByPage {
		_ = page
		if isPageNumberToken(lines[idx].Content) {
			drop[idx] = struct{}{}
		}
	}

	out := make([]extractedOpenDataLine, 0, len(lines))
	for idx, line := range lines {
		if line.Raw != "" {
			out = append(out, line)
			continue
		}
		if _, ok := drop[idx]; ok {
			continue
		}
		out = append(out, line)
	}
	return out
}

func isPageNumberToken(content string) bool {
	token := strings.TrimSpace(content)
	if token == "" {
		return false
	}
	if len(strings.Fields(token)) != 1 {
		return false
	}
	if _, err := strconv.Atoi(token); err == nil {
		return true
	}
	return romanNumeralRE.MatchString(strings.ToLower(token))
}

func filterRepeatedContentLines(lines []extractedOpenDataLine, declaredPages int) []extractedOpenDataLine {
	if !lineFileRemoveRepeatLinesEnabled() || len(lines) == 0 {
		return lines
	}

	totalPages := declaredPages
	if totalPages <= 0 {
		totalPages = countDistinctPages(lines)
	}
	if totalPages <= 1 {
		return lines
	}
	minPercent := lineFileRemoveRepeatPercent()

	pagesByContent := make(map[string]map[string]struct{})
	for _, line := range lines {
		content := strings.TrimSpace(line.Content)
		page := strings.TrimSpace(line.Page)
		if line.Raw != "" || content == "" || page == "" {
			continue
		}
		if _, ok := pagesByContent[content]; !ok {
			pagesByContent[content] = make(map[string]struct{})
		}
		pagesByContent[content][page] = struct{}{}
	}

	repeated := make(map[string]struct{})
	for content, pages := range pagesByContent {
		if percentOfPages(len(pages), totalPages) >= minPercent {
			repeated[content] = struct{}{}
		}
	}
	if len(repeated) == 0 {
		return lines
	}

	out := make([]extractedOpenDataLine, 0, len(lines))
	for _, line := range lines {
		content := strings.TrimSpace(line.Content)
		if _, ok := repeated[content]; ok && content != "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

func countDistinctPages(lines []extractedOpenDataLine) int {
	pages := make(map[string]struct{})
	for _, line := range lines {
		page := strings.TrimSpace(line.Page)
		if page == "" {
			continue
		}
		pages[page] = struct{}{}
	}
	return len(pages)
}

func lineFileRemoveRepeatLinesEnabled() bool {
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("LINE_FILE_REMOVE_REPEAT_LINES")), "false")
}

func lineFileRemoveRepeatPercent() float64 {
	raw := strings.TrimSpace(os.Getenv("LINE_FILE_REMOVE_REPEAT_PERCENT"))
	if raw == "" {
		return 85
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 85
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func percentOfPages(coveredPages int, totalPages int) float64 {
	if totalPages <= 0 {
		return 0
	}
	return float64(coveredPages) * 100 / float64(totalPages)
}

func formatOpenDataLines(items []extractedOpenDataLine) []string {
	lines := make([]string, 0, len(items))
	lineNum := 1
	for _, item := range items {
		typ := item.Type
		if item.HeadingLevel != "" {
			typ = fmt.Sprintf("%s(%s)", typ, item.HeadingLevel)
		}
		page := strings.TrimSpace(item.Page)
		if page == "" {
			page = "1"
		}
		lineType := strings.TrimSpace(typ)
		if lineType == "" {
			lineType = "paragraph"
		}
		font := strings.TrimSpace(item.Font)
		if font == "" {
			font = defaultLineFont
		}
		fontSize := strings.TrimSpace(item.FontSize)
		if fontSize == "" {
			fontSize = defaultLineFontSize
		}
		coordinate := strings.TrimSpace(item.BBox)
		if coordinate == "" {
			coordinate = "[]"
		}
		lines = append(lines, strings.Join([]string{
			strconv.Itoa(lineNum),
			page,
			lineType,
			font,
			fontSize,
			coordinate,
			escapeLineContent(item.Content),
		}, "\t"))
		lineNum++
	}
	return lines
}

func escapeLineContent(content string) string {
	c := strings.ReplaceAll(content, "\r\n", "\\n")
	c = strings.ReplaceAll(c, "\n", "\\n")
	c = strings.ReplaceAll(c, "\r", "\\n")
	c = strings.ReplaceAll(c, "\t", "\\t")
	return c
}

func renderTableFromNode(node map[string]any) (renderedTable, bool) {
	rowsAny, ok := node["rows"].([]any)
	if !ok || len(rowsAny) == 0 {
		return renderedTable{}, false
	}

	numRows := asInt(node["number of rows"])
	if numRows <= 0 {
		numRows = len(rowsAny)
	}
	numCols := asInt(node["number of columns"])
	if numCols <= 0 {
		numCols = 1
	}

	grid := make([][]string, numRows)
	for i := range grid {
		grid[i] = make([]string, numCols)
	}
	rowPages := make([]int, numRows)
	rowBBoxes := make([][4]float64, numRows)
	rowBBoxValid := make([]bool, numRows)
	tableBBox, tableBBoxOK := asBBoxWithOK(node["bounding box"])

	for _, rowRaw := range rowsAny {
		row, ok := rowRaw.(map[string]any)
		if !ok {
			continue
		}
		cellsAny, ok := row["cells"].([]any)
		if !ok {
			continue
		}
		for _, cellRaw := range cellsAny {
			cell, ok := cellRaw.(map[string]any)
			if !ok {
				continue
			}
			r := asInt(cell["row number"])
			c := asInt(cell["column number"])
			if r <= 0 || c <= 0 || r > numRows || c > numCols {
				continue
			}
			rowSpan := asInt(cell["row span"])
			if rowSpan <= 0 {
				rowSpan = 1
			}

			text := extractCellText(cell)
			if text == "" {
				continue
			}
			if rowSpan > 1 {
				text += "<br><br>"
			}
			grid[r-1][c-1] = text

			cellPage := asInt(cell["page number"])
			if cellPage > 0 {
				rowPages[r-1] = cellPage
			}
			if cellBBox, ok := asBBoxWithOK(cell["bounding box"]); ok {
				if rowBBoxValid[r-1] {
					rowBBoxes[r-1] = unionBBox(rowBBoxes[r-1], cellBBox)
				} else {
					rowBBoxes[r-1] = cellBBox
					rowBBoxValid[r-1] = true
				}
			}
		}
	}

	rows := make([]renderedTableRow, 0, numRows)
	for r := 0; r < numRows; r++ {
		row := make([]string, numCols)
		copy(row, grid[r])
		page := rowPages[r]
		if page <= 0 {
			page = asInt(node["page number"])
		}
		bbox := rowBBoxes[r]
		hasBBox := rowBBoxValid[r]
		if !hasBBox && tableBBoxOK {
			bbox = tableBBox
			hasBBox = true
		}
		rows = append(rows, renderedTableRow{
			Page:    page,
			BBox:    bbox,
			HasBBox: hasBBox,
			Cols:    row,
		})
	}
	return renderedTable{
		ID:     asInt(node["id"]),
		PrevID: asInt(node["previous table id"]),
		NextID: asInt(node["next table id"]),
		Page:   asInt(node["page number"]),
		Rows:   rows,
	}, true
}

func shouldMergeRenderedTables(prev renderedTable, next renderedTable) bool {
	if prev.Page <= 0 || next.Page <= 0 {
		return false
	}
	if next.Page != prev.Page+1 {
		return false
	}

	linkedByID := (prev.NextID > 0 && prev.NextID == next.ID) || (next.PrevID > 0 && next.PrevID == prev.ID)
	if linkedByID {
		return true
	}

	if len(prev.Rows) == 0 || len(next.Rows) == 0 {
		return false
	}
	sameHeader := tableHeaderKey(prev.Rows[0].Cols) != "" &&
		tableHeaderKey(prev.Rows[0].Cols) == tableHeaderKey(next.Rows[0].Cols)
	return sameHeader
}

func mergeRenderedTables(dst *renderedTable, next renderedTable) {
	if dst == nil {
		return
	}
	if len(next.Rows) == 0 {
		return
	}
	if len(dst.Rows) > 0 && tableHeaderKey(dst.Rows[0].Cols) == tableHeaderKey(next.Rows[0].Cols) {
		dst.Rows = append(dst.Rows, next.Rows[1:]...)
	} else {
		dst.Rows = append(dst.Rows, next.Rows...)
	}
	if next.NextID > 0 {
		dst.NextID = next.NextID
	}
}

func tableRowsFromRenderedTable(tbl renderedTable) []extractedOpenDataLine {
	if len(tbl.Rows) == 0 {
		return nil
	}
	out := make([]extractedOpenDataLine, 0, len(tbl.Rows))
	for _, row := range tbl.Rows {
		page := row.Page
		if page <= 0 {
			page = tbl.Page
		}
		bbox := "[]"
		if row.HasBBox {
			bbox = formatComputedBBox(row.BBox)
		}
		out = append(out, extractedOpenDataLine{
			Page:    formatPage(page),
			Type:    "table-row",
			Content: markdownRow(row.Cols),
			BBox:    bbox,
		})
	}
	return out
}

func tableHeaderKey(cols []string) string {
	if len(cols) == 0 {
		return ""
	}
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = strings.TrimSpace(c)
	}
	return strings.Join(parts, "|")
}

func markdownRow(cols []string) string {
	escaped := make([]string, len(cols))
	for i, c := range cols {
		escaped[i] = strings.ReplaceAll(c, "|", `\|`)
	}
	return "|" + strings.Join(escaped, "|") + "|"
}

func unionBBox(a, b [4]float64) [4]float64 {
	return [4]float64{
		minFloat(a[0], b[0]),
		minFloat(a[1], b[1]),
		maxFloat(a[2], b[2]),
		maxFloat(a[3], b[3]),
	}
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

func extractCellText(cell map[string]any) string {
	return extractNodeTextFromAny(cell["kids"])
}

func extractNodeTextFromAny(v any) string {
	nodes := asNodeSlice(v)
	if len(nodes) == 0 {
		return ""
	}
	parts := make([]string, 0, 8)
	for _, n := range nodes {
		typ := strings.ToLower(strings.TrimSpace(asString(n["type"])))
		switch typ {
		case "paragraph", "heading", "list item":
			if c := strings.TrimSpace(asString(n["content"])); c != "" {
				parts = append(parts, c)
			}
			if k := extractNodeTextFromAny(n["kids"]); k != "" {
				parts = append(parts, k)
			}
		case "list":
			if li := extractNodeTextFromAny(n["list items"]); li != "" {
				parts = append(parts, li)
			}
			if k := extractNodeTextFromAny(n["kids"]); k != "" {
				parts = append(parts, k)
			}
		default:
			if c := strings.TrimSpace(asString(n["content"])); c != "" {
				parts = append(parts, c)
			}
			if k := extractNodeTextFromAny(n["kids"]); k != "" {
				parts = append(parts, k)
			}
		}
	}
	return strings.Join(parts, "<br>")
}

func asInt(v any) int {
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return 0
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}

func asBBox(v any) [4]float64 {
	bb, _ := asBBoxWithOK(v)
	return bb
}

func asBBoxWithOK(v any) ([4]float64, bool) {
	var out [4]float64
	arr, ok := v.([]any)
	if !ok || len(arr) < 4 {
		return out, false
	}
	allParsed := true
	for i := 0; i < 4; i++ {
		s := strings.TrimSpace(asString(arr[i]))
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			out[i] = f
		} else {
			allParsed = false
		}
	}
	return out, allParsed
}

func formatBBox(v any) string {
	if v == nil {
		return "[]"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func formatComputedBBox(v [4]float64) string {
	b, err := json.Marshal([]float64{v[0], v[1], v[2], v[3]})
	if err != nil {
		return "[]"
	}
	return string(b)
}

func formatPage(v any) string {
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return "0"
	}

	if f, err := strconv.ParseFloat(s, 64); err == nil {
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return s
}

func firstNonEmptyAny(values ...any) any {
	for _, v := range values {
		if strings.TrimSpace(asString(v)) != "" {
			return v
		}
	}
	return nil
}

func normalizeFontSize(v any) string {
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return ""
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f <= 0 {
		return ""
	}
	return strconv.FormatInt(int64(math.Round(f)), 10)
}
