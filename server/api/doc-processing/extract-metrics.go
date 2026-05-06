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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type MetricsProcessor struct {
	InputStore   DocMetadataStore
	Store        MetricsStore
	Extractor    LLMJSONExtractor
	Logger       ApiTypes.JimoLogger
	Now          func() time.Time
	PromptText   string
	PromptRef    string
	PromptPath   string
	PromptErr    error
	ModelRef     string
	ModelCfgPath string
	ModelErr     error
	ModelName    string
	ChunkDir     string
}

type MetricsStore interface {
	MetricsExist(ctx context.Context, inputRecordID int64, inputFilename string) (bool, error)
	DeleteMetricsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error)
}

type MetricsSQLStore struct {
	DB *sql.DB
}

type SaveMetricsRequest struct {
	InputRecordID int64
	InputFilename string
	ExtractID     string
	Language      string
	Metrics       []map[string]any
}

func NewMetricsProcessor(inputStore DocMetadataStore, store MetricsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *MetricsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042470")
	}
	promptText, promptRef, promptPath, promptErr := loadMetricsPromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODELS_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	return &MetricsProcessor{
		InputStore:   inputStore,
		Store:        store,
		Extractor:    extractor,
		Logger:       logger,
		Now:          time.Now,
		PromptText:   promptText,
		PromptRef:    promptRef,
		PromptPath:   promptPath,
		PromptErr:    promptErr,
		ModelRef:     modelRef,
		ModelCfgPath: modelCfgPath,
		ModelErr:     modelErr,
		ModelName:    modelCfg.ModelName,
		ChunkDir:     strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
	}
}

func (p *MetricsProcessor) Name() string { return "extract_metrics" }

func (p *MetricsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26042457) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26042458) load metrics prompt %q: %w", p.PromptRef, p.PromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if err == sql.ErrNoRows {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("(MID_26042459) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("metrics extraction skipped: model config error",
			"record_id", evt.RecordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		p.persistMetricsStatus(ctx, rec, start, p.ModelErr)
		return nil
	}

	topicsPath, chunkFiles, detectErr := p.detectChunkingInputs(evt.RecordID)
	if detectErr != nil {
		p.Logger.Error("metrics extraction skipped: chunk detection failed",
			"record_id", evt.RecordID, "chunk_dir", p.ChunkDir, "error", detectErr)
		p.persistMetricsStatus(ctx, rec, start, detectErr)
		return nil
	}
	if topicsPath == "" && len(chunkFiles) == 0 {
		err := fmt.Errorf("(MID_26042460) no chunk files found for record_id=%d", evt.RecordID)
		p.Logger.Error("no chunk files",
			"record_id", evt.RecordID,
			"chunk_dir", p.ChunkDir)
		p.persistMetricsStatus(ctx, rec, start, err)
		return nil
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042466) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return nil
	}

	if evt.Force {
		_, _ = p.Store.DeleteMetricsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.MetricsExist(ctx, evt.RecordID, "")
		if err != nil {
			p.persistMetricsStatus(ctx, rec, start, err)
			return nil
		}
		if exists {
			p.Logger.Info("metrics extraction skipped", "record_id", evt.RecordID, "reason", "metrics already exist and force=false")
			p.persistMetricsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	var allLines []string
	var inputCount int
	if topicsPath != "" {
		lines, readErr := readLinesFromTopicsFile(topicsPath, lineFilePath)
		if readErr != nil {
			p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042467) read topics file %s: %w", topicsPath, readErr))
			return nil
		}
		allLines = lines
		inputCount = 1
	} else {
		allLines = make([]string, 0, 1024)
		for _, chunkPath := range chunkFiles {
			lines, err := readRegularLinesFromChunk(chunkPath, lineFilePath)
			if err != nil {
				p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042461) read chunk file %s: %w", chunkPath, err))
				return nil
			}
			allLines = append(allLines, lines...)
		}
		inputCount = len(chunkFiles)
	}
	if len(allLines) == 0 {
		err := fmt.Errorf("(MID_26042462) no regular lines found in chunk files for record_id=%d", evt.RecordID)
		p.Logger.Warn("metrics extraction skipped: no regular lines in chunk files",
			"record_id", evt.RecordID, "chunk_files", inputCount)
		p.persistMetricsStatus(ctx, rec, start, err)
		return nil
	}

	result, err := p.extractMetricsFromLinesWithLLM(ctx, allLines)
	if err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return nil
	}

	inputFilename := filepath.Base(strings.TrimSpace(rec.ResultFilename))
	if inputFilename == "" {
		inputFilename = fmt.Sprintf("record_%d", evt.RecordID)
	}
	inserted, err := p.Store.SaveMetrics(ctx, SaveMetricsRequest{
		InputRecordID: evt.RecordID,
		InputFilename: inputFilename,
		ExtractID:     result.ExtractID,
		Language:      result.Language,
		Metrics:       result.Metrics,
	})
	if err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return nil
	}

	p.Logger.Info("metrics extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"metrics_count", len(result.Metrics),
		"uncertain_metrics_count", len(result.UncertainMetrics),
		"chunk_files", inputCount,
	)
	p.persistMetricsStatus(ctx, rec, start, nil)
	return nil
}

