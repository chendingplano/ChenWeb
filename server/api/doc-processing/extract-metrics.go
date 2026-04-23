package docprocessing

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type MetricsProcessor struct {
	InputStore   DocMetadataStore
	Store        MetricsStore
	Extractor    LLMJSONExtractor
	Logger       *slog.Logger
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

func NewMetricsProcessor(inputStore DocMetadataStore, store MetricsStore, extractor LLMJSONExtractor, logger *slog.Logger) *MetricsProcessor {
	if logger == nil {
		logger = slog.Default()
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
		return fmt.Errorf("parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.PromptErr != nil {
		return fmt.Errorf("load metrics prompt %q: %w", p.PromptRef, p.PromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if err == sql.ErrNoRows {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.ModelErr != nil {
		p.persistMetricsStatus(ctx, rec, start, p.ModelErr)
		return nil
	}

	chunkFiles, err := p.listChunkFiles(evt.RecordID)
	if err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return nil
	}
	if len(chunkFiles) == 0 {
		err := fmt.Errorf("no chunk files found for record_id=%d", evt.RecordID)
		p.persistMetricsStatus(ctx, rec, start, err)
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

	allLines := make([]string, 0, 1024)
	for _, chunkPath := range chunkFiles {
		lines, err := readRegularLinesFromChunk(chunkPath)
		if err != nil {
			p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("read chunk file %s: %w", chunkPath, err))
			return nil
		}
		allLines = append(allLines, lines...)
	}
	if len(allLines) == 0 {
		err := fmt.Errorf("no regular lines found in chunk files for record_id=%d", evt.RecordID)
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
		"chunk_files", len(chunkFiles),
	)
	p.persistMetricsStatus(ctx, rec, start, nil)
	return nil
}

func (p *MetricsProcessor) listChunkFiles(recordID int64) ([]string, error) {
	if strings.TrimSpace(p.ChunkDir) == "" {
		return nil, errors.New("missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return nil, fmt.Errorf("invalid record_id: %d", recordID)
	}
	groupID := recordID / 1000
	dir := filepath.Join(p.ChunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if !strings.HasPrefix(name, "chunk_") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths, nil
}

func readRegularLinesFromChunk(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	lines := make([]string, 0, 256)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 16*1024*1024)
	for sc.Scan() {
		raw := strings.TrimSpace(sc.Text())
		if raw == "" {
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
		return metricExtractionResult{}, fmt.Errorf("extract metrics via llm: %w", err)
	}

	language := strings.TrimSpace(asString(payload["language"]))
	if language == "" {
		language = "unknown"
	}

	metricsRaw, ok := payload["metrics"].([]any)
	if !ok {
		return metricExtractionResult{}, fmt.Errorf("llm output field 'metrics' must be an array")
	}
	uncertainRaw, _ := payload["uncertain_metrics"].([]any)

	lineToPage := make(map[int]int, len(parsedLines))
	for _, line := range parsedLines {
		lineToPage[line.LineNumber] = line.PageNumber
	}

	metrics := normalizeMetricList(metricsRaw, lineToPage)
	uncertain := normalizeMetricList(uncertainRaw, lineToPage)
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
			"source_line_spans":     []string{"5:12", "5:13"},
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

func normalizeMetricList(items []any, lineToPage map[int]int) []map[string]any {
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
		normalized["source_line_spans"] = normalizeSourceLineSpans(raw["source_line_spans"], lineToPage)
		out = append(out, normalized)
	}
	return out
}

func normalizeSourceLineSpans(value any, lineToPage map[int]int) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case map[string]any:
			lineNo := int(toFloat(v["line_number"]))
			pageNo := int(toFloat(v["page_number"]))
			if lineNo > 0 && pageNo > 0 {
				out = append(out, fmt.Sprintf("%d:%d", pageNo, lineNo))
			}
		case float64:
			lineNo := int(v)
			if pageNo, ok := lineToPage[lineNo]; ok {
				out = append(out, fmt.Sprintf("%d:%d", pageNo, lineNo))
			}
		case string:
			parts := strings.SplitN(v, ":", 2)
			if len(parts) == 2 {
				p, pErr := strconv.Atoi(strings.TrimSpace(parts[0]))
				l, lErr := strconv.Atoi(strings.TrimSpace(parts[1]))
				if pErr == nil && lErr == nil {
					out = append(out, fmt.Sprintf("%d:%d", p, l))
				}
			}
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

	paths := []string{promptRef}
	paths = append(paths,
		filepath.Join("server", "cmd", "doc-processor", promptRef),
		filepath.Join("server", "cmd", "doc-processor", "prompts", promptRef),
		filepath.Join("python", "extract_metrics", "prompts", promptRef),
		filepath.Join("prompts", promptRef),
	)
	if filepath.IsAbs(promptRef) {
		paths = []string{promptRef}
	}

	var lastErr error
	for _, candidate := range paths {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("prompt file not found: %w", lastErr)
}

func (s MetricsSQLStore) ensureMetricsTable(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("db is nil")
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
