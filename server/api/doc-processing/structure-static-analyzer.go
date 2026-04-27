package docprocessing

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

var (
	staticNumericHeadingRE  = regexp.MustCompile(`^([0-9Oo]+(?:\s*\.\s*[0-9Oo]+)*)\s*\.?\s*(.*)$`)
	staticAppendixHeadingRE = regexp.MustCompile(`^([A-Za-z])\s*\.\s*([0-9Oo]+(?:\s*\.\s*[0-9Oo]+)*)\s*\.?\s*(.*)$`)
	staticNumListRE         = regexp.MustCompile(`^\d+[\.)]?\s+\S+`)
	staticSingleSymListRE   = regexp.MustCompile(`^[*\-•·—]\s+\S+`)
	staticMultiSymListRE    = regexp.MustCompile(`^([A-Za-z]+|[ivxlcdmIVXLCDM]+)\)\s+\S+`)
	staticTOCDotLeaderRE    = regexp.MustCompile(`\.{3,}`)
)

type StaticAnalyzerProcessor struct {
	Store       DocMetadataStore
	Logger      ApiTypes.JimoLogger
	Now         func() time.Time
	ArtifactDir string
}

type staticInputLine struct {
	LineNo            int
	PageNo            int
	OriginalLineType  string
	OriginalLineLower string
	Font              string
	FontSize          string
	Coordinate        string
	Content           string
}

type staticAnalyzeResult struct {
	Lines         []staticInputLine
	CorrectedType map[int]string
	NumPages      int
	NumLines      int
}

type staticStatusParams struct {
	InputFilename   string
	NumPages        int
	NumLines        int
	NumLabeledLines int
	Start           time.Time
	DurationMs      int64
	ProcErr         error
}

func NewStaticAnalyzerProcessor(store DocMetadataStore, logger ApiTypes.JimoLogger) *StaticAnalyzerProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042301")
	}
	return &StaticAnalyzerProcessor{
		Store:       store,
		Logger:      logger,
		Now:         time.Now,
		ArtifactDir: strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
	}
}

func (p *StaticAnalyzerProcessor) Name() string { return "static_analyzer" }

func (p *StaticAnalyzerProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26042302) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if strings.TrimSpace(p.ArtifactDir) == "" {
		return errors.New("missing ARTIFACT_DIR")
	}
	if p.Store == nil {
		return errors.New("doc metadata store is nil")
	}

	rec, err := p.Store.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("(MID_26042303) kb.inputs record not found: %d", evt.RecordID)
		}
		return fmt.Errorf("(MID_26042304) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if strings.TrimSpace(rec.ParserName) == "" {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, errors.New("missing parser name"))
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, errors.New("missing result filename"))
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, "", 0, 0, 0, err)
	}
	inputFilename := filepath.Base(strings.TrimSpace(inputPath))
	if strings.TrimSpace(evt.Filename) != "" {
		inputFilename = filepath.Base(strings.TrimSpace(evt.Filename))
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, 0, 0, 0, fmt.Errorf("(MID_26042305) read input file: %w", err))
	}

	out, err := analyzeStaticStructure(body, p.Logger)
	if err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, 0, 0, 0, err)
	}
	if err := p.writeCorrectedArtifact(rec.ID, inputFilename, out); err != nil {
		return p.failAndPersist(ctx, rec, start, inputFilename, out.NumPages, out.NumLines, len(out.Lines), err)
	}

	statusRaw, err := appendStaticAnalyzerStatus(rec.StatusRaw, staticStatusParams{
		InputFilename:   inputFilename,
		NumPages:        out.NumPages,
		NumLines:        out.NumLines,
		NumLabeledLines: len(out.Lines),
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         nil,
	})
	if err != nil {
		return fmt.Errorf("(MID_26042306) append static analyzer status: %w", err)
	}
	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  nil,
	}); err != nil {
		return fmt.Errorf("(MID_26042307) update kb.inputs status: %w", err)
	}
	return nil
}

