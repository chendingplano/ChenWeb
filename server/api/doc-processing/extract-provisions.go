package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type ProvisionsProcessor struct {
	InputStore                    DocMetadataStore
	Store                         ProvisionsStore
	Extractor                     LLMJSONExtractor
	Logger                        ApiTypes.JimoLogger
	ProcLogger                    DocProcLogger
	Now                           func() time.Time
	PromptText                    string
	PromptRef                     string
	PromptPath                    string
	PromptErr                     error
	ModelRef                      string
	ModelCfgPath                  string
	ModelErr                      error
	ModelName                     string
	FallbackModelRef              string
	FallbackModelCfgPath          string
	FallbackModelErr              error
	FallbackModelName             string
	FallbackExtractor             LLMJSONExtractor
	BlockSize                     int
	PrevOverlap                   int
	NextOverlap                   int
	RemoveTOC                     bool
	ArtifactDir                   string
	ArtifactWebDir                string
	ExtractProvisionsMaxTasks     int
	ExtractProvisionsInput        string // "chunks" (default) or "blocks"
	ObjectStore                   ArtifactObjectsStore
	ObjectReconciler              ObjectReconciler
	AmbiguousObjectLLMResolver    AmbiguousObjectLLMResolver
	ResolveAmbiguousMinConfidence float64

	// batch state (set by ChunkBatchProcessor.InitChunkBatch)
	batchChunks   []Chunk
	batchDocCtx   string
	batchRecordID int64
	batchRec      DocMetadataInputRecord
	batchStart    time.Time
	batchResult   provisionExtractionResult
	batchResults  []provisionChunkResult // per-chunk results for final accumulation
	batchMu       sync.Mutex             // protects batchResults append under concurrent Phase 3
	batchSkip     bool                   // set true by InitChunkBatch when data exists and !force
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

// provisionChunkResult holds the output of one concurrent chunk worker for provisions.
type provisionChunkResult struct {
	payload    map[string]any
	modelName  string
	isFallback bool
	numInChunk int
}

func NewProvisionsProcessor(inputStore DocMetadataStore, store ProvisionsStore, extractor LLMJSONExtractor, _ ApiTypes.JimoLogger) *ProvisionsProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26050520")
	promptText, promptRef, promptPath, promptErr := loadProvisionsPromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_PROVISIONS_MODEL_NAME", "EXTRACT_PROVISIONS_MODELS_FILE")
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_PROVISIONS_MODEL_FALLBACK", "EXTRACT_PROVISIONS_MODELS_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	var fallbackExtractor LLMJSONExtractor
	if fallbackModelErr == nil && strings.TrimSpace(fallbackModelCfg.ModelName) != "" {
		fe := &llmclients.OpenAIJSONClient{
			HTTPClient: &http.Client{Timeout: 100 * time.Second},
		}
		applyStructureModelConfigToExtractor(fe, fallbackModelCfg)
		fallbackExtractor = fe
	}
	prevOverlap, nextOverlap, removeTOC := blockingConfigFromViper()
	p := &ProvisionsProcessor{
		InputStore:                inputStore,
		Store:                     store,
		Extractor:                 extractor,
		Logger:                    logger,
		ProcLogger:                DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                       time.Now,
		PromptText:                promptText,
		PromptRef:                 promptRef,
		PromptPath:                promptPath,
		PromptErr:                 promptErr,
		ModelRef:                  modelRef,
		ModelCfgPath:              modelCfgPath,
		ModelErr:                  modelErr,
		ModelName:                 modelCfg.ModelName,
		FallbackModelRef:          fallbackModelRef,
		FallbackModelCfgPath:      fallbackModelCfgPath,
		FallbackModelErr:          fallbackModelErr,
		FallbackModelName:         fallbackModelCfg.ModelName,
		FallbackExtractor:         fallbackExtractor,
		BlockSize:                 envInt("INPUT_BLOCK_SIZE", DefaultBlockingBlockSize, 1),
		PrevOverlap:               prevOverlap,
		NextOverlap:               nextOverlap,
		RemoveTOC:                 removeTOC,
		ArtifactDir:               strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:            strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
		ExtractProvisionsMaxTasks: envInt("EXTRACT_PROVISIONS_MAX_TASKS", 1, 1),
		ExtractProvisionsInput:    extractProvisionsInputFromEnv(),
	}
	if resolver, minConfidence, err := NewAmbiguousObjectLLMResolverFromEnv(); err == nil {
		p.AmbiguousObjectLLMResolver = resolver
		p.ResolveAmbiguousMinConfidence = minConfidence
	} else if logger != nil {
		logger.Warn("configure ambiguous object LLM resolver failed", "err", err)
	}
	if ApiTypes.ProjectDBHandle != nil {
		p.ObjectStore = ArtifactObjectSQLStore{DB: ApiTypes.ProjectDBHandle}
		p.ObjectReconciler = ObjectReconciler{Store: ObjectNodeSQLStore{DB: ApiTypes.ProjectDBHandle}, Options: ObjectReconcileOptionsFromEnv()}
	}
	return p
}

func extractProvisionsInputFromEnv() string {
	if strings.ToLower(strings.TrimSpace(os.Getenv("EXTRACT_PROVISIONS_INPUT"))) == "blocks" {
		return "blocks"
	}
	return "chunks"
}

func (p *ProvisionsProcessor) Name() string { return "extract_provisions" }

