package docprocessing

import (
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

type ProvisionsProcessor struct {
	InputStore           DocMetadataStore
	Store                ProvisionsStore
	Extractor            LLMJSONExtractor
	Logger               ApiTypes.JimoLogger
	ProcLogger           DocProcLogger
	Now                  func() time.Time
	PromptText           string
	PromptRef            string
	PromptPath           string
	PromptErr            error
	ModelRef             string
	ModelCfgPath         string
	ModelErr             error
	ModelName            string
	FallbackModelRef     string
	FallbackModelCfgPath string
	FallbackModelErr     error
	FallbackModelName    string
	BlockSize            int
	PrevOverlap          int
	NextOverlap          int
	RemoveTOC            bool
	ArtifactDir          string
	ArtifactWebDir       string
}

type ProvisionsStore interface {
	ProvisionsExist(ctx context.Context, inputRecordID int64, inputFilename string) (bool, error)
	DeleteProvisionsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveProvisions(ctx context.Context, req SaveProvisionsRequest) (int64, error)
}

type ProvisionsSQLStore struct {
	DB *sql.DB
}

type SaveProvisionsRequest struct {
	InputRecordID int64
	InputFilename string
	ExtractID     string
	Language      string
	Provisions    []map[string]any
}

type provisionExtractionResult struct {
	ExtractID     string
	Language      string
	Provisions    []map[string]any
	ModelName     string
	LLMCallCount  int
	FallbackCount int
}

func NewProvisionsProcessor(inputStore DocMetadataStore, store ProvisionsStore, extractor LLMJSONExtractor, _ ApiTypes.JimoLogger) *ProvisionsProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26050520")
	promptText, promptRef, promptPath, promptErr := loadProvisionsPromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_PROVISIONS_MODEL_NAME", "EXTRACT_PROVISIONS_MODELS_FILE")
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_PROVISIONS_MODEL_FALLBACK", "EXTRACT_PROVISIONS_MODELS_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	prevOverlap, nextOverlap, removeTOC := blockingConfigFromViper()
	return &ProvisionsProcessor{
		InputStore:           inputStore,
		Store:                store,
		Extractor:            extractor,
		Logger:               logger,
		ProcLogger:           DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                  time.Now,
		PromptText:           promptText,
		PromptRef:            promptRef,
		PromptPath:           promptPath,
		PromptErr:            promptErr,
		ModelRef:             modelRef,
		ModelCfgPath:         modelCfgPath,
		ModelErr:             modelErr,
		ModelName:            modelCfg.ModelName,
		FallbackModelRef:     fallbackModelRef,
		FallbackModelCfgPath: fallbackModelCfgPath,
		FallbackModelErr:     fallbackModelErr,
		FallbackModelName:    fallbackModelCfg.ModelName,
		BlockSize:            envInt("INPUT_BLOCK_SIZE", DefaultBlockingBlockSize, 1),
		PrevOverlap:          prevOverlap,
		NextOverlap:          nextOverlap,
		RemoveTOC:            removeTOC,
		ArtifactDir:          strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:       strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
	}
}

func (p *ProvisionsProcessor) Name() string { return "extract_provisions" }