func (p *StaticAnalyzerProcessor) writeCorrectedArtifact(recordID int64, inputFilename string, out staticAnalyzeResult) error {
	groupID := recordID / 1000
	runDir := filepath.Join(p.ArtifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("(MID_26042308) create run dir: %w", err)
	}

	root := strings.TrimSuffix(filepath.Base(strings.TrimSpace(inputFilename)), filepath.Ext(strings.TrimSpace(inputFilename)))
	if root == "" {
		root = "result"
	}
	filePath := filepath.Join(runDir, root+".corrected")

	var b strings.Builder
	for i, line := range out.Lines {
		corrected := strings.TrimSpace(out.CorrectedType[line.LineNo])
		if corrected == "" {
			corrected = "unchanged"
		}
		row := []string{
			strconv.Itoa(line.LineNo),
			strconv.Itoa(line.PageNo),
			line.OriginalLineType,
			corrected,
			line.Font,
			line.FontSize,
			line.Coordinate,
			line.Content,
		}
		b.WriteString(strings.Join(row, "\t"))
		if i < len(out.Lines)-1 {
			b.WriteByte('\n')
		}
	}
	if err := os.WriteFile(filePath, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("(MID_26042309) write corrected file: %w", err)
	}
	return nil
}

func analyzeStaticStructure(body []byte, logger ApiTypes.JimoLogger) (staticAnalyzeResult, error) {
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	sc.Buffer(make([]byte, 1024), 16*1024*1024)

	lines := make([]staticInputLine, 0, 256)
	seenLineNo := make(map[int]struct{}, 256)
	pageSet := make(map[int]struct{}, 64)
	numLines := 0
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		numLines++
		line, err := parseStaticInputLine(raw)
		if err != nil {
			logger.Error("Failed to parse static input line", "Error", err)
			continue
		}
		if _, exists := seenLineNo[line.LineNo]; exists {
			continue
		}
		seenLineNo[line.LineNo] = struct{}{}
		if line.PageNo > 0 {
			pageSet[line.PageNo] = struct{}{}
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return staticAnalyzeResult{}, fmt.Errorf("(MID_26042310) read input lines: %w", err)
	}

	corrected := make(map[int]string, len(lines))
	for _, line := range lines {
		corrected[line.LineNo] = "unchanged"
	}

	lastTOC := applyStaticTOCLabels(lines, corrected, logger)
	applyStaticHeadingLabels(lastTOC, lines, corrected, logger)
	applyStaticListLabels(lines, corrected)

	return staticAnalyzeResult{
		Lines:         lines,
		CorrectedType: corrected,
		NumPages:      len(pageSet),
		NumLines:      numLines,
	}, nil
}