func (p *ProvisionsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26050521) parse event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
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
			reindexExistingSearchOnSkip(ctx, searchArtifactProvision, evt.RecordID, p.Logger, ReindexProvisionSearchForRecord)
			p.persistProvisionsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	p.persistProvisionsRunningStatus(ctx, rec, start)

	var result provisionExtractionResult
	var numUnits int
	var extractErr error

	var chunkBlocks []Block
	if p.ExtractProvisionsInput == "blocks" {
		var blocks []Block
		blocks, extractErr = p.resolveBlocks(ctx, evt, rec)
		if extractErr != nil {
			p.persistProvisionsStatus(ctx, rec, start, extractErr)
			p.Logger.Error("resolveBlocks error", "error", extractErr, "record_id", evt.RecordID)
			return nil
		}
		if len(blocks) == 0 {
			e := fmt.Errorf("(MID_26050526) no blocks found for record_id=%d", evt.RecordID)
			p.persistProvisionsStatus(ctx, rec, start, e)
			return nil
		}
		numUnits = len(blocks)
		chunkBlocks = blocks
		result, extractErr = p.extractProvisionsFromBlocksWithLLM(ctx, blocks, evt.RecordID)
	} else {
		var chunks []Chunk
		chunks, extractErr = p.resolveChunks(ctx, evt, rec)
		if extractErr != nil {
			p.persistProvisionsStatus(ctx, rec, start, extractErr)
			p.Logger.Error("resolveChunks error", "error", extractErr, "record_id", evt.RecordID)
			return nil
		}
		if len(chunks) == 0 {
			e := fmt.Errorf("(MID_26053115) no chunks found for record_id=%d", evt.RecordID)
			p.persistProvisionsStatus(ctx, rec, start, e)
			return nil
		}
		numUnits = len(chunks)
		chunkBlocks = chunksToBlocks(chunks)
		result, extractErr = p.extractProvisionsFromChunksWithLLM(ctx, chunks, evt.RecordID, buildDocContextLine(rec))
	}
	if extractErr != nil {
		if errors.Is(extractErr, ErrPipelineStopped) {
			p.stopAndPersistProvisions(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistProvisionsStatus(ctx, rec, start, extractErr)
		return nil
	}
	outputRows := p.buildProvisionOutputRows(evt.RecordID, result.Provisions, start, numUnits, time.Since(start).Milliseconds(), result.ModelName)

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
	if err := p.persistProvisionObjects(ctx, evt.RecordID, outputRows); err != nil {
		p.persistProvisionsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26050754) persist provision objects failed: %w", err)
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
	// Derive chunk -> provision "has-provision" line_overlap edges (registry-sourced ids).
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, evt.RecordID, searchArtifactProvision, RelationHasProvision, chunkBlocks); connErr != nil {
		p.Logger.Warn("write has-provision connections failed", "record_id", evt.RecordID, "error", connErr)
	}
	p.Logger.Info("provisions extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"provisions_count", len(outputRows),
		"units", numUnits,
	)
	p.persistProvisionsStatus(ctx, rec, start, nil)
	p.logProvisionsSummary(ctx, start, p.Now(), result, inserted, numUnits, evt.RecordID)
	return nil
}