func (p *ProvisionsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26050521) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26050522) load provisions prompt %q: %w", p.PromptRef, p.PromptErr)
	}
	if p.InputStore == nil {
		return errors.New("(MID_26050523) input store is nil")
	}
	if p.Store == nil {
		return errors.New("(MID_26050524) provisions store is nil")
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("(MID_26050525) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("provisions extraction skipped: model config error",
			"record_id", evt.RecordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		p.persistProvisionsStatus(ctx, rec, start, p.ModelErr)
		return nil
	}

	inputFilename := filepath.Base(strings.TrimSpace(rec.ResultFilename))
	if inputFilename == "" {
		inputFilename = fmt.Sprintf("record_%d", evt.RecordID)
	}

	if evt.Force {
		_, _ = p.Store.DeleteProvisionsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.ProvisionsExist(ctx, evt.RecordID, "")
		if err != nil {
			p.persistProvisionsStatus(ctx, rec, start, err)
			p.Logger.Error("provision exist", "error", err,
				"record_id", evt.RecordID)
			return nil
		}
		if exists {
			p.Logger.Info("provisions extraction skipped", "record_id", evt.RecordID, "reason", "provisions already exist and force=false")
			p.persistProvisionsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	blocks, err := p.resolveBlocks(ctx, evt, rec)
	if err != nil {
		p.persistProvisionsStatus(ctx, rec, start, err)
		p.Logger.Error("resolveBlocks error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	if len(blocks) == 0 {
		err := fmt.Errorf("(MID_26050526) no blocks found for record_id=%d", evt.RecordID)
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}

	p.Logger.Info("Before calling LLM",
		"num_blocks", len(blocks),
		"record_id", evt.RecordID,
		"filename", inputFilename)

	result, err := p.extractProvisionsFromBlocksWithLLM(ctx, blocks, evt.RecordID)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistProvisions(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}
	outputRows := p.buildProvisionOutputRows(result.Provisions, start, len(blocks), time.Since(start).Milliseconds(), result.ModelName)

	inserted, err := p.Store.SaveProvisions(ctx, SaveProvisionsRequest{
		InputRecordID: evt.RecordID,
		InputFilename: inputFilename,
		ExtractID:     result.ExtractID,
		Language:      result.Language,
		Provisions:    outputRows,
	})
	if err != nil {
		p.persistProvisionsStatus(ctx, rec, start, err)
		p.Logger.Error("save provision error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	if err := p.indexProvisionsInTree(evt.RecordID, outputRows); err != nil {
		p.Logger.Error("index provision tree error", "error", err, "record_id", evt.RecordID)
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}
	if err := p.writeProvisionsArtifact(evt.RecordID, rec, outputRows); err != nil {
		p.Logger.Error("write provisions artifact error", "error", err, "record_id", evt.RecordID)
		p.persistProvisionsStatus(ctx, rec, start, err)
		return nil
	}
	if reindexErr := ReindexProvisionSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex provision search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}
	p.Logger.Info("provisions extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"provisions_count", len(outputRows),
		"blocks", len(blocks),
	)
	p.persistProvisionsStatus(ctx, rec, start, nil)
	p.logProvisionsSummary(ctx, start, p.Now(), result, inserted, len(blocks), evt.RecordID)
	return nil
}

func (p *ProvisionsProcessor) logLLMCall(
	ctx context.Context,
	callID, activity string,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
	recordID int64,
	blockIdx int,
	totalBlocks int,
	provisionsSoFar int,
	numInBlock int,
) {
	var artifactStr *string
	if payload != nil {
		if bs, err := json.Marshal(payload); err == nil {
			s := string(bs)
			artifactStr = &s
		}
	}
	var errStr *string
	if callErr != nil {
		s := callErr.Error()
		errStr = &s
	}
	percent := 0
	if totalBlocks > 0 {
		percent = (blockIdx + 1) * 100 / totalBlocks
	}
	progress := fmt.Sprintf("%d%% (%d/%d)", percent, blockIdx+1, totalBlocks)
	extraInfo := map[string]any{
		"block":             blockIdx + 1,
		"total_blocks":      totalBlocks,
		"num_provisions":    numInBlock,
		"provisions_so_far": provisionsSoFar,
		"percent":           fmt.Sprintf("%d%%", percent),
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)
	callReason := "extract compliance provisions"
	rec := DocProcLogRecord{
		CallReason:    callReason,
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		LLMCallID:     &callID,
		ActivityName:  &activity,
		ArtifactJSON:  artifactStr,
		Errors:        errStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogExtractProvisions(ctx, rec, "MID-26052811"); err != nil {
		p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
	}
}

func (p *ProvisionsProcessor) logProvisionsSummary(
	ctx context.Context,
	start, end time.Time,
	result provisionExtractionResult,
	inserted int64,
	numBlocks int,
	recordID int64,
) {
	extraInfo := map[string]any{
		"total_provisions": inserted,
		"llm_call_count":   result.LLMCallCount,
		"fallback_count":   result.FallbackCount,
		"num_blocks":       numBlocks,
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)

	seen := map[string]struct{}{}
	modelNames := make([]string, 0, 2)
	for _, n := range []string{p.ModelName, p.FallbackModelName} {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		modelNames = append(modelNames, n)
	}

	progress := "100%"
	activityName := "extract_provisions"
	rec := DocProcLogRecord{
		CallReason:    "extract compliance provisions",
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    p.PromptRef,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogSummary(ctx, rec, "MID-26052811"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func (p *ProvisionsProcessor) resolveBlocks(ctx context.Context, evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Block, error) {
	if buf := BlockBufferFromContext(ctx); buf != nil {
		return buf.Blocks, nil
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("(MID_26050527) resolve input file for record_id=%d: %w", evt.RecordID, err)
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("(MID_26050528) input file not exist: %s", inputPath)
		}
		return nil, fmt.Errorf("(MID_26050529) read input file: %w", err)
	}
	buf, err := buildBlocks(body, p.blockSize(), p.PrevOverlap, p.NextOverlap, p.RemoveTOC)
	if err != nil {
		return nil, fmt.Errorf("(MID_26050530) build blocks: %w", err)
	}
	return buf.Blocks, nil
}

func (p *ProvisionsProcessor) blockSize() int {
	if p.BlockSize >= 1 {
		return p.BlockSize
	}
	return DefaultBlockingBlockSize
}

func (p *ProvisionsProcessor) extractProvisionsFromBlocksWithLLM(
	ctx context.Context,
	blocks []Block,
	record_id int64) (provisionExtractionResult, error) {
	extractID := p.Now().Format("20060102-150405")
	language := ""
	provisions := make([]map[string]any, 0, len(blocks))
	usedModelName := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount int

	for idx, block := range blocks {
		if isCtxStopped(ctx) {
			return provisionExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount}, ErrPipelineStopped
		}
		callStart := p.Now()
		p.Logger.Info("extract provisions start",
			"record_id", record_id,
			"idx", idx,
			"total", len(blocks),
			"model name", p.ModelName,
			"prompt name", p.PromptRef,
		)
		callID := fmt.Sprintf("%s_p1_b%d", eventIDFromContext(ctx), idx)
		payload, modelName, err := p.extractProvisionPayloadWithFallback(ctx, block)
		llmCallCount++
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != "" {
			fallbackCount++
		}
		numInBlock := 0
		if err == nil && payload != nil {
			if raw, ok := payload["provisions"].([]any); ok {
				numInBlock = len(raw)
			}
		}
		p.logLLMCall(ctx, callID, "extract_provisions",
			[]string{strings.TrimSpace(modelName)}, p.PromptRef,
			payload, err, callStart, p.Now(),
			record_id, idx, len(blocks), len(provisions), numInBlock)
		if err != nil {
			p.Logger.Error("failed extracting", "error", err)
			return provisionExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount},
				fmt.Errorf("(MID_26050531) extract provisions via llm: %w", err)
		}
		usedModelName = strings.TrimSpace(modelName)
		if language == "" {
			language = strings.TrimSpace(asString(payload["language"]))
		}
		raw := payload["provisions"].([]any)
		provisions = append(provisions, normalizeProvisionList(raw, blockLineToPage(block), blockLineText(block))...)
		p.Logger.Info("extract provisions end  ",
			"record_id", record_id,
			"# provisions", len(provisions),
			"ms_used", time.Since(callStart).Milliseconds())
	}
	if language == "" {
		language = "unknown"
	}
	return provisionExtractionResult{
		ExtractID:     extractID,
		Language:      language,
		Provisions:    provisions,
		ModelName:     firstNonEmptyTrimmed(usedModelName, p.ModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
	}, nil
}

func (p *ProvisionsProcessor) extractProvisionPayloadWithFallback(ctx context.Context, block Block) (map[string]any, string, error) {
	payload, err := p.extractProvisionPayload(ctx, block, p.ModelName)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}

	var payloadVal any = "nil"
	if payload != nil {
		payloadVal = payload
	}

	p.Logger.Warn("primary LLM failed extracting provisions",
		"error", err,
		"model_name", p.ModelName,
		"fallback_model", p.FallbackModelName,
		"payload", payloadVal)

	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, strings.TrimSpace(p.ModelName),
			fmt.Errorf("(MID_26050820) extract provisions failed and fallback model not available, err:%w, model_name:%s", err, p.ModelName)
	}

	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26050544) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	if isEmptyProvisionExtractionError(err) {
		p.Logger.Warn("primary provisions extraction returned empty JSON; retrying fallback model",
			"primary_model", p.ModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.PromptRef,
		)
	} else {
		p.Logger.Warn("primary provisions extraction failed; retrying fallback model",
			"primary_model", p.ModelName,
			"fallback_model", fallbackModelName,
			"error", err,
			"prompt_name", p.PromptRef,
		)
	}

	payload, fallbackErr := p.extractProvisionPayload(ctx, block, fallbackModelName)
	if fallbackErr != nil {
		if isEmptyProvisionExtractionError(fallbackErr) {
			p.Logger.Warn("fallback provisions extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", p.PromptRef,
			)
			return map[string]any{
				"language":   "unknown",
				"provisions": []any{},
			}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26050545) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func isEmptyProvisionExtractionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "unexpected end of JSON input") &&
		strings.Contains(msg, "json:{[]}")
}

func (p *ProvisionsProcessor) extractProvisionPayload(ctx context.Context, block Block, modelName string) (map[string]any, error) {
	p.Logger.Info("To extract provisions",
		"model", modelName,
		"prompt_name", p.PromptRef)

	in := llmclients.JSONExtractionInput{
		PromptText: p.PromptText,
		ModelName:  modelName,
		InputText:  buildProvisionUserPrompt(block),
	}
	var (
		payload map[string]any
		err     error
	)
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, provisionExtractionContract())
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, err = p.Extractor.ExtractJSON(ctx, in)
	}
	if err != nil {
		if payload == nil {
			return nil, fmt.Errorf("(MID_26050841) failed extracting provisions, error:%w", err)
		}
		return nil, fmt.Errorf("(MID_26050842) failed extracting provisions, error:%w, payload:%v", err, payload)
	}

	payload = normalizeProvisionPayload(payload)
	if len(payload) == 0 {
		return nil, errors.New("(MID_26050546) empty llm json object")
	}
	raw, ok := payload["provisions"]
	if !ok {
		return nil, fmt.Errorf("(MID_26050532) llm output field 'provisions' must be an array, JSON:%v", payload)
	}
	items, ok := normalizeProvisionItems(raw)
	if !ok {
		return nil, fmt.Errorf("(MID_26050547) llm output field 'provisions' must be an array, JSON:%v", payload)
	}
	payload["provisions"] = items
	return payload, nil
}

func normalizeProvisionPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	if _, ok := payload["provisions"]; ok {
		return payload
	}
	if looksLikeProvisionRecord(payload) {
		return map[string]any{
			"language":   firstNonEmptyTrimmed(asString(payload["language"]), "unknown"),
			"provisions": []any{payload},
		}
	}
	return payload
}

func normalizeProvisionItems(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case map[string]any:
		return []any{v}, true
	default:
		return nil, false
	}
}

func looksLikeProvisionRecord(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	for _, key := range []string{"name", "provision", "provision_original", "provision_en", "type", "subject", "source_line_spans"} {
		if strings.TrimSpace(asString(payload[key])) != "" {
			return true
		}
		if key == "source_line_spans" {
			if _, ok := payload[key].([]any); ok {
				return true
			}
		}
	}
	return false
}

func buildProvisionUserPrompt(block Block) string {
	schema := map[string]any{
		"language": "string",
		"provisions": []map[string]any{{
			"name":              "string",
			"name_en":           "string",
			"type":              "mandatory|recommended|optional",
			"provision":         "string",
			"provision_en":      "string",
			"provision_desc":    "string",
			"provision_desc_en": "string",
			"source_line_spans": []string{"2:10"},
			"context":           "string",
			"context_en":        "string",
			"subject":           "string",
			"subject_en":        "string",
			"location_type":     "string",
			"keywords":          []string{"string"},
			"keywords_en":       []string{"string"},
			"confidence":        0.0,
			"is_explicit":       true,
			"need_verify":       false,
			"category_paths":    []any{},
			"category_path_en":  []any{},
		}},
	}
	lines := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		lines = append(lines, line.String())
	}
	linesJSON, _ := json.Marshal(lines)
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nBlock index: " + strconv.Itoa(block.Index) +
		"\n\nInput block lines (plain):\n" + strings.Join(lines, "\n") +
		"\n\nInput block lines (raw, JSON array):\n" + string(linesJSON)
}

