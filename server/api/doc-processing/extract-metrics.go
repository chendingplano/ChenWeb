package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type MetricsProcessor struct {
	InputStore                  DocMetadataStore
	Store                       MetricsStore
	Extractor                   LLMJSONExtractor
	Logger                      ApiTypes.JimoLogger
	Now                         func() time.Time
	MentionPromptText           string
	MentionPromptRef            string
	MentionPromptPath           string
	MentionPromptErr            error
	MentionModelRef             string
	MentionModelCfgPath         string
	MentionModelErr             error
	MentionModelName            string
	MentionModelCfg             structureModelConfig
	FallbackMentionModelRef     string
	FallbackMentionModelCfgPath string
	FallbackMentionModelErr     error
	FallbackMentionModelName    string
	FallbackMentionModelCfg     structureModelConfig
	RelationPromptText          string
	RelationPromptRef           string
	RelationPromptPath          string
	RelationPromptErr           error
	RelationModelRef            string
	RelationModelCfgPath        string
	RelationModelErr            error
	RelationModelName           string
	RelationModelCfg            structureModelConfig
	PromptText                  string
	PromptRef                   string
	PromptPath                  string
	PromptErr                   error
	ModelRef                    string
	ModelCfgPath                string
	ModelErr                    error
	ModelName                   string
	ChunkDir                    string
	ArtifactWebDir              string
}

type MetricsStore interface {
	MetricsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteMetricsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveMetrics(ctx context.Context, req SaveMetricsRequest) (int64, error)
}

type MetricsSQLStore struct {
	DB *sql.DB
}

type SaveMetricsRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Metrics       []map[string]any
}

type metricExtractionResult struct {
	Language         string
	Metrics          []map[string]any
	UncertainMetrics []map[string]any
	ModelName        string
}

type metricCandidateMention struct {
	MetricNameHint    string
	SubjectHint       string
	EvidenceQuote     string
	SourceLineSpans   []string
	UnitHint          string
	ValueHint         string
	Confidence        float64
	ConfidenceReason  string
	ChunkIndex        int
	BlockLines        []BlockLine
	HasNormalEvidence bool
}

type metricCandidate struct {
	CandidateID        string
	MetricNameHint     string
	SubjectHint        string
	UnitHint           string
	ValueHint          string
	SupportingMentions []map[string]any
	SupportLines       []BlockLine
}

func NewMetricsProcessor(inputStore DocMetadataStore, store MetricsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *MetricsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26042470")
	}
	mentionPromptText, mentionPromptRef, mentionPromptPath, mentionPromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_METRIC_CANDIDATES_PROMPT"},
		"prompt-extract-metric-candidates-v1.md",
	)
	relationPromptText, relationPromptRef, relationPromptPath, relationPromptErr := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_METRICS_PROMPT", "EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"},
		"prompt-enrich-metrics-v1.md",
	)
	mentionModelRef, mentionModelCfgPath, mentionModelCfg, mentionModelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_METRIC_CANDIDATES_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME"},
		"MODEL_CONFIG_FILE",
	)
	fallbackMentionModelRef, fallbackMentionModelCfgPath, fallbackMentionModelCfg, fallbackMentionModelErr := loadOptionalModelConfigFromEnv(
		"EXTRACT_METRIC_CANDIDATES_MODEL_FALLBACK",
		"MODEL_CONFIG_FILE",
	)
	relationModelRef, relationModelCfgPath, relationModelCfg, relationModelErr := loadModelConfigFromEnvKeys(
		[]string{"ENRICH_METRICS_MODEL_NAME", "EXTRACT_METRICS_MODEL_NAME"},
		"MODEL_CONFIG_FILE",
	)
	applyStructureModelConfigToExtractor(extractor, relationModelCfg)
	p := &MetricsProcessor{
		InputStore:                  inputStore,
		Store:                       store,
		Extractor:                   extractor,
		Logger:                      logger,
		Now:                         time.Now,
		MentionPromptText:           mentionPromptText,
		MentionPromptRef:            mentionPromptRef,
		MentionPromptPath:           mentionPromptPath,
		MentionPromptErr:            mentionPromptErr,
		MentionModelRef:             mentionModelRef,
		MentionModelCfgPath:         mentionModelCfgPath,
		MentionModelErr:             mentionModelErr,
		MentionModelName:            mentionModelCfg.ModelName,
		MentionModelCfg:             mentionModelCfg,
		FallbackMentionModelRef:     fallbackMentionModelRef,
		FallbackMentionModelCfgPath: fallbackMentionModelCfgPath,
		FallbackMentionModelErr:     fallbackMentionModelErr,
		FallbackMentionModelName:    fallbackMentionModelCfg.ModelName,
		FallbackMentionModelCfg:     fallbackMentionModelCfg,
		RelationPromptText:          relationPromptText,
		RelationPromptRef:           relationPromptRef,
		RelationPromptPath:          relationPromptPath,
		RelationPromptErr:           relationPromptErr,
		RelationModelRef:            relationModelRef,
		RelationModelCfgPath:        relationModelCfgPath,
		RelationModelErr:            relationModelErr,
		RelationModelName:           relationModelCfg.ModelName,
		RelationModelCfg:            relationModelCfg,
		PromptText:                  relationPromptText,
		PromptRef:                   relationPromptRef,
		PromptPath:                  relationPromptPath,
		PromptErr:                   relationPromptErr,
		ModelRef:                    relationModelRef,
		ModelCfgPath:                relationModelCfgPath,
		ModelErr:                    relationModelErr,
		ModelName:                   relationModelCfg.ModelName,
		ChunkDir:                    strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:              strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
	}
	p.forceDisableThinking()
	applyStructureModelConfigToExtractor(extractor, p.RelationModelCfg)
	return p
}