// callExtractor returns the extractor whose most recent LLM call should be read for
// provider prompt-cache counters: the fallback when the fallback model was used.
func (p *ProvisionsProcessor) callExtractor(usedFallback bool) LLMJSONExtractor {
	if usedFallback && p.FallbackExtractor != nil {
		return p.FallbackExtractor
	}
	return p.Extractor
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
		"provisions_before": provisionsSoFar,
		"percent":           fmt.Sprintf("%d%%", percent),
	}
	extraJSON, _ := json.Marshal(extraInfo)
	extraStr := string(extraJSON)
	callReason := p.Name()
	usedFallback := false
	if fb := strings.TrimSpace(p.FallbackModelName); fb != "" {
		for _, m := range modelNames {
			if strings.TrimSpace(m) == fb {
				usedFallback = true
				break
			}
		}
	}
	cacheHit, cacheMiss := extractorCacheTokens(p.callExtractor(usedFallback))
	rec := DocProcLogRecord{
		CallReason:            callReason,
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            promptName,
		RecordID:              &recordID,
		ProcProgress:          &progress,
		LLMCallID:             &callID,
		ActivityName:          &activity,
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		ExtraInfoJSON:         &extraStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if err := p.ProcLogger.LogExtractProvisions(ctx, rec, "MID-26052831"); err != nil {
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
		CallReason:    p.Name(),
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    p.PromptRef,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogExtractProvisionsFinish(ctx, rec, "MID-26052832"); err != nil {
		p.Logger.Warn("failed to write extract_provisions log", "error", err)
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

func (p *ProvisionsProcessor) resolveChunks(ctx context.Context, evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Chunk, error) {
	if buf := ChunkBufferFromContext(ctx); buf != nil {
		return buf.Chunks, nil
	}
	lineFilePath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("(MID_26053101) resolve input file for record_id=%d: %w", evt.RecordID, err)
	}
	body, err := os.ReadFile(lineFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("(MID_26053102) input file not exist: %s", lineFilePath)
		}
		return nil, fmt.Errorf("(MID_26053103) read input file: %w", err)
	}
	lines, err := ParseInputLinesIncludingTOC(body)
	if err != nil {
		return nil, fmt.Errorf("(MID_26053104) parse input lines: %w", err)
	}
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, err := loadChunksFromArtifactFile(artifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if err != nil {
		return nil, fmt.Errorf("(MID_26053105) load chunk artifact: %w", err)
	}
	return chunks, nil
}

func (p *ProvisionsProcessor) extractProvisionsFromChunksWithLLM(ctx context.Context, chunks []Chunk, recordID int64, docCtx string) (provisionExtractionResult, error) {
	extractID := p.Now().Format("20060102-150405")

	results, runErr := runConcurrent(ctx, p.ExtractProvisionsMaxTasks, len(chunks), func(jobCtx context.Context, idx int) (provisionChunkResult, error) {
		if isCtxStopped(jobCtx) {
			return provisionChunkResult{}, ErrPipelineStopped
		}
		chunk := chunks[idx]
		callStart := p.Now()
		p.Logger.Info("extract provisions start",
			"record_id", recordID,
			"chunk", idx,
			"total", len(chunks),
			"model name", p.ModelName,
			"prompt name", p.PromptRef,
		)
		callID := fmt.Sprintf("%s_p1_c%d", eventIDFromContext(ctx), idx)
		// Canonical chunk InputText (shared, cacheable prefix); schema/task in the prompt.
		// See ADR 2026062701 §Phase 2.3.
		inputText := canonicalChunkInputText(chunk.Lines, docCtx)
		payload, modelName, err := p.extractProvisionPayloadWithFallback(jobCtx, inputText, provisionChunkTask(p.PromptText, chunk))
		isFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != ""
		numInChunk := 0
		if err == nil && payload != nil {
			if raw, ok := payload["provisions"].([]any); ok {
				numInChunk = len(raw)
			}
		}
		p.logLLMCall(ctx, callID, "extract_provisions",
			[]string{strings.TrimSpace(modelName)}, p.PromptRef,
			payload, err, callStart, p.Now(),
			recordID, idx, len(chunks), 0, numInChunk)
		if err != nil {
			p.Logger.Error("failed extracting from chunk", "error", err)
			return provisionChunkResult{}, fmt.Errorf("(MID_26053106) extract provisions from chunk via llm: %w", err)
		}
		usedFallback := strings.TrimSpace(p.FallbackModelName) != "" && strings.TrimSpace(modelName) == strings.TrimSpace(p.FallbackModelName)
		cacheHit, cacheMiss := cacheTokenCounts(p.callExtractor(usedFallback))
		p.Logger.Info("extract provisions end  ",
			"record_id", recordID,
			"extracted provisions", numInChunk,
			"chunk", idx,
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
			"ms_used", time.Since(callStart).Milliseconds(),
		)
		return provisionChunkResult{
			payload:    payload,
			modelName:  modelName,
			isFallback: isFallback,
			numInChunk: numInChunk,
		}, nil
	})

	if runErr != nil {
		if errors.Is(runErr, ErrPipelineStopped) {
			return provisionExtractionResult{}, ErrPipelineStopped
		}
		return provisionExtractionResult{}, runErr
	}

	language := ""
	provisions := make([]map[string]any, 0, len(chunks))
	usedModelName := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount int

	for i, res := range results {
		llmCallCount++
		if res.isFallback {
			fallbackCount++
		}
		if strings.TrimSpace(res.modelName) != "" {
			usedModelName = strings.TrimSpace(res.modelName)
		}
		if language == "" {
			language = strings.TrimSpace(asString(res.payload["language"]))
		}
		raw := res.payload["provisions"].([]any)
		provisions = append(provisions, normalizeProvisionList(raw, chunkLineToPage(chunks[i]), chunkLineText(chunks[i]))...)
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

// provisionChunkTask is the chunk-mode task prompt (base prompt + output schema + chunk index).
// The chunk lines are NOT included here — they are the canonical InputText (document section)
// under document-first layout, shared byte-for-byte with the other chunk processors so DeepSeek
// can reuse the prefix. See ADR 2026062701 §Phase 2.3.
func provisionChunkTask(base string, chunk Chunk) string {
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
			"source_line_spans": []string{"12", "16-19"},
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
			"objects": []map[string]any{{
				"object_name":       "string",
				"object_name_en":    "string",
				"object_name_zh":    "string",
				"object_type":       "equipment|material|system|process|organization|person|place|document|concept|other",
				"object_role":       "regulated_object|requirement_target|component|parent_system|other",
				"aliases":           []string{"string"},
				"acronyms":          []string{"string"},
				"description":       "string",
				"evidence_quote":    "string",
				"source_line_spans": []string{"12", "16-19"},
				"confidence":        0.0,
			}},
			"category_paths":   []any{},
			"category_path_en": []any{},
		}},
	}
	schemaJSON, _ := json.Marshal(schema)
	task := "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nChunk index: " + strconv.Itoa(chunk.SeqNo)
	if strings.TrimSpace(base) == "" {
		return task
	}
	return strings.TrimSpace(base) + "\n\n" + task
}

func chunkLineToPage(chunk Chunk) map[int]int {
	out := make(map[int]int, len(chunk.Lines))
	for _, ml := range chunk.Lines {
		if ml.Line.LineNo > 0 && ml.Line.PageNo > 0 {
			out[ml.Line.LineNo] = ml.Line.PageNo
		}
	}
	return out
}

func chunkLineText(chunk Chunk) map[string]string {
	out := make(map[string]string, len(chunk.Lines))
	for _, ml := range chunk.Lines {
		if ml.Line.LineNo <= 0 {
			continue
		}
		out[strconv.Itoa(ml.Line.LineNo)] = strings.TrimSpace(ml.Line.Content)
	}
	return out
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

	type blockResult struct {
		payload    map[string]any
		modelName  string
		isFallback bool
		numInBlock int
	}

	results, runErr := runConcurrent(ctx, p.ExtractProvisionsMaxTasks, len(blocks), func(jobCtx context.Context, idx int) (blockResult, error) {
		if isCtxStopped(jobCtx) {
			return blockResult{}, ErrPipelineStopped
		}
		block := blocks[idx]
		callStart := p.Now()
		p.Logger.Info("extract provisions start",
			"record_id", record_id,
			"idx", idx,
			"total", len(blocks),
			"model name", p.ModelName,
			"prompt name", p.PromptRef,
		)
		callID := fmt.Sprintf("%s_p1_b%d", eventIDFromContext(ctx), idx)
		// Block mode keeps its self-contained prompt (blocks are not shared across processors).
		payload, modelName, err := p.extractProvisionPayloadWithFallback(jobCtx, buildProvisionUserPrompt(block), p.PromptText)
		isFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != ""
		numInBlock := 0
		if err == nil && payload != nil {
			if raw, ok := payload["provisions"].([]any); ok {
				numInBlock = len(raw)
			}
		}
		p.logLLMCall(ctx, callID, "extract_provisions",
			[]string{strings.TrimSpace(modelName)}, p.PromptRef,
			payload, err, callStart, p.Now(),
			record_id, idx, len(blocks), 0, numInBlock)
		if err != nil {
			p.Logger.Error("failed extracting", "error", err)
			return blockResult{}, fmt.Errorf("(MID_26050531) extract provisions via llm: %w", err)
		}
		p.Logger.Info("extract provisions end  ",
			"record_id", record_id,
			"extracted provisions", numInBlock,
			"ms_used", time.Since(callStart).Milliseconds())
		return blockResult{
			payload:    payload,
			modelName:  modelName,
			isFallback: isFallback,
			numInBlock: numInBlock,
		}, nil
	})

	if runErr != nil {
		if errors.Is(runErr, ErrPipelineStopped) {
			return provisionExtractionResult{}, ErrPipelineStopped
		}
		return provisionExtractionResult{}, runErr
	}

	language := ""
	provisions := make([]map[string]any, 0, len(blocks))
	usedModelName := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount int

	for i, res := range results {
		llmCallCount++
		if res.isFallback {
			fallbackCount++
		}
		if strings.TrimSpace(res.modelName) != "" {
			usedModelName = strings.TrimSpace(res.modelName)
		}
		if language == "" {
			language = strings.TrimSpace(asString(res.payload["language"]))
		}
		raw := res.payload["provisions"].([]any)
		provisions = append(provisions, normalizeProvisionList(raw, blockLineToPage(blocks[i]), blockLineText(blocks[i]))...)
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

func (p *ProvisionsProcessor) extractProvisionPayloadWithFallback(ctx context.Context, inputText, promptText string) (map[string]any, string, error) {
	payload, err := p.extractProvisionPayloadFromText(ctx, inputText, promptText, p.ModelName, p.Extractor)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}

	var payloadVal any = "nil"
	if payload != nil {
		payloadVal = payload
	}

	if ApiUtils.IsEmptyJSONResponse(err) {
		p.Logger.Info("primary provisions extraction returned empty JSON. Try fallback if available",
			"fallback_model", p.FallbackModelName,
		)
	} else {
		p.Logger.Warn("primary LLM failed extracting provisions. Try the fallback!",
			"error", err,
			"model_name", p.ModelName,
			"timeout_sec", extractorTimeoutSec(p.Extractor),
			"fallback_model", p.FallbackModelName,
			"payload", payloadVal)
	}

	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, strings.TrimSpace(p.ModelName),
			fmt.Errorf("(MID_26050820) extract provisions failed and fallback model not available, err:%w, model_name:%s", err, p.ModelName)
	}

	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26050544) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	fallbackExt := p.FallbackExtractor
	if fallbackExt == nil {
		fallbackExt = p.Extractor
	}
	payload, fallbackErr := p.extractProvisionPayloadFromText(ctx, inputText, promptText, fallbackModelName, fallbackExt)
	if fallbackErr != nil {
		if ApiUtils.IsEmptyJSONResponse(fallbackErr) {
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

func (p *ProvisionsProcessor) extractProvisionPayloadFromText(ctx context.Context, inputText, promptText string, modelName string, extractor LLMJSONExtractor) (map[string]any, error) {
	in := newLLMJSONInput(ctx, p.PromptRef, promptText, modelName, inputText, "extract_provisions", "MID-CWB-EXTRACT-PROVISIONS")
	var (
		payload map[string]any
		err     error
	)
	if structuredExtractor, ok := extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, provisionExtractionContract())
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, err = extractor.ExtractJSON(ctx, in)
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
			if _, ok := payload[key].([]string); ok {
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
			"source_line_spans": []string{"12", "16-19"},
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
			"objects": []map[string]any{{
				"object_name":       "string",
				"object_name_en":    "string",
				"object_name_zh":    "string",
				"object_type":       "equipment|material|system|process|organization|person|place|document|concept|other",
				"object_role":       "regulated_object|requirement_target|component|parent_system|other",
				"aliases":           []string{"string"},
				"acronyms":          []string{"string"},
				"description":       "string",
				"evidence_quote":    "string",
				"source_line_spans": []string{"12", "16-19"},
				"confidence":        0.0,
			}},
			"category_paths":   []any{},
			"category_path_en": []any{},
		}},
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nBlock index: " + strconv.Itoa(block.Index) +
		"\n\nInput block lines (JSON array):\n" + blockLinesToJSON(block.Lines)
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
		if line.LineNumber <= 0 {
			continue
		}
		out[strconv.Itoa(line.LineNumber)] = strings.TrimSpace(line.Content)
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
		if objects := objectItemsFromValue(raw["objects"]); len(objects) > 0 {
			normalized["objects"] = objects
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
	seen := make(map[string]struct{}, len(spans))
	for _, span := range spans {
		for _, lineNo := range expandLineSpan(span) {
			key := strconv.Itoa(lineNo)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			text := strings.TrimSpace(lineText[key])
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeProvisionSourceLineSpans(value any, lineToPage map[int]int) []string {
	_ = lineToPage
	lineNumbers := make([]int, 0, 4)

	appendSpanValue := func(item any) {
		switch v := item.(type) {
		case map[string]any:
			lineNo := int(toFloat(v["line_number"]))
			if lineNo > 0 {
				lineNumbers = append(lineNumbers, lineNo)
			}
		case float64:
			lineNo := int(v)
			if lineNo > 0 {
				lineNumbers = append(lineNumbers, lineNo)
			}
		case int:
			if v > 0 {
				lineNumbers = append(lineNumbers, v)
			}
		case int64:
			if v > 0 {
				lineNumbers = append(lineNumbers, int(v))
			}
		case string:
			s := strings.TrimSpace(v)
			if s == "" {
				return
			}
			if pageLine := strings.SplitN(s, ":", 2); len(pageLine) == 2 {
				if lineNo, err := strconv.Atoi(strings.TrimSpace(pageLine[1])); err == nil && lineNo > 0 {
					lineNumbers = append(lineNumbers, lineNo)
					return
				}
			}
			if _, _, ok := parseMetricLineSpan(s); ok {
				lineNumbers = append(lineNumbers, expandLineSpan(s)...)
				return
			}
			if lineNo, err := strconv.Atoi(s); err == nil && lineNo > 0 {
				lineNumbers = append(lineNumbers, lineNo)
			}
		}
	}

	switch v := value.(type) {
	case []any:
		for _, item := range v {
			appendSpanValue(item)
		}
	case []string:
		for _, item := range v {
			appendSpanValue(item)
		}
	case []int:
		for _, item := range v {
			appendSpanValue(item)
		}
	case []int64:
		for _, item := range v {
			appendSpanValue(item)
		}
	case []float64:
		for _, item := range v {
			appendSpanValue(item)
		}
	case string, map[string]any, float64, int, int64:
		appendSpanValue(v)
	default:
		return nil
	}
	if len(lineNumbers) == 0 {
		return nil
	}
	return lineRangesFromNumbers(lineNumbers)
}

func (p *ProvisionsProcessor) buildProvisionOutputRows(recordID int64, provisions []map[string]any, now time.Time, numBlocks int, durationMs int64, modelName string) []map[string]any {
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
			"prov_id":               fmt.Sprintf("%d_prv_%d", recordID, i+1),
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
			"objects":               provision["objects"],
		})
	}
	return out
}

func (p *ProvisionsProcessor) persistProvisionObjects(ctx context.Context, recordID int64, provisions []map[string]any) error {
	if p.ObjectStore == nil {
		return nil
	}
	objects := make([]ArtifactObject, 0)
	for _, provision := range provisions {
		provID := strings.TrimSpace(asString(provision["prov_id"]))
		if provID == "" {
			continue
		}
		objects = append(objects, normalizeArtifactObjectsForArtifact(recordID, searchArtifactProvision, provID, provision)...)
	}
	if p.ObjectReconciler.Store != nil && len(objects) > 0 {
		reconciled, err := reconcileArtifactObjectsWithLLM(ctx, objects, p.ObjectReconciler, p.Logger, p.AmbiguousObjectLLMResolver, p.ResolveAmbiguousMinConfidence, objectReconcileLogSink{ProcLogger: p.ProcLogger, DocProcName: p.Name(), CallReason: p.Name()})
		if err != nil {
			return err
		}
		objects = reconciled
	}
	return p.ObjectStore.ReplaceObjectsForRecord(ctx, recordID, searchArtifactProvision, objects)
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
		"prov_id":               strings.TrimSpace(asString(provision["prov_id"])),
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
		"objects":               provision["objects"],
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
		entry := strings.TrimSpace(asString(prov["prov_id"]))
		if entry == "" {
			continue
		}
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
		"prov_id":             strings.TrimSpace(asString(row["prov_id"])),
		"prov_name":           strings.TrimSpace(asString(row["prov_name"])),
		"prov_name_en":        strings.TrimSpace(asString(row["prov_name_en"])),
		"prov_type":           strings.TrimSpace(asString(publicInfo["provision_type"])),
		"provision":           strings.TrimSpace(asString(row["provision"])),
		"provision_en":        strings.TrimSpace(asString(row["provision_en"])),
		"provision_desc":      strings.TrimSpace(asString(row["prov_desc"])),
		"provision_desc_en":   strings.TrimSpace(asString(row["prov_desc_en"])),
		"source_line_spans":   row["source_line_spans"],
		"context":             strings.TrimSpace(asString(row["prov_context"])),
		"context_en":          strings.TrimSpace(asString(row["prov_context_en"])),
		"subject":             strings.TrimSpace(asString(row["provision_subject"])),
		"subject_en":          strings.TrimSpace(asString(row["provision_subject_en"])),
		"location_type":       strings.TrimSpace(asString(row["location_type"])),
		"keywords":            row["provision_keywords"],
		"keywords_en":         row["provision_keywords_en"],
		"confidence":          toFloat(row["confidence"]),
		"is_explicit":         toBool(row["is_explicit"]),
		"need_verify":         toBool(row["need_verify"]),
		"category_paths":      row["category_paths"],
		"category_paths_en":   row["category_paths_en"],
		"objects":             row["objects"],
		"connected_artifacts": row["connected_artifacts"],
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
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendProvisionsStatus(current, provisionsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectProvisionsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	}); err != nil {
		p.Logger.Error("failed persisting provisions status", "record_id", rec.ID, "error", err)
	}
}

func (p *ProvisionsProcessor) persistProvisionsRunningStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	p.Logger.Info("(MID_26052910) updating kb.inputs.status extract_provisions entry to running",
		"record_id", rec.ID,
		"start_time", start.Format(defaultDocMetaStatusTime),
	)
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendProvisionsStatus(current, provisionsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectProvisionsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			ProcStatus:    "running",
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); err != nil {
		p.Logger.Error("(MID_26052912) failed persisting provisions running status", "record_id", rec.ID, "error", err)
		return
	}
}