func blockLineToPage(block Block) map[int]int {
	out := make(map[int]int, len(block.Lines))
	for _, line := range block.Lines {
		if line.LineNumber > 0 && line.PageNumber > 0 {
			out[line.LineNumber] = line.PageNumber
		}
	}
	return out
}

func blockLineText(block Block) map[string]string {
	out := make(map[string]string, len(block.Lines))
	for _, line := range block.Lines {
		if line.LineNumber <= 0 || line.PageNumber <= 0 {
			continue
		}
		out[fmt.Sprintf("%d:%d", line.PageNumber, line.LineNumber)] = strings.TrimSpace(line.Content)
	}
	return out
}

func normalizeProvisionList(items []any, lineToPage map[int]int, lineText map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		provisionName := strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["provision_name"]), asString(raw["name"])))
		provisionText := strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["provision"]), asString(raw["provision_original"])))
		categoryPaths := raw["category_paths"]
		if categoryPaths == nil {
			categoryPaths = raw["categories"]
		}
		normalized := map[string]any{
			"provision_name":    provisionName,
			"provision_name_en": strings.TrimSpace(asString(raw["name_en"])),
			"provision_type":    strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["provision_type"]), asString(raw["type"]))),
			"source_text":       strings.TrimSpace(firstNonEmptyTrimmed(asString(raw["source_text"]), provisionText)),
			"provision":         provisionText,
			"provision_en":      strings.TrimSpace(asString(raw["provision_en"])),
			"provision_desc":    strings.TrimSpace(asString(raw["provision_desc"])),
			"provision_desc_en": strings.TrimSpace(asString(raw["provision_desc_en"])),
			"context":           strings.TrimSpace(asString(raw["context"])),
			"context_en":        strings.TrimSpace(asString(raw["context_en"])),
			"subject":           strings.TrimSpace(asString(raw["subject"])),
			"subject_en":        strings.TrimSpace(asString(raw["subject_en"])),
			"location_type":     strings.TrimSpace(asString(raw["location_type"])),
			"keywords":          toStringSlice(raw["keywords"]),
			"keywords_en":       toStringSlice(raw["keywords_en"]),
			"confidence":        toFloat(raw["confidence"]),
			"is_explicit":       toBool(raw["is_explicit"]),
			"need_verify":       toBool(raw["need_verify"]),
			"category_paths":    categoryPaths,
			"category_paths_en": raw["category_path_en"],
		}
		sourceSpans := normalizeProvisionSourceLineSpans(raw["source_line_spans"], lineToPage)
		normalized["source_line_spans"] = sourceSpans
		if strings.TrimSpace(asString(normalized["source_text"])) == "" {
			normalized["source_text"] = sourceTextFromSpans(sourceSpans, lineText)
		}
		out = append(out, normalized)
	}
	return out
}