func (p *MetricsProcessor) Name() string { return "extract_metrics" }

func (p *MetricsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	p.forceDisableThinking()
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		p.Logger.Error("parse input file failed", "error", err)
		return fmt.Errorf("(MID_26042457) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		p.Logger.Warn("processor skipped")
		return nil
	}
	if p.MentionPromptErr != nil {
		p.Logger.Error("prompt error", "error", p.MentionPromptErr)
		return fmt.Errorf("(MID_26042458) load metric candidates prompt %q: %w", p.MentionPromptRef, p.MentionPromptErr)
	}
	if p.RelationPromptErr != nil {
		p.Logger.Error("prompt error", "error", p.RelationPromptErr)
		return fmt.Errorf("(MID_26042458) load metrics prompt %q: %w", p.RelationPromptRef, p.RelationPromptErr)
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if err == sql.ErrNoRows {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		p.Logger.Error("retrieve record failed", "record_id", evt.RecordID, "error", err)
		return fmt.Errorf("(MID_26042459) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.MentionModelErr != nil {
		p.Logger.Warn("metrics extraction skipped: mention model config error",
			"record_id", evt.RecordID, "model_ref", p.MentionModelRef, "error", p.MentionModelErr)
		p.persistMetricsStatus(ctx, rec, start, p.MentionModelErr)
		return nil
	}
	if p.RelationModelErr != nil {
		p.Logger.Warn("metrics extraction skipped: relation model config error",
			"record_id", evt.RecordID, "model_ref", p.RelationModelRef, "error", p.RelationModelErr)
		p.persistMetricsStatus(ctx, rec, start, p.RelationModelErr)
		return nil
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.Logger.Error("resolve input file path failed", "record_id", evt.RecordID, "error", lineFileErr)
		p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042466) resolve line file for record_id=%d: %w", evt.RecordID, lineFileErr))
		return nil
	}

	if evt.Force {
		_, _ = p.Store.DeleteMetricsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.MetricsExist(ctx, evt.RecordID)
		if err != nil {
			p.Logger.Error("check metrics exist failed", "record_id", evt.RecordID, "error", err)
			p.persistMetricsStatus(ctx, rec, start, err)
			return fmt.Errorf("(MID_26050707) failed checking metrics, error:%w, record_id:%d", err, evt.RecordID)
		}
		if exists {
			p.Logger.Info("metrics extraction skipped", "record_id", evt.RecordID, "reason", "metrics already exist and force=false")
			p.persistMetricsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	// Get blocks from context (set by BlockingProcessor), or build them from the line file.
	buf := BlockBufferFromContext(ctx)
	if buf == nil {
		body, readErr := os.ReadFile(lineFilePath)
		if readErr != nil {
			p.Logger.Error("read line file failed", "record_id", evt.RecordID, "path", lineFilePath, "error", readErr)
			p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042466) read line file for record_id=%d: %w", evt.RecordID, readErr))
			return fmt.Errorf("(MID_26050708) failed reading line file, error:%w, path:%s", readErr, lineFilePath)
		}
		bPrev, bNext, bRemoveTOC := blockingConfigFromViper()
		buf, err = buildBlocks(body, envInt("INPUT_BLOCK_SIZE", DefaultBlockingBlockSize, 1), bPrev, bNext, bRemoveTOC)
		if err != nil {
			p.Logger.Error("build blocks failed", "record_id", evt.RecordID, "error", err)
			p.persistMetricsStatus(ctx, rec, start, fmt.Errorf("(MID_26042461) build blocks for record_id=%d: %w", evt.RecordID, err))
			return fmt.Errorf("(MID_26050702) failed building blocks, error:%w", err)
		}
	}
	if len(buf.Blocks) == 0 {
		err := fmt.Errorf("(MID_26042460) no blocks found for record_id=%d", evt.RecordID)
		p.Logger.Error("no blocks", "record_id", evt.RecordID, "line_file", lineFilePath)
		p.persistMetricsStatus(ctx, rec, start, err)
		return nil
	}

	result, err := p.extractMetricsFromBlocksWithLLM(ctx, buf.Blocks)
	if err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26050701) extractMetrics failed, error:%w", err)
	}
	allMetrics := result.Metrics
	detectedLanguage := result.Language
	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}

	for i, m := range allMetrics {
		m["metric_id"] = fmt.Sprintf("%d_%d", evt.RecordID, i+1)
		allMetrics[i] = m
	}

	inserted, err := p.Store.SaveMetrics(ctx, SaveMetricsRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.RelationPromptRef,
		Metrics:       allMetrics,
	})
	if err != nil {
		p.persistMetricsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26050703) insert records failed, error:%w", err)
	}

	if fileErr := p.saveMetricsToFile(evt.RecordID, rec, allMetrics); fileErr != nil {
		p.Logger.Warn("save metrics to file failed", "record_id", evt.RecordID, "error", fileErr)
	}

	if indexErr := p.indexMetrics(evt.RecordID, allMetrics); indexErr != nil {
		p.Logger.Warn("index metrics failed", "record_id", evt.RecordID, "error", indexErr)
	}

	p.Logger.Info("metrics extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"metrics_count", len(allMetrics),
		"uncertain_metrics_count", len(result.UncertainMetrics),
		"num_blocks", len(buf.Blocks),
	)
	p.persistMetricsStatus(ctx, rec, start, nil)
	return nil
}