func (p *ProvisionsProcessor) stopAndPersistProvisions(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendProvisionsStatus(current, provisionsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectProvisionsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
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
		promptRef = "prompt-extract-provisions-v3.md"
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
			req.InputRecordID, // $1
			req.ExtractID,     // $2
			req.InputFilename, // $3
			strings.TrimSpace(asString(dbRecord["prov_id"])),                                // $4
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

// ---- Phase C artifact file refresh ----

// loadPersistedProvisionArtifactRows loads all provision rows for a record from the DB,
// including connected_artifacts populated by Phase C, and returns them as rows ready
// for buildProvisionFileRecord.
func loadPersistedProvisionArtifactRows(ctx context.Context, db *sql.DB, recordID int64) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT COALESCE(prov_id, ''),
		       COALESCE(prov_name, ''),
		       COALESCE(prov_name_en, ''),
		       COALESCE(provision_type, ''),
		       COALESCE(source_line_spans, '[]'::jsonb),
		       COALESCE(provision, ''),
		       COALESCE(provision_en, ''),
		       COALESCE(provision_subject, ''),
		       COALESCE(provision_subject_en, ''),
		       COALESCE(prov_desc, ''),
		       COALESCE(prov_desc_en, ''),
		       COALESCE(prov_context, ''),
		       COALESCE(prov_context_en, ''),
		       COALESCE(provision_keywords, '[]'::jsonb),
		       COALESCE(provision_keywords_en, '[]'::jsonb),
		       COALESCE(category_paths, '[]'::jsonb),
		       COALESCE(category_paths_en, '[]'::jsonb),
		       COALESCE(location_type, ''),
		       COALESCE(confidence, 0),
		       COALESCE(is_explicit, false),
		       COALESCE(need_verify, false),
		       COALESCE(public_info, '{}'::jsonb),
		       kb.connected_artifacts(input_record_id, 'provision', id)
		FROM kb.provisions
		WHERE input_record_id = $1
		ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []map[string]any
	for rows.Next() {
		var (
			provID, provName, provNameEn         string
			provisionType                        string
			sourceLineSpansRaw                   []byte
			provision, provisionEn               string
			provisionSubject, provisionSubjectEn string
			provDesc, provDescEn                 string
			provContext, provContextEn           string
			keywordsRaw, keywordsEnRaw           []byte
			categoryPathsRaw, categoryPathsEnRaw []byte
			locationType                         string
			confidence                           float64
			isExplicit, needVerify               bool
			publicInfoRaw                        []byte
			connectedArtifactsRaw                []byte
		)
		if err := rows.Scan(
			&provID, &provName, &provNameEn,
			&provisionType,
			&sourceLineSpansRaw,
			&provision, &provisionEn,
			&provisionSubject, &provisionSubjectEn,
			&provDesc, &provDescEn,
			&provContext, &provContextEn,
			&keywordsRaw, &keywordsEnRaw,
			&categoryPathsRaw, &categoryPathsEnRaw,
			&locationType,
			&confidence, &isExplicit, &needVerify,
			&publicInfoRaw,
			&connectedArtifactsRaw,
		); err != nil {
			return nil, err
		}
		// Build public_info so buildProvisionFileRecord can read provision_type from it.
		publicInfo := jsonColumnObject(publicInfoRaw)
		if publicInfo == nil {
			publicInfo = map[string]any{}
		}
		if strings.TrimSpace(asString(publicInfo["provision_type"])) == "" && provisionType != "" {
			publicInfo["provision_type"] = provisionType
		}
		out = append(out, map[string]any{
			"prov_id":               strings.TrimSpace(provID),
			"prov_name":             strings.TrimSpace(provName),
			"prov_name_en":          strings.TrimSpace(provNameEn),
			"source_line_spans":     jsonColumnToValue(sourceLineSpansRaw, []any{}),
			"provision":             strings.TrimSpace(provision),
			"provision_en":          strings.TrimSpace(provisionEn),
			"provision_subject":     strings.TrimSpace(provisionSubject),
			"provision_subject_en":  strings.TrimSpace(provisionSubjectEn),
			"prov_desc":             strings.TrimSpace(provDesc),
			"prov_desc_en":          strings.TrimSpace(provDescEn),
			"prov_context":          strings.TrimSpace(provContext),
			"prov_context_en":       strings.TrimSpace(provContextEn),
			"provision_keywords":    jsonColumnToValue(keywordsRaw, []any{}),
			"provision_keywords_en": jsonColumnToValue(keywordsEnRaw, []any{}),
			"category_paths":        jsonColumnToValue(categoryPathsRaw, []any{}),
			"category_paths_en":     jsonColumnToValue(categoryPathsEnRaw, []any{}),
			"location_type":         strings.TrimSpace(locationType),
			"confidence":            confidence,
			"is_explicit":           isExplicit,
			"need_verify":           needVerify,
			"public_info":           publicInfo,
			"connected_artifacts":   jsonColumnToValue(connectedArtifactsRaw, map[string]any{}),
		})
	}
	return out, rows.Err()
}

func refreshProvisionArtifactFile(ctx context.Context, db *sql.DB, artifactDir string, recordID int64, rec DocMetadataInputRecord) error {
	rows, err := loadPersistedProvisionArtifactRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	fileRecords := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		fileRecords = append(fileRecords, buildProvisionFileRecord(row))
	}
	return writeEntityRelationArtifactFile(artifactDir, recordID, rec, fileRecords, ".provisions", "MID_26060704")
}