// detectChunkingInputs checks the artifact directory for the record and returns
// the preferred topic artifact path or chunk artifact paths, prioritizing the
// spec-compliant .topics/.chunks layout before falling back to legacy files.
func (p *MetricsProcessor) detectChunkingInputs(recordID int64) (topicsPath string, chunkFiles []string, err error) {
	if strings.TrimSpace(p.ChunkDir) == "" {
		return "", nil, errors.New("(MID_26042463) missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return "", nil, fmt.Errorf("(MID_26042464) invalid record_id: %d", recordID)
	}
	groupID := recordID / 1000
	dir := filepath.Join(p.ChunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, nil
		}
		return "", nil, err
	}

	topicPaths := make([]string, 0, 2)
	legacyTopicPaths := make([]string, 0, 1)
	chunkPaths := make([]string, 0, len(entries))
	legacyChunkPaths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		fullPath := filepath.Join(dir, name)
		switch {
		case strings.HasSuffix(name, ".topics"):
			topicPaths = append(topicPaths, fullPath)
		case name == "topics.txt":
			legacyTopicPaths = append(legacyTopicPaths, fullPath)
		case strings.HasSuffix(name, ".chunks"):
			chunkPaths = append(chunkPaths, fullPath)
		case strings.HasPrefix(name, "chunk_"):
			legacyChunkPaths = append(legacyChunkPaths, fullPath)
		default:
			continue
		}
	}
	sort.Strings(topicPaths)
	if len(topicPaths) > 0 {
		return topicPaths[0], nil, nil
	}
	sort.Strings(legacyTopicPaths)
	if len(legacyTopicPaths) > 0 {
		return legacyTopicPaths[0], nil, nil
	}
	sort.Strings(chunkPaths)
	if len(chunkPaths) > 0 {
		return "", chunkPaths, nil
	}
	sort.Strings(legacyChunkPaths)
	return "", legacyChunkPaths, nil
}

func readRegularLinesFromChunk(path string, lineFilePath string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lines := make([]string, 0, 256)
	regularLineNos := make([]int, 0, 256)
	hasSummaryFormat := false
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 16*1024*1024)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		if strings.HasPrefix(raw, "overlap:") {
			hasSummaryFormat = true
			continue
		}
		if strings.HasPrefix(raw, "lines:") {
			hasSummaryFormat = true
			nums, err := parseTopicLineRanges(strings.TrimSpace(strings.TrimPrefix(raw, "lines:")))
			if err != nil {
				return nil, err
			}
			regularLineNos = append(regularLineNos, nums...)
			continue
		}
		base, mark := splitChunkMark(raw)
		if mark == "o" {
			continue
		}
		lines = append(lines, base)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if hasSummaryFormat {
		if strings.TrimSpace(lineFilePath) == "" {
			return nil, fmt.Errorf("summary chunk file requires source line file")
		}
		wantLines := make(map[int]struct{}, len(regularLineNos))
		for _, n := range regularLineNos {
			if n > 0 {
				wantLines[n] = struct{}{}
			}
		}
		return readLinesFromFileByNumbers(lineFilePath, wantLines)
	}
	return lines, nil
}