func forceDisableThinking(cfg structureModelConfig) structureModelConfig {
	cfg.ThinkingType = "disabled"
	return cfg
}

func (p *MetricsProcessor) forceDisableThinking() {
	p.MentionModelCfg = forceDisableThinking(p.MentionModelCfg)
	p.FallbackMentionModelCfg = forceDisableThinking(p.FallbackMentionModelCfg)
	p.RelationModelCfg = forceDisableThinking(p.RelationModelCfg)
}

func (p *MetricsProcessor) saveMetricsToFile(recordID int64, rec DocMetadataInputRecord, metrics []map[string]any) error {
	if len(metrics) == 0 {
		return nil
	}
	if strings.TrimSpace(p.ChunkDir) == "" {
		return fmt.Errorf("(MID_26042480) missing ARTIFACT_DIR")
	}

	stagingBase := filepath.Base(strings.TrimSpace(rec.StagingFilename))
	filenameRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if filenameRoot == "" {
		return fmt.Errorf("(MID_26042481) cannot determine filename root from staging_filename %q", rec.StagingFilename)
	}

	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		return fmt.Errorf("(MID_26042482) parser_name is empty for record_id=%d", recordID)
	}

	groupID := recordID / 1000
	dir := filepath.Join(p.ChunkDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("(MID_26042483) create directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, filenameRoot+"_"+parserName+".metrics")
	bs, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26042484) marshal metrics: %w", err)
	}
	if err := os.WriteFile(path, bs, 0644); err != nil {
		return fmt.Errorf("(MID_26042485) write metrics file %s: %w", path, err)
	}
	return nil
}

type metricsStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcErr       error
}

func detectMetricsFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *MetricsProcessor) persistMetricsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	statusRaw, err := appendMetricsStatus(rec.StatusRaw, metricsStatusParams{
		RecordID:      rec.ID,
		FileType:      detectMetricsFileType(rec),
		InputFilename: strings.TrimSpace(rec.ResultFilename),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
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

func appendMetricsStatus(raw string, p metricsStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_metrics",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
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

/*
func (p *MetricsProcessor) extractMetricsFromLinesWithLLM(ctx context.Context, lines []string) (metricExtractionResult, error) {
	parsedLines := parseMetricInputLines(lines)
	blockLines := make([]BlockLine, 0, len(parsedLines))
	for _, line := range parsedLines {
		blockLines = append(blockLines, BlockLine{
			Flag:       line.Flag,
			LineNumber: line.LineNumber,
			PageNumber: line.PageNumber,
			LineType:   line.LineType,
			Content:    line.Content,
		})
	}
	return p.extractMetricsFromBlocksWithLLM(ctx, []Block{{Index: 1, Lines: blockLines}})
}
*/

func (p *MetricsProcessor) extractMetricsFromBlocksWithLLM(ctx context.Context, blocks []Block) (metricExtractionResult, error) {
	mentions := make([]metricCandidateMention, 0, len(blocks))
	usedMentionModel := strings.TrimSpace(p.MentionModelName)
	detectedLanguage := "unknown"
	for _, block := range blocks {
		lines := make([]string, len(block.Lines))
		for i, bl := range block.Lines {
			lines[i] = bl.String()
		}
		parsedLines := parseMetricInputLines(lines)
		userPrompt := buildMetricCandidateUserPrompt(lines, parsedLines)
		p.Logger.Info("Ready to extract metric candidates",
			"model name", p.MentionModelName,
			"num_lines", len(lines),
			"prompt", p.MentionPromptRef)
		payload, modelName, err := p.extractMetricCandidatePayloadWithFallback(ctx, userPrompt)
		if err != nil {
			return metricExtractionResult{}, fmt.Errorf("(MID_26042451) extract metric candidates via llm: %w", err)
		}
		if language := strings.TrimSpace(asString(payload["language"])); language != "" && detectedLanguage == "unknown" {
			detectedLanguage = language
		}
		usedMentionModel = strings.TrimSpace(firstNonEmptyTrimmed(modelName, usedMentionModel, p.MentionModelName))
		raw, _ := payload["candidates"].([]any)
		mentions = append(mentions, normalizeMetricCandidateMentions(raw, block)...)
	}

	candidates := mergeMetricMentionCandidates(mentions)
	p.Logger.Info("Merged metric candidates",
		"mentions_count", len(mentions),
		"candidate_count", len(candidates),
		"record_stage", "post_merge",
	)
	metrics := make([]map[string]any, 0, len(candidates))
	uncertain := make([]map[string]any, 0)
	usedRelationModel := strings.TrimSpace(p.RelationModelName)
	for idx, candidate := range candidates {
		p.Logger.Info("Start enriching metric candidate",
			"idx", idx,
			"total", len(candidates),
			"candidate_id", candidate.CandidateID,
			"model_name", p.RelationModelName,
			"prompt_name", p.RelationPromptRef,
		)
		payload, err := p.extractMetricPayload(ctx, buildMetricRelationUserPrompt(candidate), p.RelationPromptText, p.RelationModelName, p.RelationModelCfg)
		if err != nil {
			return metricExtractionResult{}, fmt.Errorf("(MID_26042452) enrich metrics via llm: %w", err)
		}
		if language := strings.TrimSpace(asString(payload["language"])); language != "" && detectedLanguage == "unknown" {
			detectedLanguage = language
		}
		usedRelationModel = strings.TrimSpace(firstNonEmptyTrimmed(usedRelationModel, p.RelationModelName))
		metricsRaw, _ := payload["metrics"].([]any)
		uncertainRaw, _ := payload["uncertain_metrics"].([]any)
		metrics = append(metrics, normalizeMetricList(metricsRaw)...)
		uncertain = append(uncertain, normalizeMetricList(uncertainRaw)...)
		p.Logger.Info("LLM responded with enriched metrics",
			"candidate_id", candidate.CandidateID,
			"metrics_so_far", len(metrics),
			"uncertain_metrics_so_far", len(uncertain),
			"model_name", p.RelationModelName,
			"prompt_name", p.RelationPromptRef,
		)
	}

	preDedupeMetrics := len(metrics)
	preDedupeUncertain := len(uncertain)
	metrics = dedupeFinalMetricRows(metrics)
	uncertain = dedupeFinalMetricRows(uncertain)
	p.Logger.Info("Deduped final metric rows",
		"metrics_before_dedup", preDedupeMetrics,
		"metrics_after_dedup", len(metrics),
		"uncertain_before_dedup", preDedupeUncertain,
		"uncertain_after_dedup", len(uncertain),
		"record_stage", "post_metric_dedup",
	)

	return metricExtractionResult{
		Language:         detectedLanguage,
		Metrics:          metrics,
		UncertainMetrics: uncertain,
		ModelName:        firstNonEmptyTrimmed(usedRelationModel, usedMentionModel, p.RelationModelName, p.ModelName),
	}, nil
}

// metricParsedLine mirrors the block format: <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
type metricParsedLine struct {
	Flag       string `json:"flag"`
	LineNumber int    `json:"line_number"`
	PageNumber int    `json:"page_number"`
	LineType   string `json:"line_type"`
	Content    string `json:"content"`
}

func parseMetricInputLine(line string) (metricParsedLine, bool) {
	raw := strings.TrimSpace(line)
	if raw == "" {
		return metricParsedLine{}, false
	}
	// Block format: <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
	fields := strings.Split(raw, "\t")
	if len(fields) != 5 {
		return metricParsedLine{}, false
	}
	flag := strings.TrimSpace(fields[0])
	if flag != "o" && flag != "n" {
		return metricParsedLine{}, false
	}
	lineNo, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil || lineNo < 1 {
		return metricParsedLine{}, false
	}
	pageNo, err := strconv.Atoi(strings.TrimSpace(fields[2]))
	if err != nil || pageNo < 1 {
		return metricParsedLine{}, false
	}
	lineType := strings.TrimSpace(fields[3])
	if lineType == "" {
		return metricParsedLine{}, false
	}
	return metricParsedLine{
		Flag:       flag,
		LineNumber: lineNo,
		PageNumber: pageNo,
		LineType:   lineType,
		Content:    strings.TrimSpace(fields[4]),
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

func buildMetricCandidateUserPrompt(lines []string, parsedLines []metricParsedLine) string {
	schema := map[string]any{
		"language": "string",
		"candidates": []map[string]any{{
			"metric_name_hint":  "string",
			"subject_hint":      "string",
			"evidence_quote":    "string",
			"source_line_spans": []string{"5", "12:14"},
			"unit_hint":         "string",
			"value_hint":        "string",
			"confidence":        0.0,
			"confidence_reason": "string",
		}},
	}

	linesJSON, _ := json.Marshal(lines)
	parsedLinesJSON, _ := json.Marshal(parsedLines)
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nInput lines (raw, JSON array):\n" + string(linesJSON) +
		"\n\nParsed line hints (best effort; do not treat as complete):\n" + string(parsedLinesJSON)
}

/*
func buildMetricUserPrompt(lines []string, parsedLines []metricParsedLine) string {
	schema := map[string]any{
		"language": "string",
		"metrics": []map[string]any{{
			"metric_name":           "string",
			"metric_name_en":        "string",
			"source_line_spans":     []string{"5", "12:14"},
			"subject":               "string",
			"subject_en":            "string",
			"desc":                  "string",
			"desc_en":               "string",
			"context":               "string",
			"context_en":            "string",
			"keywords":              []string{"string"},
			"keywords_en":           []string{"string"},
			"location_type":         "string",
			"unit":                  "string",
			"unit_en":               "string",
			"metric_value":          "string",
			"value_data_type":       "string",
			"value_range_type":      "string",
			"value_class":           "string",
			"value_class_en":        "string",
			"formula_or_definition": "string",
			"threshold_or_target":   "string",
			"measurement_frequency": "string",
			"confidence":            0.0,
			"is_explicit_metric":    true,
			"table_name_or_section": "string",
			"reasoning_tags":        []string{"string"},
			"category_paths": []map[string]any{{
				"category_path": []map[string]any{{
					"name":       "string",
					"keywords":   []string{"string"},
					"confidence": 0.0,
				}},
				"path_keywords":   []string{"string"},
				"path_confidence": 0.0,
			}},
			"category_paths_en": []map[string]any{{
				"category_path": []map[string]any{{
					"name":       "string",
					"keywords":   []string{"string"},
					"confidence": 0.0,
				}},
				"path_keywords":   []string{"string"},
				"path_confidence": 0.0,
			}},
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
*/

func buildMetricRelationUserPrompt(candidate metricCandidate) string {
	lines := make([]string, 0, len(candidate.SupportLines))
	for _, line := range candidate.SupportLines {
		lines = append(lines, line.String())
	}
	linesJSON, _ := json.Marshal(lines)
	candidateJSON, _ := json.Marshal(map[string]any{
		"candidate_id":        candidate.CandidateID,
		"metric_name_hint":    candidate.MetricNameHint,
		"subject_hint":        candidate.SubjectHint,
		"unit_hint":           candidate.UnitHint,
		"value_hint":          candidate.ValueHint,
		"supporting_mentions": candidate.SupportingMentions,
	})
	return "Return JSON only.\n\nCandidate:\n" + string(candidateJSON) +
		"\n\nSource lines (plain):\n" + strings.Join(lines, "\n") +
		"\n\nSource lines (raw, JSON array):\n" + string(linesJSON)
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
			"metric_name_en":        strings.TrimSpace(asString(raw["metric_name_en"])),
			"subject":               strings.TrimSpace(asString(raw["subject"])),
			"subject_en":            strings.TrimSpace(asString(raw["subject_en"])),
			"desc":                  strings.TrimSpace(asString(raw["desc"])),
			"desc_en":               strings.TrimSpace(asString(raw["desc_en"])),
			"context":               strings.TrimSpace(asString(raw["context"])),
			"context_en":            strings.TrimSpace(asString(raw["context_en"])),
			"keywords":              toStringSlice(raw["keywords"]),
			"keywords_en":           toStringSlice(raw["keywords_en"]),
			"location_type":         strings.TrimSpace(asString(raw["location_type"])),
			"unit":                  strings.TrimSpace(asString(raw["unit"])),
			"unit_en":               strings.TrimSpace(asString(raw["unit_en"])),
			"metric_value":          strings.TrimSpace(asString(raw["metric_value"])),
			"value_data_type":       strings.TrimSpace(asString(raw["value_data_type"])),
			"value_range_type":      strings.TrimSpace(asString(raw["value_range_type"])),
			"value_class":           strings.TrimSpace(asString(raw["value_class"])),
			"value_class_en":        strings.TrimSpace(asString(raw["value_class_en"])),
			"formula_or_definition": strings.TrimSpace(asString(raw["formula_or_definition"])),
			"threshold_or_target":   strings.TrimSpace(asString(raw["threshold_or_target"])),
			"measurement_frequency": strings.TrimSpace(asString(raw["measurement_frequency"])),
			"confidence":            toFloat(raw["confidence"]),
			"is_explicit_metric":    toBool(raw["is_explicit_metric"]),
			"table_name_or_section": strings.TrimSpace(asString(raw["table_name_or_section"])),
			"reasoning_tags":        toStringSlice(raw["reasoning_tags"]),
		}
		normalized["source_line_spans"] = normalizeSourceLineSpans(raw["source_line_spans"])
		normalized["category_paths"] = raw["category_paths"]
		// handle spec typo "caetgory_paths_en"
		catPathsEn := raw["category_paths_en"]
		if catPathsEn == nil {
			catPathsEn = raw["caetgory_paths_en"]
		}
		normalized["category_paths_en"] = catPathsEn
		out = append(out, normalized)
	}
	return out
}

func normalizeMetricCandidateMentions(items []any, block Block) []metricCandidateMention {
	out := make([]metricCandidateMention, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		spans := normalizeSourceLineSpans(raw["source_line_spans"])
		quote := strings.TrimSpace(asString(raw["evidence_quote"]))
		out = append(out, metricCandidateMention{
			MetricNameHint:    strings.TrimSpace(asString(raw["metric_name_hint"])),
			SubjectHint:       strings.TrimSpace(asString(raw["subject_hint"])),
			EvidenceQuote:     quote,
			SourceLineSpans:   spans,
			UnitHint:          strings.TrimSpace(asString(raw["unit_hint"])),
			ValueHint:         strings.TrimSpace(asString(raw["value_hint"])),
			Confidence:        toFloat(raw["confidence"]),
			ConfidenceReason:  strings.TrimSpace(asString(raw["confidence_reason"])),
			ChunkIndex:        block.Index,
			BlockLines:        append([]BlockLine(nil), block.Lines...),
			HasNormalEvidence: metricCandidateHasNormalEvidence(block, spans, quote),
		})
	}
	return out
}

func metricCandidateHasNormalEvidence(block Block, spans []string, quote string) bool {
	lineNums := make(map[int]struct{})
	for _, span := range spans {
		start, end, ok := parseMetricLineSpan(span)
		if !ok {
			continue
		}
		for i := start; i <= end; i++ {
			lineNums[i] = struct{}{}
		}
	}
	for _, line := range block.Lines {
		if line.Flag != "n" {
			continue
		}
		if len(lineNums) == 0 {
			if quote == "" || strings.Contains(strings.ToLower(line.Content), strings.ToLower(quote)) {
				return true
			}
			continue
		}
		if _, ok := lineNums[line.LineNumber]; ok {
			return true
		}
	}
	return false
}

func parseMetricLineSpan(span string) (int, int, bool) {
	span = strings.TrimSpace(span)
	if span == "" {
		return 0, 0, false
	}
	if strings.Contains(span, ":") {
		parts := strings.SplitN(span, ":", 2)
		start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || end < start {
			return 0, 0, false
		}
		return start, end, true
	}
	n, err := strconv.Atoi(span)
	if err != nil {
		return 0, 0, false
	}
	return n, n, true
}

func mergeMetricMentionCandidates(mentions []metricCandidateMention) []metricCandidate {
	type bucket struct {
		mentions []metricCandidateMention
	}
	grouped := map[string]*bucket{}
	order := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		key := normalizedMetricCandidateKey(mention.MetricNameHint, mention.SubjectHint, mention.UnitHint, mention.ValueHint)
		if key == "" {
			continue
		}
		if grouped[key] == nil {
			grouped[key] = &bucket{}
			order = append(order, key)
		}
		grouped[key].mentions = append(grouped[key].mentions, mention)
	}

	out := make([]metricCandidate, 0, len(order))
	for _, key := range order {
		b := grouped[key]
		if b == nil || len(b.mentions) == 0 {
			continue
		}
		hasNormal := false
		for _, mention := range b.mentions {
			if mention.HasNormalEvidence {
				hasNormal = true
				break
			}
		}
		if !hasNormal {
			continue
		}
		supportMentions := make([]map[string]any, 0, len(b.mentions))
		lineMap := map[string]BlockLine{}
		metricName := ""
		subject := ""
		unit := ""
		value := ""
		for _, mention := range b.mentions {
			metricName = firstNonEmptyTrimmed(metricName, mention.MetricNameHint)
			subject = firstNonEmptyTrimmed(subject, mention.SubjectHint)
			unit = firstNonEmptyTrimmed(unit, mention.UnitHint)
			value = firstNonEmptyTrimmed(value, mention.ValueHint)
			supportMentions = append(supportMentions, map[string]any{
				"metric_name_hint":  mention.MetricNameHint,
				"subject_hint":      mention.SubjectHint,
				"evidence_quote":    mention.EvidenceQuote,
				"source_line_spans": mention.SourceLineSpans,
				"unit_hint":         mention.UnitHint,
				"value_hint":        mention.ValueHint,
				"confidence":        mention.Confidence,
				"confidence_reason": mention.ConfidenceReason,
			})
			for _, line := range mention.BlockLines {
				lineKey := fmt.Sprintf("%d:%d:%s", line.PageNumber, line.LineNumber, line.Content)
				existing, exists := lineMap[lineKey]
				if !exists || (existing.Flag != "n" && line.Flag == "n") {
					lineMap[lineKey] = line
				}
			}
		}
		supportLines := make([]BlockLine, 0, len(lineMap))
		for _, line := range lineMap {
			supportLines = append(supportLines, line)
		}
		sort.Slice(supportLines, func(i, j int) bool {
			if supportLines[i].PageNumber != supportLines[j].PageNumber {
				return supportLines[i].PageNumber < supportLines[j].PageNumber
			}
			if supportLines[i].LineNumber != supportLines[j].LineNumber {
				return supportLines[i].LineNumber < supportLines[j].LineNumber
			}
			return supportLines[i].Flag < supportLines[j].Flag
		})
		out = append(out, metricCandidate{
			CandidateID:        fmt.Sprintf("metric_cand_%d", len(out)+1),
			MetricNameHint:     metricName,
			SubjectHint:        subject,
			UnitHint:           unit,
			ValueHint:          value,
			SupportingMentions: supportMentions,
			SupportLines:       supportLines,
		})
	}
	return out
}

func normalizedMetricCandidateKey(parts ...string) string {
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ToLower(strings.TrimSpace(part))
		part = strings.Join(strings.Fields(part), " ")
		if part != "" {
			normalized = append(normalized, part)
		}
	}
	return strings.Join(normalized, "|")
}

func dedupeFinalMetricRows(metrics []map[string]any) []map[string]any {
	grouped := map[string]map[string]any{}
	order := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		key := normalizedMetricCandidateKey(
			asString(metric["metric_name"]),
			asString(metric["subject"]),
			asString(metric["unit"]),
			asString(metric["metric_value"]),
			strings.Join(normalizeSourceLineSpans(metric["source_line_spans"]), ","),
		)
		if existing, ok := grouped[key]; ok {
			mergedSpans := normalizeSourceLineSpans(existing["source_line_spans"])
			for _, span := range normalizeSourceLineSpans(metric["source_line_spans"]) {
				if !slices.Contains(mergedSpans, span) {
					mergedSpans = append(mergedSpans, span)
				}
			}
			existing["source_line_spans"] = mergedSpans
			if toFloat(metric["confidence"]) > toFloat(existing["confidence"]) {
				existing["confidence"] = metric["confidence"]
			}
			continue
		}
		cloned := map[string]any{}
		for k, v := range metric {
			cloned[k] = v
		}
		grouped[key] = cloned
		order = append(order, key)
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		out = append(out, grouped[key])
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

func (p *MetricsProcessor) extractMetricPayload(ctx context.Context, inputText string, promptText string, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	payload, err := p.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  inputText,
	})
	p.Logger.Info("LLM response received for metrics extraction",
		"model_name", modelName,
		"prompt_name", p.PromptRef,
		"extract_error", err,
	)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errors.New("(MID_26042490) empty llm payload")
	}
	if _, ok := payload["metrics"]; ok {
		return payload, nil
	}
	if _, ok := payload["candidates"]; ok {
		return payload, nil
	}
	return nil, fmt.Errorf("(MID_26042491) llm output must contain 'metrics' or 'candidates'")
}