// ---- ChunkBatchProcessor implementation ----

// InitChunkBatch initialises the provisions processor for per-chunk batching.
// It validates the model configuration and stores the shared chunk data.
func (p *ProvisionsProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26062710) %s prompt error: %w", p.Name(), p.PromptErr)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("%s skipped: model config error", p.Name(),
			"record_id", recordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		return nil // skip silently, not an error
	}
	if p.ExtractProvisionsInput == "blocks" {
		return fmt.Errorf("(MID_26062711) %s does not support batch mode for blocks input", p.Name())
	}
	if p.InputStore == nil {
		return errors.New("(MID_26062712) input store is nil")
	}
	if p.Store == nil {
		return errors.New("(MID_26062713) provisions store is nil")
	}

	force, _ := docProcessorFlagsFromContext(ctx)
	p.batchSkip = false
	if force {
		_, _ = p.Store.DeleteProvisionsByInputRecordID(ctx, recordID)
	} else {
		exists, err := p.Store.ProvisionsExist(ctx, recordID, "")
		if err != nil {
			return fmt.Errorf("(MID_26071120) %s check provisions exist: %w", p.Name(), err)
		}
		if exists {
			p.Logger.Info("provisions extraction skipped", "record_id", recordID, "reason", "provisions already exist and force=false")
			reindexExistingSearchOnSkip(ctx, searchArtifactProvision, recordID, p.Logger, ReindexProvisionSearchForRecord)
			p.batchSkip = true
			return nil
		}
	}

	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchResults = make([]provisionChunkResult, 0, len(chunks))
	p.batchResult = provisionExtractionResult{
		ExtractID: p.Now().Format("20060102-150405"),
	}

	// Load the full record for finalization (need ResultFilename, StagingFilename, etc.)
	// We use the first input record we can find for this recordID.
	p.batchRec = DocMetadataInputRecord{ID: recordID} // minimal; fields filled on finalize
	// The coordinator has already loaded the record; we store what we need here.
	return nil
}