func sourceTextFromSpans(spans []string, lineText map[string]string) string {
	if len(spans) == 0 || len(lineText) == 0 {
		return ""
	}
	parts := make([]string, 0, len(spans))
	for _, span := range spans {
		text := strings.TrimSpace(lineText[strings.TrimSpace(span)])
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeProvisionSourceLineSpans(value any, lineToPage map[int]int) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}

	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	appendSpan := func(span string) {
		span = strings.TrimSpace(span)
		if span == "" {
			return
		}
		if _, ok := seen[span]; ok {
			return
		}
		seen[span] = struct{}{}
		out = append(out, span)
	}

	formatLineSpan := func(pageNo int, lineNo int) string {
		if lineNo <= 0 {
			return ""
		}
		if pageNo > 0 {
			return fmt.Sprintf("%d:%d", pageNo, lineNo)
		}
		return strconv.Itoa(lineNo)
	}

	for _, item := range items {
		switch v := item.(type) {
		case map[string]any:
			lineNo := int(toFloat(v["line_number"]))
			pageNo := int(toFloat(v["page_number"]))
			if pageNo <= 0 && lineNo > 0 {
				pageNo = lineToPage[lineNo]
			}
			appendSpan(formatLineSpan(pageNo, lineNo))
		case float64:
			lineNo := int(v)
			appendSpan(formatLineSpan(lineToPage[lineNo], lineNo))
		case string:
			appendSpan(v)
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *ProvisionsProcessor) buildProvisionOutputRows(provisions []map[string]any, now time.Time, numBlocks int, durationMs int64, modelName string) []map[string]any {
	out := make([]map[string]any, 0, len(provisions))
	timeText := now.Format(defaultDocMetaStatusTime)
	timePerProvision := float64(0)
	if len(provisions) > 0 {
		timePerProvision = float64(durationMs) / float64(len(provisions))
	}
	for i, provision := range provisions {
		publicInfo := map[string]any{
			"provision_type":    strings.TrimSpace(asString(provision["provision_type"])),
			"source_line_spans": provision["source_line_spans"],
		}
		out = append(out, map[string]any{
			"prov_id":               i + 1,
			"prov_name":             strings.TrimSpace(asString(provision["provision_name"])),
			"prov_name_en":          strings.TrimSpace(asString(provision["provision_name_en"])),
			"provision":             strings.TrimSpace(asString(provision["provision"])),
			"provision_en":          strings.TrimSpace(asString(provision["provision_en"])),
			"provision_subject":     strings.TrimSpace(asString(provision["subject"])),
			"provision_subject_en":  strings.TrimSpace(asString(provision["subject_en"])),
			"prov_desc":             strings.TrimSpace(firstNonEmptyTrimmed(asString(provision["provision_desc"]), asString(provision["provision_en"]), asString(provision["provision"]), asString(provision["source_text"]))),
			"prov_desc_en":          strings.TrimSpace(asString(provision["provision_desc_en"])),
			"prov_context":          strings.TrimSpace(asString(provision["context"])),
			"prov_context_en":       strings.TrimSpace(asString(provision["context_en"])),
			"provision_keywords":    provision["keywords"],
			"provision_keywords_en": provision["keywords_en"],
			"category_paths":        provision["category_paths"],
			"category_paths_en":     provision["category_paths_en"],
			"location_type":         strings.TrimSpace(asString(provision["location_type"])),
			"confidence":            toFloat(provision["confidence"]),
			"source_text":           strings.TrimSpace(asString(provision["source_text"])),
			"num_blocks":            numBlocks,
			"num_provisions":        len(provisions),
			"time_per_provision":    timePerProvision,
			"model_name":            strings.TrimSpace(firstNonEmptyTrimmed(modelName, p.ModelName)),
			"prompt_name":           strings.TrimSpace(p.PromptRef),
			"is_explicit":           toBool(provision["is_explicit"]),
			"need_verify":           toBool(provision["need_verify"]),
			"status":                "active",
			"create_time":           timeText,
			"modify_time":           timeText,
			"public_info":           publicInfo,
			"private_info":          map[string]any{},
			"notes":                 "",
			"error_msg":             "",
			"source_line_spans":     provision["source_line_spans"],
		})
	}
	return out
}

func loadOptionalModelConfigFromEnv(modelRefEnv string, modelsFileEnv string) (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(os.Getenv(modelRefEnv))
	if modelRef == "" {
		return "", "", structureModelConfig{}, nil
	}
	return loadModelConfigFromEnv(modelRefEnv, modelsFileEnv)
}

func buildProvisionDBRecord(provision map[string]any, language string) map[string]any {
	publicInfo, _ := provision["public_info"].(map[string]any)
	provisionText := strings.TrimSpace(asString(provision["provision"]))
	provisionEn := strings.TrimSpace(asString(provision["provision_en"]))
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "en" || lang == "eng" || lang == "english" || strings.EqualFold(provisionText, provisionEn) {
		provisionText = ""
	}

	var provisionValue any
	if provisionText != "" {
		provisionValue = provisionText
	}

	return map[string]any{
		"prov_id":               int(toFloat(provision["prov_id"])),
		"prov_name":             strings.TrimSpace(asString(provision["prov_name"])),
		"prov_name_en":          strings.TrimSpace(asString(provision["prov_name_en"])),
		"provision_type":        strings.TrimSpace(asString(publicInfo["provision_type"])),
		"source_text":           strings.TrimSpace(asString(provision["source_text"])),
		"source_line_spans":     provision["source_line_spans"],
		"provision":             provisionValue,
		"provision_en":          provisionEn,
		"prov_context":          strings.TrimSpace(asString(provision["prov_context"])),
		"prov_context_en":       strings.TrimSpace(asString(provision["prov_context_en"])),
		"provision_subject":     strings.TrimSpace(asString(provision["provision_subject"])),
		"provision_subject_en":  strings.TrimSpace(asString(provision["provision_subject_en"])),
		"prov_desc":             strings.TrimSpace(asString(provision["prov_desc"])),
		"prov_desc_en":          strings.TrimSpace(asString(provision["prov_desc_en"])),
		"provision_keywords":    provision["provision_keywords"],
		"provision_keywords_en": provision["provision_keywords_en"],
		"category_paths":        provision["category_paths"],
		"category_paths_en":     provision["category_paths_en"],
		"location_type":         strings.TrimSpace(asString(provision["location_type"])),
		"confidence":            toFloat(provision["confidence"]),
		"is_explicit":           toBool(provision["is_explicit"]),
		"need_verify":           toBool(provision["need_verify"]),
		"num_blocks":            int(toFloat(provision["num_blocks"])),
		"num_provisions":        int(toFloat(provision["num_provisions"])),
		"time_per_provision":    toFloat(provision["time_per_provision"]),
		"model_name":            strings.TrimSpace(asString(provision["model_name"])),
		"prompt_name":           strings.TrimSpace(asString(provision["prompt_name"])),
		"status":                strings.TrimSpace(firstNonEmptyTrimmed(asString(provision["status"]), "active")),
		"public_info":           publicInfo,
		"private_info":          provision["private_info"],
		"notes":                 strings.TrimSpace(asString(provision["notes"])),
		"error_msg":             strings.TrimSpace(asString(provision["error_msg"])),
	}
}

func (p *ProvisionsProcessor) indexProvisionsInTree(recordID int64, provisions []map[string]any) error {
	dir := strings.TrimSpace(p.ArtifactWebDir)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	}
	if dir == "" {
		return errors.New("(MID_26050560) missing ARTIFACT_WEB_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26050561) invalid record_id: %d", recordID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("(MID_26050562) create artifact web dir: %w", err)
	}
	if err := removeProvisionTreeRecord(dir, recordID); err != nil {
		return fmt.Errorf("(MID_26050563) remove old provision tree entries for record %d: %w", recordID, err)
	}
	now := p.Now()
	for _, prov := range provisions {
		provID := int(toFloat(prov["prov_id"]))
		if provID <= 0 {
			continue
		}
		entry := fmt.Sprintf("%d_%d", recordID, provID)
		for _, pair := range pairCategoryPathEntries(prov["category_paths"], prov["category_paths_en"]) {
			if err := upsertProvisionIDToLeafDir(p.Logger, dir, pair.Index, pair.Original, entry, now); err != nil {
				return fmt.Errorf("(MID_26050564) index provision %s: %w", entry, err)
			}
		}
	}
	return nil
}

func removeProvisionTreeRecord(baseDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(baseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "provisions.txt" {
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

func upsertProvisionIDToLeafDir(_ ApiTypes.JimoLogger, baseDir string, indexEntry CategoryPathEntry, originalEntry CategoryPathEntry, provisionID string, now time.Time) error {
	if len(indexEntry.Nodes) == 0 {
		return nil
	}
	currentDir := baseDir
	for i := range indexEntry.Nodes {
		indexNode, originalNode := categoryNodePair(indexEntry, originalEntry, i)
		normalizedSeg := normalizeCategorySegment(indexNode.Name)
		if normalizedSeg == "" {
			return fmt.Errorf("(MID_26050566) empty category segment for provision %s", provisionID)
		}
		subdir := filepath.Join(currentDir, normalizedSeg)
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			return err
		}
		if err := upsertCategoryDirMetadataLocalized(
			subdir,
			firstNonEmptyTrimmed(originalNode.Name, indexNode.Name),
			indexNode.Name,
			"provision",
			maxFloat(indexNode.Confidence, originalNode.Confidence),
			trimStringSlice(originalNode.Keywords),
			localizedEnglishKeywords(originalNode.Name, indexNode.Name, indexNode.Keywords),
			now,
		); err != nil {
			return err
		}
		currentDir = subdir
	}
	leaf := filepath.Join(currentDir, "provisions.txt")
	existing := make([]string, 0)
	if bs, err := os.ReadFile(leaf); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	for _, e := range existing {
		if e == provisionID {
			return nil
		}
	}
	existing = append(existing, provisionID)
	sort.Strings(existing)
	return os.WriteFile(leaf, []byte(strings.Join(existing, "\n")), 0o644)
}

func (p *ProvisionsProcessor) writeProvisionsArtifact(recordID int64, rec DocMetadataInputRecord, provisions []map[string]any) error {
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	}
	if artifactDir == "" {
		return errors.New("(MID_26050570) missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26050571) invalid record_id: %d", recordID)
	}

	groupID := recordID / 1000
	stagingBase := filepath.Base(strings.TrimSpace(rec.StagingFilename))
	filenameRoot := strings.TrimSuffix(stagingBase, filepath.Ext(stagingBase))
	if filenameRoot == "" {
		filenameRoot = fmt.Sprintf("record_%d", recordID)
	}
	parserName := strings.TrimSpace(rec.ParserName)
	if parserName == "" {
		parserName = "default"
	}

	outDir := filepath.Join(artifactDir, strconv.FormatInt(groupID, 10), strconv.FormatInt(recordID, 10))
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("(MID_26050572) create artifact dir: %w", err)
	}

	outPath := filepath.Join(outDir, filenameRoot+"_"+parserName+".provisions")
	fileRecords := make([]map[string]any, 0, len(provisions))
	for _, row := range provisions {
		fileRecords = append(fileRecords, buildProvisionFileRecord(row))
	}
	bs, err := json.MarshalIndent(fileRecords, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26050573) marshal provisions: %w", err)
	}
	if err := os.WriteFile(outPath, bs, 0o644); err != nil {
		return fmt.Errorf("(MID_26050574) write provisions artifact: %w", err)
	}
	return nil
}

func buildProvisionFileRecord(row map[string]any) map[string]any {
	publicInfo, _ := row["public_info"].(map[string]any)
	return map[string]any{
		"prov_id":           int(toFloat(row["prov_id"])),
		"prov_name":         strings.TrimSpace(asString(row["prov_name"])),
		"prov_name_en":      strings.TrimSpace(asString(row["prov_name_en"])),
		"prov_type":         strings.TrimSpace(asString(publicInfo["provision_type"])),
		"provision":         strings.TrimSpace(asString(row["provision"])),
		"provision_en":      strings.TrimSpace(asString(row["provision_en"])),
		"provision_desc":    strings.TrimSpace(asString(row["prov_desc"])),
		"provision_desc_en": strings.TrimSpace(asString(row["prov_desc_en"])),
		"source_line_spans": row["source_line_spans"],
		"context":           strings.TrimSpace(asString(row["prov_context"])),
		"context_en":        strings.TrimSpace(asString(row["prov_context_en"])),
		"subject":           strings.TrimSpace(asString(row["provision_subject"])),
		"subject_en":        strings.TrimSpace(asString(row["provision_subject_en"])),
		"location_type":     strings.TrimSpace(asString(row["location_type"])),
		"keywords":          row["provision_keywords"],
		"keywords_en":       row["provision_keywords_en"],
		"confidence":        toFloat(row["confidence"]),
		"is_explicit":       toBool(row["is_explicit"]),
		"need_verify":       toBool(row["need_verify"]),
		"category_paths":    row["category_paths"],
		"category_paths_en": row["category_paths_en"],
	}
}

type provisionsStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	ProcErr       error
}