func firstPromptLine(promptText string) string {
	promptText = strings.TrimSpace(promptText)
	if promptText == "" {
		return ""
	}
	if idx := strings.IndexByte(promptText, '\n'); idx >= 0 {
		return strings.TrimSpace(promptText[:idx])
	}
	return promptText
}

func (p *MetricsProcessor) extractMetricCandidatePayloadWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	payload, err := p.extractMetricPayload(ctx, inputText, p.MentionPromptText, p.MentionModelName, p.MentionModelCfg)
	if err == nil {
		return payload, strings.TrimSpace(p.MentionModelName), nil
	}

	primaryModelName := strings.TrimSpace(p.MentionModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackMentionModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackMentionModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26042492) primary metric candidate extraction failed and fallback model %q is unavailable: %w", p.FallbackMentionModelRef, err)
	}

	if isEmptyMetricExtractionError(err) {
		p.Logger.Warn("primary metric candidate extraction returned empty JSON; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.MentionPromptRef,
		)
	} else {
		p.Logger.Warn("primary metric candidate extraction failed; retrying fallback model",
			"primary_model", primaryModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.MentionPromptRef,
		)
	}

	payload, fallbackErr := p.extractMetricPayload(ctx, inputText, p.MentionPromptText, fallbackModelName, p.FallbackMentionModelCfg)
	if fallbackErr != nil {
		if isEmptyMetricExtractionError(fallbackErr) {
			p.Logger.Warn("fallback metric candidate extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", p.MentionPromptRef,
			)
			return map[string]any{"language": "unknown", "candidates": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26042493) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func isEmptyMetricExtractionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return strings.Contains(msg, "unexpected end of JSON input") &&
		strings.Contains(msg, "json:{[]}")
}

func loadMetricsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_METRICS_PROMPT", "PROMPT_FILE_NAME"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = "prompt-enrich-metrics-v1.md"
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
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    metric_id TEXT,
    metric_name TEXT,
    metric_name_en TEXT,
    source_line_spans JSONB,
    metric_subject TEXT,
    metric_subject_en TEXT,
    metric_desc TEXT,
    metric_desc_en TEXT,
    metric_context TEXT,
    metric_context_en TEXT,
    metric_keywords JSONB,
    metric_keywords_en JSONB,
    model_name TEXT,
    prompt_name TEXT,
    location_type TEXT,
    metric_unit TEXT,
    metric_unit_en TEXT,
    metric_value TEXT,
    value_data_type TEXT,
    value_range_type TEXT,
    value_class TEXT,
    value_class_en TEXT,
    formula_or_definition TEXT,
    threshold_or_target TEXT,
    measurement_frequency TEXT,
    confidence DOUBLE PRECISION,
    is_explicit_metric BOOLEAN,
    table_name_or_section TEXT,
    reasoning_tags JSONB,
    category_paths JSONB,
    category_paths_en JSONB,
    ext_info JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s MetricsSQLStore) MetricsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureMetricsTable(ctx); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.metrics WHERE input_record_id = $1 LIMIT 1`
	var one int
	err := s.DB.QueryRowContext(ctx, q, inputRecordID).Scan(&one)
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
	event_id,
	input_record_id,
	metric_id,
	metric_name,
	metric_name_en,
	source_line_spans,
	metric_subject,
	metric_subject_en,
	metric_desc,
	metric_desc_en,
	metric_context,
	metric_context_en,
	metric_keywords,
	metric_keywords_en,
	model_name,
	prompt_name,
	location_type,
	metric_unit,
	metric_unit_en,
	metric_value,
	value_data_type,
	value_range_type,
	value_class,
	value_class_en,
	formula_or_definition,
	threshold_or_target,
	measurement_frequency,
	confidence,
	is_explicit_metric,
	table_name_or_section,
	reasoning_tags,
	category_paths,
	category_paths_en,
	ext_info
)
VALUES (
	$1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31::jsonb,$32::jsonb,$33::jsonb,$34::jsonb
)`

	isEnglish := strings.EqualFold(strings.TrimSpace(req.Language), "en") ||
		strings.EqualFold(strings.TrimSpace(req.Language), "english")

	var eventIDVal interface{}
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}

	var inserted int64
	for _, metric := range req.Metrics {
		sourceSpansJSON, _ := json.Marshal(metric["source_line_spans"])
		keywordsJSON, _ := json.Marshal(metric["keywords"])
		reasoningTagsJSON, _ := json.Marshal(metric["reasoning_tags"])
		categoryPathsJSON, _ := json.Marshal(metric["category_paths"])
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": "2",
		})

		var (
			metricNameEn       any
			subjectEn          any
			descEn             any
			contextEn          any
			keywordsEnVal      any
			unitEn             any
			valueClassEn       any
			categoryPathsEnVal any
		)
		if !isEnglish {
			metricNameEn = strings.TrimSpace(asString(metric["metric_name_en"]))
			subjectEn = strings.TrimSpace(asString(metric["subject_en"]))
			descEn = strings.TrimSpace(asString(metric["desc_en"]))
			contextEn = strings.TrimSpace(asString(metric["context_en"]))
			kw, _ := json.Marshal(metric["keywords_en"])
			keywordsEnVal = string(kw)
			unitEn = strings.TrimSpace(asString(metric["unit_en"]))
			valueClassEn = strings.TrimSpace(asString(metric["value_class_en"]))
			cp, _ := json.Marshal(metric["category_paths_en"])
			categoryPathsEnVal = string(cp)
		}

		_, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(metric["metric_id"])),
			strings.TrimSpace(asString(metric["metric_name"])),
			metricNameEn,
			string(sourceSpansJSON),
			strings.TrimSpace(asString(metric["subject"])),
			subjectEn,
			strings.TrimSpace(asString(metric["desc"])),
			descEn,
			strings.TrimSpace(asString(metric["context"])),
			contextEn,
			string(keywordsJSON),
			keywordsEnVal,
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			strings.TrimSpace(asString(metric["location_type"])),
			strings.TrimSpace(asString(metric["unit"])),
			unitEn,
			strings.TrimSpace(asString(metric["metric_value"])),
			strings.TrimSpace(asString(metric["value_data_type"])),
			strings.TrimSpace(asString(metric["value_range_type"])),
			strings.TrimSpace(asString(metric["value_class"])),
			valueClassEn,
			strings.TrimSpace(asString(metric["formula_or_definition"])),
			strings.TrimSpace(asString(metric["threshold_or_target"])),
			strings.TrimSpace(asString(metric["measurement_frequency"])),
			toFloat(metric["confidence"]),
			toBool(metric["is_explicit_metric"]),
			strings.TrimSpace(asString(metric["table_name_or_section"])),
			string(reasoningTagsJSON),
			string(categoryPathsJSON),
			categoryPathsEnVal,
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