// ProcessChunk processes one chunk for the provisions processor: builds the
// LLM input, calls the LLM, and accumulates the result. Called sequentially
// by the per-chunk batching coordinator.
func (p *ProvisionsProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if p.batchSkip {
		return nil
	}

	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062714) %s chunk index %d out of range (len=%d)",
			p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}

	chunk := p.batchChunks[chunkIdx]
	callStart := p.Now()
	recordID := p.batchRecordID

	p.Logger.Info("extract provisions start (batch)",
		"record_id", recordID,
		"chunk", chunkIdx,
		"total", len(p.batchChunks),
		"model name", p.ModelName,
		"prompt name", p.PromptRef,
	)

	inputText := canonicalChunkInputText(chunk.Lines, p.batchDocCtx)
	payload, modelName, err := p.extractProvisionPayloadWithFallback(ctx, inputText, provisionChunkTask(p.PromptText, chunk))

	isFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != ""
	numInChunk := 0
	if err == nil && payload != nil {
		if raw, ok := payload["provisions"].([]any); ok {
			numInChunk = len(raw)
		}
	}

	// Log the LLM call.
	callID := fmt.Sprintf("%d_batch_c%d", recordID, chunkIdx)
	p.logLLMCall(ctx, callID, "extract_provisions",
		[]string{strings.TrimSpace(modelName)}, p.PromptRef,
		payload, err, callStart, p.Now(),
		recordID, chunkIdx, len(p.batchChunks), 0, numInChunk)

	if err != nil {
		p.Logger.Error("provisions batch: failed extracting from chunk",
			"record_id", recordID, "chunk", chunkIdx, "error", err)
		return fmt.Errorf("(MID_26062715) %s chunk %d: %w", p.Name(), chunkIdx, err)
	}

	usedFallback := strings.TrimSpace(p.FallbackModelName) != "" && strings.TrimSpace(modelName) == strings.TrimSpace(p.FallbackModelName)
	cacheHit, cacheMiss := cacheTokenCounts(p.callExtractor(usedFallback))
	p.Logger.Info("extract provisions end  (batch)",
		"record_id", recordID,
		"extracted provisions", numInChunk,
		"chunk", chunkIdx,
		"cache_hit", cacheHit,
		"cache_miss", cacheMiss,
		"ms_used", time.Since(callStart).Milliseconds(),
	)

	p.batchMu.Lock()
	p.batchResults = append(p.batchResults, provisionChunkResult{
		payload:    payload,
		modelName:  modelName,
		isFallback: isFallback,
		numInChunk: numInChunk,
	})
	p.batchMu.Unlock()
	return nil
}