func splitChunkMark(line string) (base string, mark string) {
	raw := strings.TrimSpace(line)
	if raw == "" {
		return "", ""
	}

	// Preferred format: "<mark> <line...>"
	if len(raw) > 2 && (raw[0] == 'r' || raw[0] == 'o') && raw[1] == ' ' {
		return strings.TrimSpace(raw[2:]), string(raw[0])
	}

	// Backward compatibility: "<line...> <mark>"
	idx := strings.LastIndex(raw, " ")
	if idx < 0 {
		return raw, "r"
	}
	last := strings.TrimSpace(raw[idx+1:])
	if last == "r" || last == "o" {
		return strings.TrimSpace(raw[:idx]), last
	}
	return raw, "r"
}

// readLinesFromTopicsFile reads topics.txt and the original line file,
// returning the ordered, deduplicated set of lines referenced by the topics.
func readLinesFromTopicsFile(topicsPath, lineFilePath string) ([]string, error) {
	topics, err := readTopicsFile(topicsPath)
	if err != nil {
		return nil, fmt.Errorf("(MID_26042468) read topics file %s: %w", topicsPath, err)
	}
	wantLines := make(map[int]struct{})
	for _, t := range topics {
		nums, parseErr := parseTopicLineRanges(t.Lines)
		if parseErr != nil {
			return nil, fmt.Errorf("(MID_26042469) parse line ranges for topic seq=%d: %w", t.SeqNo, parseErr)
		}
		for _, n := range nums {
			wantLines[n] = struct{}{}
		}
	}
	if len(wantLines) == 0 {
		return nil, nil
	}
	return readLinesFromFileByNumbers(lineFilePath, wantLines)
}

// parseTopicLineRanges parses a lines field like "[38-45, 47, 49]" into a slice of line numbers.
func parseTopicLineRanges(raw string) ([]int, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := make([]int, 0, 16)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, "-"); idx > 0 {
			start, err1 := strconv.Atoi(strings.TrimSpace(part[:idx]))
			end, err2 := strconv.Atoi(strings.TrimSpace(part[idx+1:]))
			if err1 != nil || err2 != nil || start < 1 || end < start {
				return nil, fmt.Errorf("invalid range: %q", part)
			}
			for n := start; n <= end; n++ {
				out = append(out, n)
			}
		} else {
			n, err := strconv.Atoi(part)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid line number: %q", part)
			}
			out = append(out, n)
		}
	}
	return out, nil
}

// readLinesFromFileByNumbers reads a canonical line file and returns lines whose
// line number (first tab-separated field) is in wantLines, sorted and deduplicated.
func readLinesFromFileByNumbers(path string, wantLines map[int]struct{}) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type numberedLine struct {
		no   int
		text string
	}
	collected := make([]numberedLine, 0, len(wantLines))

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 16*1024*1024)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
			continue
		}
		idx := strings.Index(raw, "\t")
		if idx < 0 {
			continue
		}
		lineNo, err := strconv.Atoi(strings.TrimSpace(raw[:idx]))
		if err != nil || lineNo < 1 {
			continue
		}
		if _, ok := wantLines[lineNo]; ok {
			collected = append(collected, numberedLine{no: lineNo, text: raw})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	sort.Slice(collected, func(i, j int) bool { return collected[i].no < collected[j].no })

	out := make([]string, 0, len(collected))
	lastNo := -1
	for _, nl := range collected {
		if nl.no == lastNo {
			continue
		}
		lastNo = nl.no
		out = append(out, nl.text)
	}
	return out, nil
}

func (p *MetricsProcessor) persistMetricsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	statusRaw, err := appendMetricsStatus(rec.StatusRaw, start, time.Since(start).Milliseconds(), procErr)
	if err != nil {
		p.Logger.Error("failed building metrics status", "record_id", rec.ID, "error", err)
		return
	}
	if err := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); err != nil {
		p.Logger.Error("failed persisting metrics status", "record_id", rec.ID, "error", err)
	}
}