func (p *MetricsProcessor) indexMetrics(recordID int64, metrics []map[string]any) error {
	if strings.TrimSpace(p.ArtifactWebDir) == "" {
		return fmt.Errorf("(MID_26051801) missing ARTIFACT_WEB_DIR")
	}
	return indexMetricsInTreeDir(p.Logger, p.ArtifactWebDir, recordID, metrics)
}

func indexMetricsInTreeDir(logger ApiTypes.JimoLogger, treeRootDir string, recordID int64, metrics []map[string]any) error {
	if strings.TrimSpace(treeRootDir) == "" {
		return fmt.Errorf("(MID_26051802) metric tree root dir is empty")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26051803) invalid record id: %d", recordID)
	}
	if err := os.MkdirAll(treeRootDir, 0o755); err != nil {
		return err
	}
	if err := removeMetricTreeRecord(treeRootDir, recordID); err != nil {
		return fmt.Errorf("(MID_26051804) remove old metric tree entries for record %d: %w", recordID, err)
	}
	now := time.Now()
	for _, metric := range metrics {
		metricID := strings.TrimSpace(asString(metric["metric_id"]))
		if metricID == "" {
			continue
		}
		if pathsRaw, ok := metric["category_paths"].([]any); ok {
			for _, entry := range parseCategoryPathsArray(pathsRaw) {
				if err := writeMetricTreeEntry(logger, treeRootDir, metricID, entry, now); err != nil {
					return fmt.Errorf("(MID_26051805) index metric %s path: %w", metricID, err)
				}
			}
		}
		if pathsEnRaw, ok := metric["category_paths_en"].([]any); ok {
			for _, entry := range parseCategoryPathsArray(pathsEnRaw) {
				if err := writeMetricTreeEntry(logger, treeRootDir, metricID, entry, now); err != nil {
					return fmt.Errorf("(MID_26051806) index metric %s en path: %w", metricID, err)
				}
			}
		}
	}
	return nil
}

func writeMetricTreeEntry(logger ApiTypes.JimoLogger, treeRootDir string, metricID string, pathEntry CategoryPathEntry, now time.Time) error {
	if len(pathEntry.Nodes) == 0 {
		return nil
	}
	currentDir := treeRootDir
	for _, node := range pathEntry.Nodes {
		subdir, err := findOrCreateCategorySubdir(logger, currentDir, node, now)
		if err != nil {
			return err
		}
		currentDir = subdir
	}
	return upsertMetricToLeafDir(currentDir, metricID)
}

func upsertMetricToLeafDir(leafDir string, metricID string) error {
	filePath := filepath.Join(leafDir, "metrics.txt")
	existing := make([]string, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, metricID)
	sort.Strings(existing)
	return os.WriteFile(filePath, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeMetricTreeRecord(treeRootDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "metrics.txt" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rows := make([]string, 0)
		for _, row := range strings.Split(string(body), "\n") {
			row = strings.TrimSpace(row)
			if row == "" || strings.HasPrefix(row, prefix) {
				continue
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		sort.Strings(rows)
		return os.WriteFile(path, []byte(strings.Join(rows, "\n")), 0o644)
	})
}