// FinalizeChunkBatch saves all accumulated provisions from batch processing
// to the database, writes artifacts, and indexes the results.
func (p *ProvisionsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if p.batchSkip {
		return nil
	}

	if len(p.batchResults) == 0 {
		p.Logger.Info("provisions batch: no results to finalize", "record_id", p.batchRecordID)
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}

	// Accumulate results (same logic as extractProvisionsFromChunksWithLLM post-runConcurrent).
	language := ""
	provisions := make([]map[string]any, 0, len(p.batchResults))
	usedModelName := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount int

	for i, res := range p.batchResults {
		llmCallCount++
		if res.isFallback {
			fallbackCount++
		}
		if strings.TrimSpace(res.modelName) != "" {
			usedModelName = strings.TrimSpace(res.modelName)
		}
		if language == "" {
			language = strings.TrimSpace(asString(res.payload["language"]))
		}
		raw := res.payload["provisions"].([]any)
		provisions = append(provisions, normalizeProvisionList(raw, chunkLineToPage(p.batchChunks[i]), chunkLineText(p.batchChunks[i]))...)
	}

	if language == "" {
		language = "unknown"
	}

	// Fill extraction result.
	p.batchResult = provisionExtractionResult{
		ExtractID:     p.batchResult.ExtractID,
		Language:      language,
		Provisions:    provisions,
		ModelName:     firstNonEmptyTrimmed(usedModelName, p.ModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
	}

	// Determine input filename (needed for DB save).
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("provisions batch: record not found", "record_id", p.batchRecordID)
			return nil
		}
		return fmt.Errorf("(MID_26062716) %s load record for finalize: %w", p.Name(), err)
	}
	p.batchRec = rec

	inputFilename := filepath.Base(strings.TrimSpace(rec.ResultFilename))
	if inputFilename == "" {
		inputFilename = fmt.Sprintf("record_%d", p.batchRecordID)
	}

	outputRows := p.buildProvisionOutputRows(p.batchRecordID, p.batchResult.Provisions, p.batchStart,
		len(p.batchChunks), time.Since(p.batchStart).Milliseconds(), p.batchResult.ModelName)

	inserted, err := p.Store.SaveProvisions(ctx, SaveProvisionsRequest{
		InputRecordID: p.batchRecordID,
		InputFilename: inputFilename,
		ExtractID:     p.batchResult.ExtractID,
		Language:      p.batchResult.Language,
		Provisions:    outputRows,
	})
	if err != nil {
		return fmt.Errorf("(MID_26062717) %s save provisions: %w", p.Name(), err)
	}
	if err := p.persistProvisionObjects(ctx, p.batchRecordID, outputRows); err != nil {
		return fmt.Errorf("(MID_26062718) %s persist provision objects: %w", p.Name(), err)
	}

	if err := p.indexProvisionsInTree(p.batchRecordID, outputRows); err != nil {
		p.Logger.Error("provisions batch: index tree error", "record_id", p.batchRecordID, "error", err)
		// non-fatal
	}
	if err := p.writeProvisionsArtifact(p.batchRecordID, rec, outputRows); err != nil {
		p.Logger.Error("provisions batch: write artifact error", "record_id", p.batchRecordID, "error", err)
		// non-fatal
	}
	if reindexErr := ReindexProvisionSearchForRecord(ctx, p.batchRecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("provisions batch: reindex search failed", "record_id", p.batchRecordID, "error", reindexErr)
	}
	// Derive chunk -> provision "has-provision" line_overlap edges.
	chunkBlocks := chunksToBlocks(p.batchChunks)
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, p.batchRecordID, searchArtifactProvision, RelationHasProvision, chunkBlocks); connErr != nil {
		p.Logger.Warn("provisions batch: write connections failed", "record_id", p.batchRecordID, "error", connErr)
	}

	p.Logger.Info("provisions batch: extraction complete",
		"record_id", p.batchRecordID,
		"inserted_rows", inserted,
		"provisions_count", len(outputRows),
		"units", len(p.batchChunks),
	)
	p.persistProvisionsStatus(ctx, rec, p.batchStart, nil)
	p.logProvisionsSummary(ctx, p.batchStart, p.Now(), p.batchResult, inserted, len(p.batchChunks), p.batchRecordID)

	return nil
}