func appendMetricsStatus(raw string, start time.Time, durationMs int64, procErr error) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":  "extract_metrics",
		"start_time": start.Format(defaultDocMetaStatusTime),
		"ms-used":    durationMs,
	}
	if procErr == nil {
		entry["proc-status"] = "success"
		entry["proc_status"] = "success"
	} else {
		entry["proc-status"] = "failed"
		entry["proc_status"] = "failed"
		entry["error"] = strings.TrimSpace(procErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "extract_metrics" && op != "extract-metrics" {
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

type metricExtractionResult struct {
	ExtractID        string
	Language         string
	Metrics          []map[string]any
	UncertainMetrics []map[string]any
}

func (p *MetricsProcessor) extractMetricsFromLinesWithLLM(ctx context.Context, lines []string) (metricExtractionResult, error) {
	parsedLines := parseMetricInputLines(lines)
	userPrompt := buildMetricUserPrompt(lines, parsedLines)
	payload, err := p.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: p.PromptText,
		ModelName:  p.ModelName,
		InputText:  userPrompt,
	})
	if err != nil {
		return metricExtractionResult{}, fmt.Errorf("(MID_26042451) extract metrics via llm: %w", err)
	}

	language := strings.TrimSpace(asString(payload["language"]))
	if language == "" {
		language = "unknown"
	}

	metricsRaw, ok := payload["metrics"].([]any)
	if !ok {
		return metricExtractionResult{}, fmt.Errorf("(MID_26042452) llm output field 'metrics' must be an array")
	}
	uncertainRaw, _ := payload["uncertain_metrics"].([]any)

	metrics := normalizeMetricList(metricsRaw)
	uncertain := normalizeMetricList(uncertainRaw)
	return metricExtractionResult{
		ExtractID:        p.Now().Format("20060102-150405"),
		Language:         language,
		Metrics:          metrics,
		UncertainMetrics: uncertain,
	}, nil
}

type metricParsedLine struct {
	LineNumber int    `json:"line_number"`
	PageNumber int    `json:"page_number"`
	LineType   string `json:"line_type"`
	Font       string `json:"font"`
	FontSize   string `json:"font_size"`
	Content    string `json:"content"`
	Coordinate string `json:"coordinate"`
}

func parseMetricInputLine(line string) (metricParsedLine, bool) {
	raw := strings.TrimSpace(line)
	if raw == "" {
		return metricParsedLine{}, false
	}
	fields := strings.Split(raw, "\t")
	if len(fields) != 7 {
		return metricParsedLine{}, false
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[0]))
	if err != nil || lineNo < 1 {
		return metricParsedLine{}, false
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || pageNo < 1 {
		return metricParsedLine{}, false
	}
	lineType := strings.TrimSpace(fields[2])
	font := strings.TrimSpace(fields[3])
	fontSize := strings.TrimSpace(fields[4])
	coordinate := strings.TrimSpace(fields[5])
	content := strings.TrimSpace(fields[6])
	if lineType == "" || font == "" || fontSize == "" || coordinate == "" {
		return metricParsedLine{}, false
	}

	return metricParsedLine{
		LineNumber: lineNo,
		PageNumber: pageNo,
		LineType:   lineType,
		Font:       font,
		FontSize:   fontSize,
		Content:    content,
		Coordinate: coordinate,
	}, true
}

func parseMetricInputLines(lines []string) []metricParsedLine {
	parsedLines := make([]metricParsedLine, 0, len(lines))
	for _, raw := range lines {
		if line, ok := parseMetricInputLine(raw); ok {
			parsedLines = append(parsedLines, line)
		}
	}
	return parsedLines
}

func buildMetricUserPrompt(lines []string, parsedLines []metricParsedLine) string {
	schema := map[string]any{
		"language": "string",
		"metrics": []map[string]any{{
			"metric_name":           "string",
			"source_line_spans":     []string{"5", "12:14"},
			"subject":               "string",
			"desc":                  "string",
			"context":               "string",
			"keywords":              []string{"string"},
			"location_type":         "string",
			"unit":                  "string",
			"formula_or_definition": "string",
			"threshold_or_target":   "string",
			"measurement_frequency": "string",
			"confidence":            0.0,
			"is_explicit_metric":    true,
			"table_name_or_section": "string",
			"reasoning_tags":        []string{"string"},
		}},
		"uncertain_metrics": []any{},
	}

	linesJSON, _ := json.Marshal(lines)
	parsedLinesJSON, _ := json.Marshal(parsedLines)
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nInput lines (raw, JSON array):\n" + string(linesJSON) +
		"\n\nParsed line hints (best effort; do not treat as complete):\n" + string(parsedLinesJSON)
}

func normalizeMetricList(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		normalized := map[string]any{
			"metric_name":           strings.TrimSpace(asString(raw["metric_name"])),
			"subject":               strings.TrimSpace(asString(raw["subject"])),
			"desc":                  strings.TrimSpace(asString(raw["desc"])),
			"context":               strings.TrimSpace(asString(raw["context"])),
			"keywords":              toStringSlice(raw["keywords"]),
			"location_type":         strings.TrimSpace(asString(raw["location_type"])),
			"unit":                  strings.TrimSpace(asString(raw["unit"])),
			"formula_or_definition": strings.TrimSpace(asString(raw["formula_or_definition"])),
			"threshold_or_target":   strings.TrimSpace(asString(raw["threshold_or_target"])),
			"measurement_frequency": strings.TrimSpace(asString(raw["measurement_frequency"])),
			"confidence":            toFloat(raw["confidence"]),
			"is_explicit_metric":    toBool(raw["is_explicit_metric"]),
			"table_name_or_section": strings.TrimSpace(asString(raw["table_name_or_section"])),
			"reasoning_tags":        toStringSlice(raw["reasoning_tags"]),
		}
		normalized["source_line_spans"] = normalizeSourceLineSpans(raw["source_line_spans"])
		out = append(out, normalized)
	}
	return out
}