func (p *ProvisionsProcessor) persistProvisionsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	statusRaw, err := appendProvisionsStatus(rec.StatusRaw, provisionsStatusParams{
		RecordID:      rec.ID,
		FileType:      detectProvisionsFileType(rec),
		InputFilename: strings.TrimSpace(rec.ResultFilename),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
	if err != nil {
		p.Logger.Error("failed building provisions status", "record_id", rec.ID, "error", err)
		return
	}
	if err := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); err != nil {
		p.Logger.Error("failed persisting provisions status", "record_id", rec.ID, "error", err)
	}
}

func (p *ProvisionsProcessor) stopAndPersistProvisions(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	statusRaw, err := appendProvisionsStatus(rec.StatusRaw, provisionsStatusParams{
		RecordID:      rec.ID,
		FileType:      detectProvisionsFileType(rec),
		InputFilename: strings.TrimSpace(rec.ResultFilename),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcStatus:    "stopped",
	})
	if err != nil {
		p.Logger.Error("(MID_26052841) failed building provisions stopped status", "record_id", rec.ID, "error", err)
		return
	}
	if updateErr := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
	}); updateErr != nil {
		p.Logger.Error("(MID_26052842) failed persisting provisions stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) extract_provisions stopped by user request", "record_id", rec.ID)
}

func detectProvisionsFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func appendProvisionsStatus(raw string, p provisionsStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_provisions",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
	}
	if override := strings.TrimSpace(p.ProcStatus); override != "" {
		entry["proc_status"] = override
		if p.ProcErr != nil {
			entry["error"] = strings.TrimSpace(p.ProcErr.Error())
		}
	} else if p.ProcErr == nil {
		entry["proc_status"] = "success"
	} else {
		entry["proc_status"] = "failed"
		entry["error"] = strings.TrimSpace(p.ProcErr.Error())
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "extract_provisions" && op != "extract-provisions" {
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

func loadProvisionsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_PROVISIONS_PROMPT", "PROMPT_FILE_NAME"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = "prompt-extract-provisions.md"
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
		addCandidate(filepath.Join("prompts", promptRef))
	}

	var lastErr error
	for _, candidate := range paths {
		bs, err := os.ReadFile(candidate)
		if err != nil {
			lastErr = fmt.Errorf("(MID_26050533) failed reading file. Path:%s, error:%w", candidate, err)
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("(MID_26050534) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("(MID_26050535) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26050536) prompt file not found: %w", lastErr)
}

func (s ProvisionsSQLStore) ensureReady() error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26050537) db is nil")
	}
	return nil
}

func (s ProvisionsSQLStore) ProvisionsExist(ctx context.Context, inputRecordID int64, inputFilename string) (bool, error) {
	if err := s.ensureReady(); err != nil {
		return false, err
	}
	const q = `
SELECT 1
FROM kb.provisions
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

func (s ProvisionsSQLStore) DeleteProvisionsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.provisions WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s ProvisionsSQLStore) SaveProvisions(ctx context.Context, req SaveProvisionsRequest) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	if len(req.Provisions) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.provisions (
	input_record_id,
	extract_id,
	input_filename,
	prov_id,
	prov_name,
	prov_name_en,
	provision_type,
	source_text,
	source_line_spans,
	provision,
	provision_en,
	provision_subject,
	provision_subject_en,
	prov_desc,
	prov_desc_en,
	prov_context,
	prov_context_en,
	provision_keywords,
	provision_keywords_en,
	category_paths,
	category_paths_en,
	location_type,
	confidence,
	is_explicit,
	need_verify,
	num_blocks,
	num_provisions,
	time_per_provision,
	model_name,
	prompt_name,
	status,
	public_info,
	private_info,
	notes,
	error_msg
)
VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb,$19::jsonb,$20::jsonb,$21::jsonb,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32::jsonb,$33::jsonb,$34,$35
)
ON CONFLICT (input_record_id, prov_id) DO UPDATE SET
	extract_id = EXCLUDED.extract_id,
	input_filename = EXCLUDED.input_filename,
	prov_name = EXCLUDED.prov_name,
	prov_name_en = EXCLUDED.prov_name_en,
	provision_type = EXCLUDED.provision_type,
	source_text = EXCLUDED.source_text,
	source_line_spans = EXCLUDED.source_line_spans,
	provision = EXCLUDED.provision,
	provision_en = EXCLUDED.provision_en,
	provision_subject = EXCLUDED.provision_subject,
	provision_subject_en = EXCLUDED.provision_subject_en,
	prov_desc = EXCLUDED.prov_desc,
	prov_desc_en = EXCLUDED.prov_desc_en,
	prov_context = EXCLUDED.prov_context,
	prov_context_en = EXCLUDED.prov_context_en,
	provision_keywords = EXCLUDED.provision_keywords,
	provision_keywords_en = EXCLUDED.provision_keywords_en,
	category_paths = EXCLUDED.category_paths,
	category_paths_en = EXCLUDED.category_paths_en,
	location_type = EXCLUDED.location_type,
	confidence = EXCLUDED.confidence,
	is_explicit = EXCLUDED.is_explicit,
	need_verify = EXCLUDED.need_verify,
	num_blocks = EXCLUDED.num_blocks,
	num_provisions = EXCLUDED.num_provisions,
	time_per_provision = EXCLUDED.time_per_provision,
	model_name = EXCLUDED.model_name,
	prompt_name = EXCLUDED.prompt_name,
	status = EXCLUDED.status,
	public_info = EXCLUDED.public_info,
	private_info = EXCLUDED.private_info,
	notes = EXCLUDED.notes,
	error_msg = EXCLUDED.error_msg,
	modify_time = NOW()`

	var inserted int64
	for _, provision := range req.Provisions {
		dbRecord := buildProvisionDBRecord(provision, req.Language)
		sourceSpansJSON, _ := json.Marshal(dbRecord["source_line_spans"])
		keywordsJSON, _ := json.Marshal(dbRecord["provision_keywords"])
		keywordsEnJSON, _ := json.Marshal(dbRecord["provision_keywords_en"])
		categoryPathsJSON, _ := json.Marshal(dbRecord["category_paths"])
		categoryPathsEnJSON, _ := json.Marshal(dbRecord["category_paths_en"])
		publicInfo := map[string]any{
			"language":       req.Language,
			"schema_version": "1",
		}
		if existing, ok := dbRecord["public_info"].(map[string]any); ok {
			for k, v := range existing {
				publicInfo[k] = v
			}
		}
		publicInfoJSON, _ := json.Marshal(publicInfo)
		privateInfoJSON, _ := json.Marshal(dbRecord["private_info"])

		_, err := s.DB.ExecContext(ctx, stmt,
			req.InputRecordID,                 // $1
			req.ExtractID,                     // $2
			req.InputFilename,                 // $3
			int(toFloat(dbRecord["prov_id"])), // $4
			strings.TrimSpace(asString(dbRecord["prov_name"])),                              // $5
			strings.TrimSpace(asString(dbRecord["prov_name_en"])),                           // $6
			strings.TrimSpace(asString(dbRecord["provision_type"])),                         // $7
			strings.TrimSpace(asString(dbRecord["source_text"])),                            // $8
			string(sourceSpansJSON),                                                         // $9
			dbRecord["provision"],                                                           // $10
			strings.TrimSpace(asString(dbRecord["provision_en"])),                           // $11
			strings.TrimSpace(asString(dbRecord["provision_subject"])),                      // $12
			strings.TrimSpace(asString(dbRecord["provision_subject_en"])),                   // $13
			strings.TrimSpace(asString(dbRecord["prov_desc"])),                              // $14
			strings.TrimSpace(asString(dbRecord["prov_desc_en"])),                           // $15
			strings.TrimSpace(asString(dbRecord["prov_context"])),                           // $16
			strings.TrimSpace(asString(dbRecord["prov_context_en"])),                        // $17
			string(keywordsJSON),                                                            // $18
			string(keywordsEnJSON),                                                          // $19
			string(categoryPathsJSON),                                                       // $20
			string(categoryPathsEnJSON),                                                     // $21
			strings.TrimSpace(asString(dbRecord["location_type"])),                          // $22
			toFloat(dbRecord["confidence"]),                                                 // $23
			toBool(dbRecord["is_explicit"]),                                                 // $24
			toBool(dbRecord["need_verify"]),                                                 // $25
			int(toFloat(dbRecord["num_blocks"])),                                            // $26
			int(toFloat(dbRecord["num_provisions"])),                                        // $27
			toFloat(dbRecord["time_per_provision"]),                                         // $28
			strings.TrimSpace(asString(dbRecord["model_name"])),                             // $29
			strings.TrimSpace(asString(dbRecord["prompt_name"])),                            // $30
			strings.TrimSpace(firstNonEmptyTrimmed(asString(dbRecord["status"]), "active")), // $31
			string(publicInfoJSON),                                                          // $32
			string(privateInfoJSON),                                                         // $33
			strings.TrimSpace(asString(dbRecord["notes"])),                                  // $34
			strings.TrimSpace(asString(dbRecord["error_msg"])),                              // $35
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