func parseStaticInputLine(raw string) (staticInputLine, error) {
	fields := strings.Split(raw, "\t")
	if len(fields) != 7 {
		return staticInputLine{}, errors.New("invalid field count")
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || lineNo <= 0 {
		return staticInputLine{}, errors.New("invalid line number")
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || pageNo <= 0 {
		return staticInputLine{}, errors.New("invalid page number")
	}
	originalLineType := strings.TrimSpace(fields[2])
	font := strings.TrimSpace(fields[3])
	fontSize := strings.TrimSpace(fields[4])
	coordinate := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if originalLineType == "" || font == "" || fontSize == "" || coordinate == "" {
		return staticInputLine{}, errors.New("empty required field")
	}
	return staticInputLine{
		LineNo:            lineNo,
		PageNo:            pageNo,
		OriginalLineType:  originalLineType,
		OriginalLineLower: strings.ToLower(originalLineType),
		Font:              font,
		FontSize:          fontSize,
		Coordinate:        coordinate,
		Content:           content,
	}, nil
}

// applyStaticTOCLabels identifies lines that are likely part of a table of contents based on their content and proximity to
// a TOC title, and updates the corrected map with "toc" for those lines. It returns the last pos in 'lines' of the TOC title if found,
// or -1 if no TOC is detected.
func applyStaticTOCLabels(lines []staticInputLine, corrected map[int]string, _ ApiTypes.JimoLogger) int {
	start := -1
	startPage := 0
	for i, line := range lines {
		n := normalizeStaticTitle(line.Content)
		if n == "table of content" || n == "table of contents" || n == "目录" || n == "目次" || n == "目 次" {
			start = i
			startPage = line.PageNo
			break
		}
	}
	if start < 0 {
		return -1
	}

	firstTOC := -1
	gapBefore := 0
	for i := start + 1; i < len(lines); i++ {
		if lines[i].PageNo != startPage {
			return -1
		}
		if isStaticTOCLine(lines[i]) {
			firstTOC = i
			break
		}
		gapBefore++
		if gapBefore > 2 {
			return -1
		}
	}
	if firstTOC < 0 {
		return -1
	}

	lastTOC := firstTOC
	tocCount := 0
	gap := 0
	for i := firstTOC; i < len(lines); i++ {
		if isStaticTOCLine(lines[i]) {
			tocCount++
			lastTOC = i
			gap = 0
			continue
		}
		gap++
		if gap > 3 {
			break
		}
	}
	if tocCount < 2 {
		return -1
	}
	for i := start; i <= lastTOC; i++ {
		corrected[lines[i].LineNo] = "toc"
	}
	return lastTOC
}

func normalizeStaticTitle(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

func isStaticTOCLine(line staticInputLine) bool {
	if line.OriginalLineLower != "paragraph" && line.OriginalLineLower != "list-item" {
		return false
	}
	s := strings.TrimSpace(line.Content)
	if s == "" {
		return false
	}
	if strings.Contains(s, "…") {
		return true
	}
	return staticTOCDotLeaderRE.MatchString(s)
}

func applyStaticHeadingLabels(lastTOC int, lines []staticInputLine, corrected map[int]string, logger ApiTypes.JimoLogger) {
	var prevNumeric []int
	hasNumericSeq := false
	appendixSeq := make(map[string][]int, 8)

	for i, line := range lines {
		if lastTOC > 0 && i <= lastTOC {
			continue
		}

		if corrected[line.LineNo] == "toc" {
			continue
		}
		// if line.OriginalLineLower == "list-item" {
		// 	continue
		// }

		if symbol, parts, title, ok := parseStaticAppendixHeading(line.Content); ok {
			if title == "" && i+1 < len(lines) && lines[i+1].OriginalLineLower == "paragraph" {
				title = strings.TrimSpace(lines[i+1].Content)
			}
			if title != "" && len(parts) > 0 && symbol != "" {
				if prev, exists := appendixSeq[symbol]; !exists || isStaticNextHeading(prev, parts) || len(parts) == 1 {
					corrected[line.LineNo] = fmt.Sprintf("heading-%d", len(parts))
					appendixSeq[symbol] = parts
				}
				continue
			}
		}

		if i == 88 {
			logger.Debug("Debugging line 88")
		}

		parts, title, ok := parseStaticNumericHeading(line.Content)
		if !ok {
			continue
		}
		if title == "" && i+1 < len(lines) && lines[i+1].OriginalLineLower == "paragraph" {
			title = strings.TrimSpace(lines[i+1].Content)
		}
		if title == "" || len(parts) == 0 {
			continue
		}
		if len(parts) >= 3 && parts[1] == 0 {
			parts = append([]int{parts[0]}, parts[2:]...)
		}
		if len(parts) == 0 {
			continue
		}

		if !hasNumericSeq {
			corrected[line.LineNo] = fmt.Sprintf("heading-%d", len(parts))
			prevNumeric = parts
			hasNumericSeq = true
			continue
		}
		if isStaticNextHeading(prevNumeric, parts) {
			corrected[line.LineNo] = fmt.Sprintf("heading-%d", len(parts))
			prevNumeric = parts
			hasNumericSeq = true
		}
	}
}

func parseStaticNumericHeading(content string) ([]int, string, bool) {
	m := staticNumericHeadingRE.FindStringSubmatch(strings.TrimSpace(content))
	if len(m) != 3 {
		return nil, "", false
	}
	normalized := normalizeStaticHeadingNumber(m[1])
	parts, ok := parseStaticHeadingParts(normalized)
	if !ok {
		return nil, "", false
	}
	return parts, strings.TrimSpace(m[2]), true
}

func parseStaticAppendixHeading(content string) (string, []int, string, bool) {
	m := staticAppendixHeadingRE.FindStringSubmatch(strings.TrimSpace(content))
	if len(m) != 4 {
		return "", nil, "", false
	}
	normalized := normalizeStaticHeadingNumber(m[2])
	parts, ok := parseStaticHeadingParts(normalized)
	if !ok {
		return "", nil, "", false
	}
	return strings.ToUpper(strings.TrimSpace(m[1])), parts, strings.TrimSpace(m[3]), true
}

func normalizeStaticHeadingNumber(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimSuffix(s, ".")
	s = strings.NewReplacer("O", "0", "o", "0").Replace(s)
	return s
}

func parseStaticHeadingParts(raw string) ([]int, bool) {
	norm := strings.ReplaceAll(strings.TrimSpace(raw), " ", "")
	norm = strings.TrimSuffix(norm, ".")
	if norm == "" {
		return nil, false
	}
	strParts := strings.Split(norm, ".")
	out := make([]int, 0, len(strParts))
	for _, p := range strParts {
		if strings.TrimSpace(p) == "" {
			return nil, false
		}
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, len(out) > 0
}

func isStaticNextHeading(prev []int, cur []int) bool {
	if len(prev) == 0 || len(cur) == 0 {
		return false
	}
	if len(cur) == len(prev)+1 {
		if cur[len(cur)-1] != 1 {
			return false
		}
		for i := range prev {
			if prev[i] != cur[i] {
				return false
			}
		}
		return true
	}
	if len(cur) == len(prev) {
		for i := 0; i < len(prev)-1; i++ {
			if prev[i] != cur[i] {
				return false
			}
		}
		return cur[len(cur)-1] == prev[len(prev)-1]+1
	}
	if len(cur) < len(prev) {
		for targetLen := len(cur); targetLen >= 1; targetLen-- {
			if len(cur) != targetLen {
				continue
			}
			matchPrefix := true
			for i := 0; i < targetLen-1; i++ {
				if cur[i] != prev[i] {
					matchPrefix = false
					break
				}
			}
			if !matchPrefix {
				continue
			}
			if cur[targetLen-1] == prev[targetLen-1]+1 {
				return true
			}
		}
	}
	return false
}

func applyStaticListLabels(lines []staticInputLine, corrected map[int]string) {
	for _, line := range lines {
		if line.OriginalLineLower != "list-item" {
			continue
		}
		if corrected[line.LineNo] != "unchanged" {
			continue
		}
		content := strings.TrimSpace(line.Content)
		switch {
		case staticNumListRE.MatchString(content):
			corrected[line.LineNo] = "num-list-item"
		case staticSingleSymListRE.MatchString(content):
			corrected[line.LineNo] = "s-sym-list-item"
		case staticMultiSymListRE.MatchString(content):
			corrected[line.LineNo] = "m-sym-list-item"
		}
	}
}

func appendStaticAnalyzerStatus(raw string, p staticStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":         "static_analyzer",
		"input_filename":    strings.TrimSpace(p.InputFilename),
		"num_pages":         p.NumPages,
		"num_lines":         p.NumLines,
		"num_labeled_lines": p.NumLabeledLines,
		"start_time":        p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":           p.DurationMs,
	}
	if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = strings.TrimSpace(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "static_analyzer" && op != "static-analyzer" {
			out = append(out, e)
			continue
		}
		if !replaced {
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}
	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (p *StaticAnalyzerProcessor) failAndPersist(
	ctx context.Context,
	rec DocMetadataInputRecord,
	start time.Time,
	inputFilename string,
	numPages int,
	numLines int,
	numLabeledLines int,
	procErr error,
) error {
	statusRaw, err := appendStaticAnalyzerStatus(rec.StatusRaw, staticStatusParams{
		InputFilename:   inputFilename,
		NumPages:        numPages,
		NumLines:        numLines,
		NumLabeledLines: numLabeledLines,
		Start:           start,
		DurationMs:      time.Since(start).Milliseconds(),
		ProcErr:         procErr,
	})
	if err != nil {
		return fmt.Errorf("(MID_26042311) append static analyzer status: %w", err)
	}
	errMsg := strings.TrimSpace(procErr.Error())
	if err := p.Store.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  &errMsg,
	}); err != nil {
		return fmt.Errorf("(MID_26042312) persist static analyzer failure: %w", err)
	}
	return procErr
}

func (p *StaticAnalyzerProcessor) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