func normalizeSourceLineSpans(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}

	type lineSpan struct{ start, end int }
	spans := make([]lineSpan, 0, len(items))

	for _, item := range items {
		switch v := item.(type) {
		case map[string]any:
			lineNo := int(toFloat(v["line_number"]))
			if lineNo > 0 {
				spans = append(spans, lineSpan{lineNo, lineNo})
			}
		case float64:
			lineNo := int(v)
			if lineNo > 0 {
				spans = append(spans, lineSpan{lineNo, lineNo})
			}
		case string:
			s := strings.TrimSpace(v)
			if idx := strings.Index(s, ":"); idx > 0 {
				start, err1 := strconv.Atoi(strings.TrimSpace(s[:idx]))
				end, err2 := strconv.Atoi(strings.TrimSpace(s[idx+1:]))
				if err1 == nil && err2 == nil && start > 0 && end >= start {
					spans = append(spans, lineSpan{start, end})
				}
			} else {
				n, err := strconv.Atoi(s)
				if err == nil && n > 0 {
					spans = append(spans, lineSpan{n, n})
				}
			}
		}
	}

	if len(spans) == 0 {
		return nil
	}

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end < spans[j].end
	})

	merged := []lineSpan{spans[0]}
	for _, s := range spans[1:] {
		last := &merged[len(merged)-1]
		if s.start <= last.end+1 {
			if s.end > last.end {
				last.end = s.end
			}
		} else {
			merged = append(merged, s)
		}
	}

	out := make([]string, 0, len(merged))
	for _, s := range merged {
		if s.start == s.end {
			out = append(out, strconv.Itoa(s.start))
		} else {
			out = append(out, fmt.Sprintf("%d:%d", s.start, s.end))
		}
	}
	return out
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		s := strings.TrimSpace(asString(item))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	default:
		return 0
	}
}

func toBool(v any) bool {
	b, err := asBool(v, false)
	if err != nil {
		return false
	}
	return b
}

func loadMetricsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = "default_metric_prompt.txt"
	}

	paths := make([]string, 0, 8)
	addCandidate := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, existing := range paths {
			if existing == p {
				return
			}
		}
		paths = append(paths, p)
	}

	if filepath.IsAbs(promptRef) {
		addCandidate(promptRef)
	} else {
		addCandidate(promptRef)
		if promptDir := strings.TrimSpace(os.Getenv("PROMPT_DIR")); promptDir != "" {
			addCandidate(filepath.Join(promptDir, promptRef))
		}
		addCandidate(filepath.Join("server", "cmd", "doc-processor", promptRef))
		addCandidate(filepath.Join("server", "cmd", "doc-processor", "prompts", promptRef))
		addCandidate(filepath.Join("python", "extract_metrics", "prompts", promptRef))
		addCandidate(filepath.Join("prompts", promptRef))
	}

	var lastErr error
	for _, candidate := range paths {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = fmt.Errorf("(MID_26042465) failed reading file. Path:%s, error:%w", candidate, err)
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("(MID_26042453) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("(MID_26042454) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26042455) prompt file not found: %w", lastErr)
}

func (s MetricsSQLStore) ensureMetricsTable(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26042456) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.metrics (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL,
    extract_id TEXT NOT NULL,
    input_filename TEXT NOT NULL,
    metric_name TEXT,
    source_line_spans JSONB,
    metric_subject TEXT,
    metric_desc TEXT,
    metric_context TEXT,
    metric_keywords JSONB,
    location_type TEXT,
    metric_unit TEXT,
    formula_or_definition TEXT,
    threshold_or_target TEXT,
    measurement_frequency TEXT,
    confidence DOUBLE PRECISION,
    is_explicit_metric BOOLEAN,
    reasoning_tags JSONB,
    ext_info JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s MetricsSQLStore) MetricsExist(ctx context.Context, inputRecordID int64, inputFilename string) (bool, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return false, err
	}
	const q = `
SELECT 1
FROM kb.metrics
WHERE input_record_id = $1
  AND ($2 = '' OR input_filename = $2)
LIMIT 1`
	var one int
	err := s.DB.QueryRowContext(ctx, q, inputRecordID, strings.TrimSpace(inputFilename)).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s MetricsSQLStore) DeleteMetricsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.metrics WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s MetricsSQLStore) SaveMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return 0, err
	}
	if len(req.Metrics) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.metrics (
	input_record_id,
	extract_id,
	input_filename,
	metric_name,
	source_line_spans,
	metric_subject,
	metric_desc,
	metric_context,
	metric_keywords,
	location_type,
	metric_unit,
	formula_or_definition,
	threshold_or_target,
	measurement_frequency,
	confidence,
	is_explicit_metric,
	reasoning_tags,
	ext_info
)
VALUES (
	$1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18::jsonb
)`

	var inserted int64
	for _, metric := range req.Metrics {
		sourceSpansJSON, _ := json.Marshal(metric["source_line_spans"])
		keywordsJSON, _ := json.Marshal(metric["keywords"])
		reasoningTagsJSON, _ := json.Marshal(metric["reasoning_tags"])
		extInfo, _ := json.Marshal(map[string]any{
			"table_name_or_section": metric["table_name_or_section"],
			"language":              req.Language,
			"schema_version":        "1",
		})

		_, err := s.DB.ExecContext(ctx, stmt,
			req.InputRecordID,
			req.ExtractID,
			req.InputFilename,
			strings.TrimSpace(asString(metric["metric_name"])),
			string(sourceSpansJSON),
			strings.TrimSpace(asString(metric["subject"])),
			strings.TrimSpace(asString(metric["desc"])),
			strings.TrimSpace(asString(metric["context"])),
			string(keywordsJSON),
			strings.TrimSpace(asString(metric["location_type"])),
			strings.TrimSpace(asString(metric["unit"])),
			strings.TrimSpace(asString(metric["formula_or_definition"])),
			strings.TrimSpace(asString(metric["threshold_or_target"])),
			strings.TrimSpace(asString(metric["measurement_frequency"])),
			toFloat(metric["confidence"]),
			toBool(metric["is_explicit_metric"]),
			string(reasoningTagsJSON),
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
