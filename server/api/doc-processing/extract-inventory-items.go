package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/ApiUtils"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

const (
	inventoryItemsSchemaVersion = "1"
	searchArtifactInventoryItem = "inventory_item"
)

type InventoryItemsProcessor struct {
	InputStore                    DocMetadataStore
	Store                         InventoryItemsStore
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
	ModelCfg                      structureModelConfig
	FallbackModelRef              string
	FallbackModelPath             string
	FallbackModelErr              error
	FallbackModelName             string
	FallbackModelCfg              structureModelConfig
	ArtifactDir                   string
	Dictionary                    inventoryDictionary
	DictionaryDir                 string
	DictionaryErr                 error
	ExtractInventoryItemsMaxTasks int
	CategoryRegistry              InventoryCategoryRegistry
	CategoryCurator               InventoryCategoryCurator
	categorySeedDone              bool
	ObjectStore                   ArtifactObjectsStore
	ObjectReconciler              ObjectReconciler
	AmbiguousObjectLLMResolver    AmbiguousObjectLLMResolver
	ResolveAmbiguousMinConfidence float64

	// batch state (set by ChunkBatchProcessor.InitChunkBatch)
	batchRecordID int64
	batchChunks   []Chunk
	batchDocCtx   string
	batchResults  []inventoryChunkOutcome
	batchMu       sync.Mutex // protects batchResults append under concurrent Phase 3
	batchStart    time.Time
}

type InventoryItemsStore interface {
	InventoryItemsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteInventoryItemsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveInventoryItems(ctx context.Context, req SaveInventoryItemsRequest) (int64, error)
	DeleteInventoryItemDuplicatesByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveInventoryItemDuplicates(ctx context.Context, req SaveInventoryItemDuplicatesRequest) (int64, error)
}

type InventoryItemsSQLStore struct {
	DB *sql.DB
}

type SaveInventoryItemsRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Items         []map[string]any
}

// SaveInventoryItemDuplicatesRequest carries the discarded duplicate rows for a
// record. Each item must already have inventory_item_id and duplicate_of set.
type SaveInventoryItemDuplicatesRequest struct {
	InputRecordID int64
	EventID       string
	Language      string
	ModelName     string
	PromptName    string
	Items         []map[string]any
}

type inventoryItemsExtractionResult struct {
	Language      string
	Items         []map[string]any
	Duplicates    []map[string]any
	ModelName     string
	LLMCallCount  int
	FallbackCount int
	FailedChunks  int
}

type inventoryDictionary struct {
	Version         string
	Categories      map[string]inventoryCategorySchema
	Units           map[string]inventoryUnitRule
	Aliases         map[string]string
	Standards       map[string][]string
	PlausibleRanges map[string]map[string]InventoryPlausibleRange
}

type inventoryCategorySchema struct {
	RequiredAttrs []string                       `json:"required_attrs"`
	Specs         map[string]InventorySpecSchema `json:"specs"`
}

type InventorySpecSchema struct {
	CanonicalUnit string   `json:"canonical_unit"`
	Aliases       []string `json:"aliases"`
}

type inventoryUnitRule struct {
	Canonical string  `json:"canonical"`
	Factor    float64 `json:"factor"`
}

type InventoryPlausibleRange struct {
	Min  *float64 `json:"min"`
	Max  *float64 `json:"max"`
	Unit string   `json:"unit"`
}

func NewInventoryItemsProcessor(
	inputStore DocMetadataStore,
	store InventoryItemsStore,
	extractor LLMJSONExtractor,
	_ ApiTypes.JimoLogger,
) *InventoryItemsProcessor {
	logger := loggerutil.CreateDefaultLogger("MID_26053101")
	promptText, promptRef, promptPath, promptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_INVENTORY_ITEMS_PROMPT"},
		"prompt-extract-inventory-items-v3.md",
	)
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_INVENTORY_ITEMS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackModelRef, fallbackModelPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv(
		"EXTRACT_INVENTORY_ITEMS_FALLBACK",
		"MODEL_DEF_FILE",
	)
	dictDir := resolveInventoryDictionaryDir()
	dict, dictErr := loadInventoryDictionaryDir(dictDir)
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	registry := InventoryCategoryRegistry{DB: ApiTypes.ProjectDBHandle}
	categoryEmbedModelRef := strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME"))
	if categoryEmbedModelRef == "" {
		logger.Error("EMBEDDING_MODEL_NAME is not defined")
		panic("EMBEDDING_MODEL_NAME is not defined")
	}
	var categoryEmbedder Embedder
	var categoryEmbedModelName string
	if _, _, embCfg, embErr := loadModelConfigFromEnv("EMBEDDING_MODEL_NAME", ""); embErr == nil && strings.TrimSpace(embCfg.ModelName) != "" {
		categoryEmbedder = newOpenAIJSONClientForStructureModel(embCfg, 0)
		categoryEmbedModelName = embCfg.ModelName
	} else if e, ok := extractor.(Embedder); ok {
		categoryEmbedder = e
		categoryEmbedModelName = categoryEmbedModelRef
	}
	curator := InventoryCategoryCurator{
		Registry:       registry,
		Embedder:       categoryEmbedder,
		EmbedModelName: categoryEmbedModelName,
		FuzzyThreshold: envFloat("INVENTORY_CATEGORY_FUZZY_THRESHOLD", defaultInventoryCategoryFuzzyThreshold, 0),
		Logger:         logger,
	}
	p := &InventoryItemsProcessor{
		InputStore:                    inputStore,
		Store:                         store,
		Extractor:                     extractor,
		Logger:                        logger,
		ProcLogger:                    DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                           time.Now,
		PromptText:                    promptText,
		PromptRef:                     promptRef,
		PromptPath:                    promptPath,
		PromptErr:                     promptErr,
		ModelRef:                      modelRef,
		ModelCfgPath:                  modelCfgPath,
		ModelErr:                      modelErr,
		ModelName:                     modelCfg.ModelName,
		ModelCfg:                      modelCfg,
		FallbackModelRef:              fallbackModelRef,
		FallbackModelPath:             fallbackModelPath,
		FallbackModelErr:              fallbackModelErr,
		FallbackModelName:             fallbackModelCfg.ModelName,
		FallbackModelCfg:              fallbackModelCfg,
		ArtifactDir:                   strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		Dictionary:                    dict,
		DictionaryDir:                 dictDir,
		DictionaryErr:                 dictErr,
		ExtractInventoryItemsMaxTasks: envInt("EXTRACT_INTENTORY_ITEMS_MAX_TASKS", 1, 1),
		CategoryRegistry:              registry,
		CategoryCurator:               curator,
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

func (p *InventoryItemsProcessor) Name() string { return "extract_inventory_items" }

func (p *InventoryItemsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26053102) parse event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26053103) load inventory items prompt %q: %w", p.PromptRef, p.PromptErr)
	}
	if p.DictionaryErr != nil {
		return fmt.Errorf("(MID_26053104) load inventory dictionary %q: %w", p.DictionaryDir, p.DictionaryErr)
	}
	if p.ModelErr != nil {
		return fmt.Errorf("(MID_26053119) load inventory items model config: %w", p.ModelErr)
	}
	if p.FallbackModelErr != nil {
		return fmt.Errorf("(MID_26053120) load inventory items fallback model config: %w", p.FallbackModelErr)
	}
	if p.InputStore == nil {
		return errors.New("(MID_26053105) input store is nil")
	}
	if p.Store == nil {
		return errors.New("(MID_26053106) inventory items store is nil")
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("(MID_26053107) load kb.inputs record %d: %w", evt.RecordID, err)
	}

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		p.persistInventoryItemsStatus(ctx, rec, start, lineFileErr)
		return nil
	}
	if evt.Force {
		_, _ = p.Store.DeleteInventoryItemsByInputRecordID(ctx, evt.RecordID)
		_, _ = p.Store.DeleteInventoryItemDuplicatesByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, existErr := p.Store.InventoryItemsExist(ctx, evt.RecordID)
		if existErr != nil {
			p.persistInventoryItemsStatus(ctx, rec, start, existErr)
			return fmt.Errorf("(MID_26053108) check inventory items for record_id=%d: %w", evt.RecordID, existErr)
		}
		if exists {
			reindexExistingSearchOnSkip(ctx, searchArtifactInventoryItem, evt.RecordID, p.Logger, ReindexInventoryItemSearchForRecord)
			p.persistInventoryItemsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		p.persistInventoryItemsStatus(ctx, rec, start, readErr)
		return fmt.Errorf("(MID_26053109) read line file: %w", readErr)
	}
	lines, parseErr := ParseInputLinesIncludingTOC(body)
	if parseErr != nil {
		p.persistInventoryItemsStatus(ctx, rec, start, parseErr)
		return fmt.Errorf("(MID_26053110) parse input lines: %w", parseErr)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, loadErr := loadChunksFromArtifactFile(p.ArtifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if loadErr != nil {
		p.persistInventoryItemsStatus(ctx, rec, start, loadErr)
		return fmt.Errorf("(MID_26053111) load chunk artifact: %w", loadErr)
	}
	if len(chunks) == 0 {
		procErr := fmt.Errorf("(MID_26053112) no chunks found for record_id=%d", evt.RecordID)
		p.persistInventoryItemsStatus(ctx, rec, start, procErr)
		return nil
	}

	p.persistInventoryItemsInProgressStatus(ctx, rec, start, fmt.Sprintf("0%% (0/%d)", len(chunks)))
	result, err := p.extractInventoryItemsFromChunks(ctx, evt.RecordID, chunks, buildDocContextLine(rec))
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistInventoryItems(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistInventoryItemsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26053113) extract inventory items: %w", err)
	}
	language := firstNonEmptyTrimmed(result.Language, "unknown")
	createTime := p.now().UTC().Format(time.RFC3339)
	for i := range result.Items {
		result.Items[i]["inventory_item_id"] = fmt.Sprintf("%d_inv_%d", evt.RecordID, i+1)
		result.Items[i]["create_time"] = createTime
	}
	// Point each discarded duplicate at the surviving row it collapsed into. The
	// survivor for a group is the row whose dedupe_key matches the duplicate's.
	survivorByDedupeKey := make(map[string]string, len(result.Items))
	for _, item := range result.Items {
		survivorByDedupeKey[asString(item["dedupe_key"])] = asString(item["inventory_item_id"])
	}
	for j := range result.Duplicates {
		result.Duplicates[j]["inventory_item_id"] = fmt.Sprintf("%d_dup_%d", evt.RecordID, j+1)
		result.Duplicates[j]["create_time"] = createTime
		result.Duplicates[j]["duplicate_of"] = survivorByDedupeKey[asString(result.Duplicates[j]["dedupe_key"])]
	}

	inserted, err := p.Store.SaveInventoryItems(ctx, SaveInventoryItemsRequest{
		InputRecordID: evt.RecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      language,
		ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
		PromptName:    p.PromptRef,
		Items:         result.Items,
	})
	if err != nil {
		p.persistInventoryItemsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26053114) save inventory items: %w", err)
	}
	if err := p.persistInventoryItemObjects(ctx, evt.RecordID, result.Items); err != nil {
		p.persistInventoryItemsStatus(ctx, rec, start, err)
		return fmt.Errorf("(MID_26053121) persist inventory item objects: %w", err)
	}
	// Persist the discarded duplicates to the audit table. Best-effort: the
	// survivors are already saved, so a failure here must not fail the document.
	if len(result.Duplicates) > 0 {
		if _, dupErr := p.Store.SaveInventoryItemDuplicates(ctx, SaveInventoryItemDuplicatesRequest{
			InputRecordID: evt.RecordID,
			EventID:       eventIDFromContext(ctx),
			Language:      language,
			ModelName:     firstNonEmptyTrimmed(result.ModelName, p.ModelName),
			PromptName:    p.PromptRef,
			Items:         result.Duplicates,
		}); dupErr != nil {
			p.Logger.Warn("save inventory item duplicates failed", "record_id", evt.RecordID, "error", dupErr)
		}
	}
	if fileErr := writeInventoryItemsArtifactFile(p.ArtifactDir, evt.RecordID, rec, result.Items); fileErr != nil {
		p.Logger.Warn("save inventory items artifact failed", "record_id", evt.RecordID, "error", fileErr)
	}
	if reindexErr := ReindexInventoryItemSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex inventory item search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}
	p.Logger.Info("inventory items extracted",
		"record_id", evt.RecordID,
		"inserted_items", inserted,
		"num_chunks", len(chunks),
		"language", language,
	)
	p.persistInventoryItemsStatus(ctx, rec, start, nil)
	p.logInventoryItemsSummary(ctx, start, p.now(), result, inserted, len(chunks), evt.RecordID)
	return nil
}

func (p *InventoryItemsProcessor) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

type inventoryChunkOutcome struct {
	items     []map[string]any
	modelName string
	language  string
	failed    bool
	fallback  bool
}

func (p *InventoryItemsProcessor) extractInventoryItemsFromChunks(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) (inventoryItemsExtractionResult, error) {
	eventID := eventIDFromContext(ctx)

	startTime := time.Now()

	p.Logger.Info("extract inventory items start",
		"record_id", recordID,
		"model_name", p.ModelName,
		"prompt", p.PromptRef,
	)

	outcomes, runErr := runConcurrent(ctx, p.ExtractInventoryItemsMaxTasks, len(chunks), func(runCtx context.Context, chunk_id int) (inventoryChunkOutcome, error) {
		localStart := time.Now()
		chunk := chunks[chunk_id]
		// Canonical chunk serialization shared across chunk processors (no task/label text
		// here; task instructions live in the prompt). See ADR 2026062701 §Phase 2.3.
		inputText := canonicalChunkInputText(chunk.Lines, docCtx)
		callStart := p.now()
		callID := fmt.Sprintf("%s_p1_c%d", eventID, chunk_id+1)
		p.Logger.Info("extract inventory items start",
			"record_id", recordID,
			"chunk_idx", chunk_id+1,
		)
		payload, modelName, err := p.extractInventoryItemsWithFallback(runCtx, inputText)
		if isCtxStopped(runCtx) {
			return inventoryChunkOutcome{}, ErrPipelineStopped
		}
		wasFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != ""
		var chunkItems []map[string]any
		var chunkLang string
		if err == nil && payload != nil {
			if lang := strings.TrimSpace(asString(payload["language"])); lang != "" {
				chunkLang = lang
			}
			chunkItems = normalizeInventoryItemRows(payload["items"], chunk.SeqNo, p.Dictionary)
		}
		// totalCount unknown during concurrent execution; pass 0
		p.logInventoryItemsLLMCall(ctx, callID, []string{strings.TrimSpace(modelName)}, payload, err, callStart, p.now(), recordID, chunk_id, len(chunks), len(chunkItems), 0)
		if err != nil {
			p.Logger.Warn("inventory item extraction failed for chunk; skipping", "record_id", recordID, "chunk_idx", chunk_id, "error", err)
			return inventoryChunkOutcome{failed: true, fallback: wasFallback}, nil
		}

		cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
		p.Logger.Info("extract inventory items end  ",
			"record_id", recordID,
			"chunk_idx", chunk_id+1,
			"extracted", len(chunkItems),
			"ms_used", time.Since(localStart).Milliseconds(),
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
		)

		return inventoryChunkOutcome{
			items:     chunkItems,
			modelName: modelName,
			language:  chunkLang,
			fallback:  wasFallback,
		}, nil
	})
	if runErr != nil {
		return inventoryItemsExtractionResult{}, runErr
	}

	items := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	usedModel := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount, failedChunks int
	for _, outcome := range outcomes {
		llmCallCount++
		if outcome.fallback {
			fallbackCount++
		}
		if outcome.failed {
			failedChunks++
			continue
		}
		if outcome.language != "" && detectedLanguage == "unknown" {
			detectedLanguage = outcome.language
		}
		if strings.TrimSpace(outcome.modelName) != "" {
			usedModel = strings.TrimSpace(outcome.modelName)
		}
		items = append(items, outcome.items...)
	}

	survivors, duplicates := dedupeInventoryItemRows(items)

	p.Logger.Info("extract inventory items finished",
		"record_id", recordID,
		"extracted", len(items),
		"unique", len(survivors),
		"duplicates", len(duplicates),
		"ms_used", time.Since(startTime).Milliseconds(),
	)

	return inventoryItemsExtractionResult{
		Language:      detectedLanguage,
		Items:         survivors,
		Duplicates:    duplicates,
		ModelName:     usedModel,
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		FailedChunks:  failedChunks,
	}, nil
}

func (p *InventoryItemsProcessor) extractInventoryItemsWithFallback(ctx context.Context, inputText string) (map[string]any, string, error) {
	payload, err := p.extractInventoryItemsPayload(ctx, inputText, p.ModelName, p.ModelCfg)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}
	primaryModelName := strings.TrimSpace(p.ModelName)
	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, primaryModelName, err
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26053115) primary inventory extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}
	fallbackPayload, fallbackErr := p.extractInventoryItemsPayload(ctx, inputText, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if ApiUtils.IsEmptyJSONResponse(fallbackErr) {
			return map[string]any{"language": "unknown", "items": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26053116) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return fallbackPayload, fallbackModelName, nil
}

func (p *InventoryItemsProcessor) extractInventoryItemsPayload(ctx context.Context, inputText string, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	in := newLLMJSONInput(ctx, p.PromptRef, p.PromptText, modelName, inputText, "extract_inventory_items", "MID-CWB-EXTRACT-INVENTORY-ITEMS")
	var (
		payload map[string]any
		err     error
	)
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, inventoryItemsExtractionContract())
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, err = p.Extractor.ExtractJSON(ctx, in)
	}
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errors.New("(MID_26053117) empty inventory items payload")
	}
	if _, ok := payload["items"]; !ok {
		return nil, fmt.Errorf("(MID_26053118) inventory items payload missing items: %#v", payload)
	}
	return payload, nil
}

func inventoryItemsExtractionContract() llmclients.StructuredOutputContract {
	return newDocProcessingContract("chenweb_inventory_items_extraction", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"language": schemaString(),
			"items":    schemaArray(),
		},
		"required":             []string{"items"},
		"additionalProperties": true,
	})
}

func normalizeInventoryItemRows(raw any, chunkSeqNo int, dict inventoryDictionary) []map[string]any {
	items, ok := normalizeProductItems(raw)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemName := strings.TrimSpace(asString(m["item_name"]))
		if itemName == "" {
			continue
		}
		categories := normalizeInventoryCategories(m["item_categories"])
		// The first category is the primary one used for the single-schema
		// normalization steps (spec names, required attrs, plausible ranges).
		primaryCategory := categories[0]
		categoryKnown := true
		for _, c := range categories {
			if _, ok := dict.Categories[c]; !ok {
				categoryKnown = false
				break
			}
		}
		rawSpecs := normalizeInventorySpecs(m["raw_specs"], primaryCategory, dict)
		normalizedSpecs := normalizeInventorySpecUnits(rawSpecs, dict)
		missing := missingRequiredInventoryAttrs(m, normalizedSpecs, primaryCategory, dict)
		confidence := toFloat(m["confidence"])
		rangeFlags := validateInventoryPlausibleRanges(normalizedSpecs, primaryCategory, dict)
		flags := buildInventoryValidationFlags(missing, rangeFlags, confidence, normalizeEntityLineSpans(m["lines"]), categoryKnown)
		row := map[string]any{
			"item_name":              itemName,
			"canonical_name":         strings.TrimSpace(asString(m["canonical_name"])),
			"item_categories":        categories,
			"manufacturer":           strings.TrimSpace(asString(m["manufacturer"])),
			"brand":                  strings.TrimSpace(asString(m["brand"])),
			"model_number":           strings.TrimSpace(asString(m["model_number"])),
			"part_number":            strings.TrimSpace(asString(m["part_number"])),
			"normalized_specs":       normalizedSpecs,
			"raw_specs":              rawSpecs,
			"standards":              toStringSlice(m["standards"]),
			"aliases":                toStringSlice(m["aliases"]),
			"evidence_quote":         strings.TrimSpace(asString(m["evidence_quote"])),
			"source_line_spans":      normalizeEntityLineSpans(m["lines"]),
			"validation_flags":       flags,
			"missing_required_attrs": missing,
			"dedupe_key":             buildInventoryDedupeKey(inventoryDedupeCategorySegment(categories), m, normalizedSpecs),
			"schema_version":         inventoryItemsSchemaVersion,
			"dictionary_version":     dict.Version,
			"confidence":             confidence,
			"confidence_reason":      strings.TrimSpace(asString(m["confidence_reason"])),
			"chunk_seq_no":           chunkSeqNo,
		}
		if objects := objectItemsFromValue(m["objects"]); len(objects) > 0 {
			row["objects"] = objects
		}
		if strings.TrimSpace(asString(row["canonical_name"])) == "" {
			row["canonical_name"] = itemName
		}
		out = append(out, row)
	}
	return out
}

func normalizeInventorySpecs(raw any, category string, dict inventoryDictionary) []map[string]any {
	items, ok := normalizeProductItems(raw)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := normalizeInventorySpecName(asString(m["name"]), category, dict)
		if name == "" {
			continue
		}
		out = append(out, map[string]any{
			"name":  name,
			"value": normalizeInventorySpecValue(m["value"]),
			"unit":  strings.TrimSpace(asString(m["unit"])),
		})
	}
	return out
}

func normalizeInventorySpecName(raw string, category string, dict inventoryDictionary) string {
	name := normalizeInventoryToken(raw)
	if name == "" {
		return ""
	}
	if cat, ok := dict.Categories[category]; ok {
		if _, ok := cat.Specs[name]; ok {
			return name
		}
		for canonical, spec := range cat.Specs {
			for _, alias := range spec.Aliases {
				if normalizeInventoryToken(alias) == name {
					return normalizeInventoryToken(canonical)
				}
			}
		}
	}
	return name
}

func normalizeInventorySpecValue(raw any) any {
	switch v := raw.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f
		}
	case string:
		s := strings.TrimSpace(v)
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
		return s
	}
	return raw
}

func normalizeInventorySpecUnits(specs []map[string]any, dict inventoryDictionary) []map[string]any {
	out := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		next := map[string]any{
			"name":  spec["name"],
			"value": spec["value"],
			"unit":  strings.TrimSpace(asString(spec["unit"])),
		}
		unitKey := normalizeInventoryToken(asString(spec["unit"]))
		if rule, ok := dict.Units[unitKey]; ok {
			next["unit"] = rule.Canonical
			if value, ok := numericSpecValue(spec["value"]); ok && rule.Factor != 0 {
				next["value"] = normalizeFloat(value * rule.Factor)
			}
		}
		out = append(out, next)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.TrimSpace(asString(out[i]["name"])) < strings.TrimSpace(asString(out[j]["name"]))
	})
	return out
}

func numericSpecValue(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

func normalizeFloat(v float64) float64 {
	if math.Abs(v-math.Round(v)) < 0.0000001 {
		return math.Round(v)
	}
	return math.Round(v*1000000) / 1000000
}

func missingRequiredInventoryAttrs(row map[string]any, specs []map[string]any, category string, dict inventoryDictionary) []string {
	cat, ok := dict.Categories[category]
	if !ok {
		return nil
	}
	specNames := map[string]struct{}{}
	for _, spec := range specs {
		if name := normalizeInventoryToken(asString(spec["name"])); name != "" {
			specNames[name] = struct{}{}
		}
	}
	missing := make([]string, 0, len(cat.RequiredAttrs))
	for _, raw := range cat.RequiredAttrs {
		attr := normalizeInventoryToken(raw)
		if attr == "" {
			continue
		}
		if _, ok := specNames[attr]; ok {
			continue
		}
		if strings.TrimSpace(asString(row[attr])) != "" {
			continue
		}
		missing = append(missing, attr)
	}
	return missing
}

func validateInventoryPlausibleRanges(specs []map[string]any, category string, dict inventoryDictionary) []string {
	catRanges, ok := dict.PlausibleRanges[category]
	if !ok {
		return nil
	}
	flags := make([]string, 0, 1)
	for _, spec := range specs {
		name := normalizeInventoryToken(asString(spec["name"]))
		r, ok := catRanges[name]
		if !ok {
			continue
		}
		value, ok := numericSpecValue(spec["value"])
		if !ok {
			continue
		}
		unit := strings.TrimSpace(asString(spec["unit"]))
		if expected := strings.TrimSpace(r.Unit); expected != "" && unit != expected {
			continue
		}
		if r.Min != nil && value < *r.Min {
			flags = append(flags, "implausible_"+name)
			continue
		}
		if r.Max != nil && value > *r.Max {
			flags = append(flags, "implausible_"+name)
		}
	}
	return uniqueSortedStrings(flags)
}

func buildInventoryValidationFlags(missing []string, rangeFlags []string, confidence float64, spans []string, categoryKnown bool) []string {
	flags := make([]string, 0, 4)
	if !categoryKnown {
		flags = append(flags, "unknown_category")
	}
	if len(missing) > 0 {
		flags = append(flags, "missing_required_attrs")
	}
	flags = append(flags, rangeFlags...)
	if confidence > 0 && confidence < 0.6 {
		flags = append(flags, "low_confidence")
	}
	if len(spans) == 0 {
		flags = append(flags, "missing_source_lines")
	}
	return flags
}

// normalizeInventoryCategories parses the LLM's item_categories field (an array of
// category surface forms) into a deduplicated, normalized list of category keys. It
// tolerates a scalar string for resilience. The result always has at least one element
// ("unknown") so downstream single-schema steps (specs / required attrs) have a primary
// category to key on.
func normalizeInventoryCategories(raw any) []string {
	vals := toStringSlice(raw)
	if len(vals) == 0 {
		if s := strings.TrimSpace(asString(raw)); s != "" {
			vals = []string{s}
		}
	}
	out := make([]string, 0, len(vals))
	seen := make(map[string]struct{}, len(vals))
	for _, v := range vals {
		c := normalizeInventoryToken(strings.TrimSpace(v))
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	if len(out) == 0 {
		out = append(out, normalizeInventoryToken("unknown"))
	}
	return out
}

// inventoryDedupeCategorySegment builds the deterministic category segment of the dedupe
// key from an item's (possibly multiple) categories: normalized, sorted, comma-joined so
// the segment is stable regardless of the order the model emitted the categories in.
func inventoryDedupeCategorySegment(categories []string) string {
	if len(categories) == 0 {
		return ""
	}
	sorted := append([]string(nil), categories...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

func buildInventoryDedupeKey(category string, row map[string]any, specs []map[string]any) string {
	// The name is part of the identity: two items that differ only in name are
	// distinct items, not duplicates. Without it, every spec-less item in a
	// category (e.g. raw materials with no manufacturer/model/part) would collapse
	// to a single "<category>||||" key and all but the highest-confidence one would
	// be deleted. Overlapping-chunk duplicates share identical source text and thus
	// an identical name, so including the name does not weaken true-duplicate merging.
	name := normalizeInventoryKeyPart(firstNonEmptyTrimmed(asString(row["canonical_name"]), asString(row["item_name"])))
	manufacturer := normalizeInventoryKeyPart(firstNonEmptyTrimmed(asString(row["manufacturer"]), asString(row["brand"])))
	model := normalizeInventoryKeyPart(asString(row["model_number"]))
	part := normalizeInventoryKeyPart(asString(row["part_number"]))
	specParts := make([]string, 0, len(specs))
	for _, spec := range specs {
		name := normalizeInventoryKeyPart(asString(spec["name"]))
		if name == "" {
			continue
		}
		value := formatInventorySpecValue(spec["value"])
		unit := strings.TrimSpace(asString(spec["unit"]))
		specParts = append(specParts, name+"="+value+unit)
	}
	sort.Strings(specParts)
	return strings.Join([]string{
		normalizeInventoryKeyPart(category),
		name,
		manufacturer,
		model,
		part,
		strings.Join(specParts, ","),
	}, "|")
}

func formatInventorySpecValue(raw any) string {
	if f, ok := numericSpecValue(raw); ok {
		if math.Abs(f-math.Round(f)) < 0.0000001 {
			return strconv.FormatInt(int64(math.Round(f)), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return normalizeInventoryKeyPart(asString(raw))
}

func normalizeInventoryToken(raw string) string {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func normalizeInventoryKeyPart(raw string) string {
	return strings.ReplaceAll(normalizeInventoryToken(raw), "_", "")
}

func uniqueSortedStrings(items []string) []string {
	out := uniqueStrings(items)
	sort.Strings(out)
	return out
}

// dedupeInventoryItemRows collapses raw extractions that refer to the same item
// (identical dedupe_key). Within each group it keeps the highest-confidence row as
// the survivor, unions the multi-valued provenance fields (source line spans,
// aliases, standards) from every member into that survivor, and records how many
// raw mentions collapsed into it (ext_info.mention_count). The discarded members
// are returned separately so the caller can persist them for audit / recovery
// instead of dropping them silently.
func dedupeInventoryItemRows(items []map[string]any) (survivors, duplicates []map[string]any) {
	order := make([]string, 0, len(items))
	groups := map[string][]map[string]any{}
	for _, item := range items {
		key := dedupeGroupKey(item)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], item)
	}
	for _, key := range order {
		members := groups[key]
		survivorIdx := 0
		for i := 1; i < len(members); i++ {
			if toFloat(members[i]["confidence"]) > toFloat(members[survivorIdx]["confidence"]) {
				survivorIdx = i
			}
		}
		survivor := members[survivorIdx]
		for i, m := range members {
			if i == survivorIdx {
				continue
			}
			survivor["source_line_spans"] = unionStringList(survivor["source_line_spans"], m["source_line_spans"])
			survivor["aliases"] = unionStringList(survivor["aliases"], m["aliases"])
			survivor["standards"] = unionStringList(survivor["standards"], m["standards"])
			survivor["objects"] = unionObjectItems(survivor["objects"], m["objects"])
			duplicates = append(duplicates, m)
		}
		survivor["mention_count"] = len(members)
		survivors = append(survivors, survivor)
	}
	return survivors, duplicates
}

func unionObjectItems(a, b any) []map[string]any {
	items := append(objectItemsFromValue(a), objectItemsFromValue(b)...)
	if len(items) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		name := firstNonEmptyTrimmed(asString(item["object_name"]), asString(item["object_name_en"]), asString(item["object_name_zh"]))
		role := normalizeObjectToken(asString(item["object_role"]))
		key := normalizeObjectName(name) + "|" + role
		if strings.TrimSpace(key) == "|" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeGroupKey(item map[string]any) string {
	key := strings.TrimSpace(asString(item["dedupe_key"]))
	if key == "" {
		key = strings.TrimSpace(asString(item["item_name"]))
	}
	return key
}

// unionStringList merges two string-list values (each may be []string or []any)
// into a single deduplicated, sorted []string. Used to combine provenance fields
// across collapsed duplicate mentions without losing any value.
func unionStringList(a, b any) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, v := range []any{a, b} {
		for _, s := range toStringList(v) {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func toStringList(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, asString(e))
		}
		return out
	default:
		return nil
	}
}

func loadInventoryDictionaryDir(dir string) (inventoryDictionary, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return inventoryDictionary{}, fmt.Errorf("inventory dictionary dir is empty")
	}
	var catFile struct {
		Version    string                             `json:"version"`
		Categories map[string]inventoryCategorySchema `json:"categories"`
	}
	if err := readInventoryJSON(filepath.Join(dir, "category_schemas.json"), &catFile); err != nil {
		return inventoryDictionary{}, err
	}
	var unitFile struct {
		Version string                       `json:"version"`
		Units   map[string]inventoryUnitRule `json:"units"`
	}
	if err := readInventoryJSON(filepath.Join(dir, "units.json"), &unitFile); err != nil && !os.IsNotExist(err) {
		return inventoryDictionary{}, err
	}
	var aliasFile struct {
		Version string              `json:"version"`
		Aliases map[string][]string `json:"aliases"`
	}
	if err := readInventoryJSON(filepath.Join(dir, "aliases.json"), &aliasFile); err != nil && !os.IsNotExist(err) {
		return inventoryDictionary{}, err
	}
	var standardsFile struct {
		Version   string              `json:"version"`
		Standards map[string][]string `json:"standards"`
	}
	if err := readInventoryJSON(filepath.Join(dir, "standards.json"), &standardsFile); err != nil && !os.IsNotExist(err) {
		return inventoryDictionary{}, err
	}
	var rangeFile struct {
		Version string                                        `json:"version"`
		Ranges  map[string]map[string]InventoryPlausibleRange `json:"ranges"`
	}
	if err := readInventoryJSON(filepath.Join(dir, "plausible_ranges.json"), &rangeFile); err != nil && !os.IsNotExist(err) {
		return inventoryDictionary{}, err
	}
	dict := inventoryDictionary{
		Version:         strings.TrimSpace(catFile.Version),
		Categories:      map[string]inventoryCategorySchema{},
		Units:           map[string]inventoryUnitRule{},
		Aliases:         map[string]string{},
		Standards:       map[string][]string{},
		PlausibleRanges: map[string]map[string]InventoryPlausibleRange{},
	}
	if dict.Version == "" {
		dict.Version = "unknown"
	}
	for category, schema := range catFile.Categories {
		key := normalizeInventoryToken(category)
		if key == "" {
			continue
		}
		specs := map[string]InventorySpecSchema{}
		for specName, spec := range schema.Specs {
			specs[normalizeInventoryToken(specName)] = spec
		}
		schema.Specs = specs
		dict.Categories[key] = schema
	}
	for unit, rule := range unitFile.Units {
		key := normalizeInventoryToken(unit)
		if key == "" {
			continue
		}
		if strings.TrimSpace(rule.Canonical) == "" {
			rule.Canonical = unit
		}
		if rule.Factor == 0 {
			rule.Factor = 1
		}
		dict.Units[key] = rule
	}
	for canonical, aliases := range aliasFile.Aliases {
		canonicalKey := normalizeInventoryToken(canonical)
		if canonicalKey == "" {
			continue
		}
		for _, alias := range aliases {
			aliasKey := normalizeInventoryToken(alias)
			if aliasKey != "" {
				dict.Aliases[aliasKey] = canonicalKey
			}
		}
	}
	for category, values := range standardsFile.Standards {
		categoryKey := normalizeInventoryToken(category)
		if categoryKey == "" {
			continue
		}
		dict.Standards[categoryKey] = uniqueSortedStrings(values)
	}
	for category, specs := range rangeFile.Ranges {
		categoryKey := normalizeInventoryToken(category)
		if categoryKey == "" {
			continue
		}
		dict.PlausibleRanges[categoryKey] = map[string]InventoryPlausibleRange{}
		for specName, specRange := range specs {
			specKey := normalizeInventoryToken(specName)
			if specKey == "" {
				continue
			}
			dict.PlausibleRanges[categoryKey][specKey] = specRange
		}
	}
	return dict, nil
}

func readInventoryJSON(path string, out any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func resolveInventoryDictionaryDir() string {
	if dir := strings.TrimSpace(os.Getenv("INVENTORY_ITEMS_DICTIONARY_DIR")); dir != "" {
		return dir
	}
	candidates := []string{
		"config/inventory_items",
		"../../../config/inventory_items",
		"../../../../config/inventory_items",
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}
	return "config/inventory_items"
}

func writeInventoryItemsArtifactFile(artifactDir string, recordID int64, rec DocMetadataInputRecord, rows []map[string]any) error {
	return writeEntityRelationArtifactFile(artifactDir, recordID, rec, rows, ".inventory_items", "MID_26053140")
}

func refreshInventoryItemsArtifactFile(ctx context.Context, db *sql.DB, artifactDir string, recordID int64, rec DocMetadataInputRecord) error {
	rows, err := loadPersistedInventoryItemArtifactRows(ctx, db, recordID)
	if err != nil {
		return err
	}
	return writeInventoryItemsArtifactFile(artifactDir, recordID, rec, rows)
}

func loadPersistedInventoryItemArtifactRows(ctx context.Context, db *sql.DB, recordID int64) ([]map[string]any, error) {
	const q = `
SELECT inventory_item_id, item_name, canonical_name, item_categories, manufacturer, brand,
       model_number, part_number, normalized_specs, raw_specs, standards, aliases,
       evidence_quote, source_line_spans, validation_flags, missing_required_attrs,
       dedupe_key, schema_version, dictionary_version, confidence, confidence_reason,
       kb.connected_artifacts(input_record_id, 'inventory_item', id), ext_info
FROM kb.inventory_items
WHERE input_record_id = $1
ORDER BY id`
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]map[string]any, 0, 16)
	for rows.Next() {
		var (
			inventoryItemID      string
			itemName             string
			canonicalName        string
			itemCategories       []byte
			manufacturer         string
			brand                string
			modelNumber          string
			partNumber           string
			normalizedSpecs      []byte
			rawSpecs             []byte
			standards            []byte
			aliases              []byte
			evidenceQuote        string
			sourceLineSpans      []byte
			validationFlags      []byte
			missingRequiredAttrs []byte
			dedupeKey            string
			schemaVersion        string
			dictionaryVersion    string
			confidence           float64
			confidenceReason     string
			connectedArtifacts   []byte
			extInfo              []byte
		)
		if err := rows.Scan(
			&inventoryItemID, &itemName, &canonicalName, &itemCategories, &manufacturer, &brand,
			&modelNumber, &partNumber, &normalizedSpecs, &rawSpecs, &standards, &aliases,
			&evidenceQuote, &sourceLineSpans, &validationFlags, &missingRequiredAttrs,
			&dedupeKey, &schemaVersion, &dictionaryVersion, &confidence, &confidenceReason,
			&connectedArtifacts, &extInfo,
		); err != nil {
			return nil, err
		}

		row := map[string]any{
			"inventory_item_id":      strings.TrimSpace(inventoryItemID),
			"item_name":              strings.TrimSpace(itemName),
			"canonical_name":         strings.TrimSpace(canonicalName),
			"item_categories":        jsonColumnToValue(itemCategories, []any{}),
			"manufacturer":           strings.TrimSpace(manufacturer),
			"brand":                  strings.TrimSpace(brand),
			"model_number":           strings.TrimSpace(modelNumber),
			"part_number":            strings.TrimSpace(partNumber),
			"normalized_specs":       jsonColumnToValue(normalizedSpecs, []any{}),
			"raw_specs":              jsonColumnToValue(rawSpecs, []any{}),
			"standards":              jsonColumnToValue(standards, []any{}),
			"aliases":                jsonColumnToValue(aliases, []any{}),
			"evidence_quote":         strings.TrimSpace(evidenceQuote),
			"source_line_spans":      jsonColumnToValue(sourceLineSpans, []any{}),
			"validation_flags":       jsonColumnToValue(validationFlags, []any{}),
			"missing_required_attrs": jsonColumnToValue(missingRequiredAttrs, []any{}),
			"dedupe_key":             strings.TrimSpace(dedupeKey),
			"schema_version":         strings.TrimSpace(schemaVersion),
			"dictionary_version":     strings.TrimSpace(dictionaryVersion),
			"confidence":             confidence,
			"confidence_reason":      strings.TrimSpace(confidenceReason),
			"connected_artifacts":    jsonColumnToValue(connectedArtifacts, map[string]any{}),
		}

		if ext := jsonColumnObject(extInfo); ext != nil {
			if chunkSeqNo, ok := ext["chunk_seq_no"]; ok {
				row["chunk_seq_no"] = chunkSeqNo
			}
			if mentionCount, ok := ext["mention_count"]; ok {
				row["mention_count"] = mentionCount
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func jsonColumnToValue(raw []byte, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil || v == nil {
		return fallback
	}
	return v
}

func jsonColumnObject(raw []byte) map[string]any {
	v, ok := jsonColumnToValue(raw, map[string]any{}).(map[string]any)
	if !ok || v == nil {
		return nil
	}
	return v
}

type inventoryItemsStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	Progress      string
	ProcErr       error
}

func appendInventoryItemsStatus(raw string, p inventoryItemsStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_inventory_items",
		"input_filename": strings.TrimSpace(p.InputFilename),
		"start_time":     p.Start.Format(defaultDocMetaStatusTime),
		"ms_used":        p.DurationMs,
	}
	if progress := strings.TrimSpace(p.Progress); progress != "" {
		entry["progress"] = progress
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
		if strings.ToLower(strings.TrimSpace(asString(e["operation"]))) != "extract_inventory_items" {
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

func detectInventoryItemsFileType(rec DocMetadataInputRecord) string {
	return detectEntityRelationFileType(rec)
}

func (p *InventoryItemsProcessor) persistInventoryItemsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	if updateErr := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendInventoryItemsStatus(current, inventoryItemsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectInventoryItemsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcErr:       procErr,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	}); updateErr != nil {
		p.Logger.Error("failed persisting inventory items status", "record_id", rec.ID, "error", updateErr)
	}
}

func (p *InventoryItemsProcessor) persistInventoryItemsInProgressStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, progress string) {
	if err := updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendInventoryItemsStatus(current, inventoryItemsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectInventoryItemsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "in_progress",
			Progress:      progress,
		})
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); err != nil {
		p.Logger.Warn("failed persisting inventory items in-progress status", "record_id", rec.ID, "error", err)
	}
}

func (p *InventoryItemsProcessor) stopAndPersistInventoryItems(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	_ = updateInputStatusAtomic(ctx, p.InputStore, rec.ID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendInventoryItemsStatus(current, inventoryItemsStatusParams{
			RecordID:      rec.ID,
			FileType:      detectInventoryItemsFileType(rec),
			InputFilename: strings.TrimSpace(rec.ResultFilename),
			Start:         start,
			DurationMs:    time.Since(start).Milliseconds(),
			ProcStatus:    "stopped",
		})
		if err != nil {
			p.Logger.Error("failed building inventory items stopped status", "record_id", rec.ID, "error", err)
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	})
}

func (p *InventoryItemsProcessor) logInventoryItemsLLMCall(ctx context.Context, callID string, modelNames []string, payload map[string]any, callErr error, start, end time.Time, recordID int64, chunkIdx, totalChunks, chunkCount, totalCount int) {
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
	if totalChunks > 0 {
		percent = (chunkIdx + 1) * 100 / totalChunks
	}
	progress := fmt.Sprintf("%d%% (%d/%d)", percent, chunkIdx+1, totalChunks)
	extraJSON, _ := json.Marshal(map[string]any{
		"chunk":            chunkIdx + 1,
		"total_chunks":     totalChunks,
		"chunk_item_count": chunkCount,
		"total_item_count": totalCount,
	})
	extraStr := string(extraJSON)
	pass := 1
	cacheHit, cacheMiss := extractorCacheTokens(p.Extractor)
	rec := DocProcLogRecord{
		CallReason:            "extract inventory items",
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            p.PromptRef,
		RecordID:              &recordID,
		ProcProgress:          &progress,
		Pass:                  &pass,
		LLMCallID:             &callID,
		ActivityName:          stringPtr("extract_inventory_items"),
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		ExtraInfoJSON:         &extraStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if err := p.ProcLogger.LogExtractInventoryItems(ctx, rec, "MID-26053141"); err != nil {
		p.Logger.Warn("failed to write inventory items llm log", "call_id", callID, "error", err)
	}
}

func (p *InventoryItemsProcessor) logInventoryItemsSummary(ctx context.Context, start, end time.Time, result inventoryItemsExtractionResult, inserted int64, numChunks int, recordID int64) {
	progress := "100%"
	activity := "extract_inventory_items"
	extraJSON, _ := json.Marshal(map[string]any{
		"total_items":    inserted,
		"llm_call_count": result.LLMCallCount,
		"fallback_count": result.FallbackCount,
		"failed_chunks":  result.FailedChunks,
		"num_chunks":     numChunks,
	})
	extraStr := string(extraJSON)
	rec := DocProcLogRecord{
		CallReason:    "extract inventory items",
		DocProcName:   p.Name(),
		ModelNames:    []string{strings.TrimSpace(p.ModelName)},
		PromptName:    p.PromptRef,
		RecordID:      &recordID,
		ProcProgress:  &progress,
		ActivityName:  &activity,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}
	if err := p.ProcLogger.LogSummary(ctx, "extract_inventory_items_finish", rec, "MID-26053142"); err != nil {
		p.Logger.Warn("failed to write inventory items summary log", "error", err)
	}
}

func stringPtr(v string) *string { return &v }

func (s InventoryItemsSQLStore) ensureTables(ctx context.Context) error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26053150) db is nil")
	}
	const ddl = `
CREATE SCHEMA IF NOT EXISTS kb;
CREATE TABLE IF NOT EXISTS kb.inventory_items (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    inventory_item_id TEXT NOT NULL,
    language TEXT,
    item_name TEXT,
    canonical_name TEXT,
    item_categories JSONB DEFAULT '[]'::jsonb,
    manufacturer TEXT,
    brand TEXT,
    model_number TEXT,
    part_number TEXT,
    normalized_specs JSONB,
    raw_specs JSONB,
    standards JSONB,
    aliases JSONB,
    evidence_quote TEXT,
    source_line_spans JSONB,
    validation_flags JSONB,
    missing_required_attrs JSONB,
    dedupe_key TEXT,
    schema_version TEXT,
    dictionary_version TEXT,
    confidence DOUBLE PRECISION,
    confidence_reason TEXT,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_inventory_items_input_item_idx ON kb.inventory_items(input_record_id, inventory_item_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_items_input_record_id ON kb.inventory_items(input_record_id);
CREATE TABLE IF NOT EXISTS kb.inventory_item_duplicates (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    inventory_item_id TEXT NOT NULL,
    duplicate_of TEXT NOT NULL,
    language TEXT,
    item_name TEXT,
    canonical_name TEXT,
    item_categories JSONB DEFAULT '[]'::jsonb,
    manufacturer TEXT,
    brand TEXT,
    model_number TEXT,
    part_number TEXT,
    normalized_specs JSONB,
    raw_specs JSONB,
    standards JSONB,
    aliases JSONB,
    evidence_quote TEXT,
    source_line_spans JSONB,
    validation_flags JSONB,
    missing_required_attrs JSONB,
    dedupe_key TEXT,
    schema_version TEXT,
    dictionary_version TEXT,
    confidence DOUBLE PRECISION,
    confidence_reason TEXT,
    model_name TEXT,
    prompt_name TEXT,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_inventory_item_duplicates_input_item_idx ON kb.inventory_item_duplicates(input_record_id, inventory_item_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_duplicates_input_record_id ON kb.inventory_item_duplicates(input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_inventory_item_duplicates_duplicate_of ON kb.inventory_item_duplicates(duplicate_of);
`
	_, err := s.DB.ExecContext(ctx, ddl)
	return err
}

func (s InventoryItemsSQLStore) InventoryItemsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureTables(ctx); err != nil {
		return false, err
	}
	var one int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM kb.inventory_items WHERE input_record_id = $1 LIMIT 1`, inputRecordID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s InventoryItemsSQLStore) DeleteInventoryItemsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.inventory_items WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// parseJSONStringArray decodes a JSONB string-array column (e.g. item_categories) into a
// trimmed, non-empty []string. Invalid/empty input yields nil.
func parseJSONStringArray(raw []byte) []string {
	if len(raw) == 0 {
		return nil
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// inventoryCategoriesJSON marshals an item's normalized item_categories list into a JSON
// array string for the JSONB column, defaulting to "[]" so the column is never null.
func inventoryCategoriesJSON(raw any) string {
	bs, err := json.Marshal(raw)
	if err != nil || len(bs) == 0 || string(bs) == "null" {
		return "[]"
	}
	return string(bs)
}

func (s InventoryItemsSQLStore) SaveInventoryItems(ctx context.Context, req SaveInventoryItemsRequest) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	if len(req.Items) == 0 {
		return 0, nil
	}
	const stmt = `
INSERT INTO kb.inventory_items (
    event_id, input_record_id, inventory_item_id, language,
    item_name, canonical_name, item_categories, manufacturer, brand, model_number, part_number,
    normalized_specs, raw_specs, standards, aliases, evidence_quote, source_line_spans,
    validation_flags, missing_required_attrs, dedupe_key, schema_version, dictionary_version,
    confidence, confidence_reason, model_name, prompt_name, ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12::jsonb,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17::jsonb,
    $18::jsonb,$19::jsonb,$20,$21,$22,$23,$24,$25,$26,$27::jsonb
)
ON CONFLICT (input_record_id, inventory_item_id) DO UPDATE SET
    language = EXCLUDED.language,
    item_name = EXCLUDED.item_name,
    canonical_name = EXCLUDED.canonical_name,
    item_categories = EXCLUDED.item_categories,
    manufacturer = EXCLUDED.manufacturer,
    brand = EXCLUDED.brand,
    model_number = EXCLUDED.model_number,
    part_number = EXCLUDED.part_number,
    normalized_specs = EXCLUDED.normalized_specs,
    raw_specs = EXCLUDED.raw_specs,
    standards = EXCLUDED.standards,
    aliases = EXCLUDED.aliases,
    evidence_quote = EXCLUDED.evidence_quote,
    source_line_spans = EXCLUDED.source_line_spans,
    validation_flags = EXCLUDED.validation_flags,
    missing_required_attrs = EXCLUDED.missing_required_attrs,
    dedupe_key = EXCLUDED.dedupe_key,
    schema_version = EXCLUDED.schema_version,
    dictionary_version = EXCLUDED.dictionary_version,
    confidence = EXCLUDED.confidence,
    confidence_reason = EXCLUDED.confidence_reason,
    model_name = EXCLUDED.model_name,
    prompt_name = EXCLUDED.prompt_name,
    ext_info = EXCLUDED.ext_info,
    modify_time = NOW()`

	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}
	var inserted int64
	for _, item := range req.Items {
		normalizedSpecs, _ := json.Marshal(item["normalized_specs"])
		rawSpecs, _ := json.Marshal(item["raw_specs"])
		standards, _ := json.Marshal(item["standards"])
		aliases, _ := json.Marshal(item["aliases"])
		spans, _ := json.Marshal(item["source_line_spans"])
		flags, _ := json.Marshal(item["validation_flags"])
		missing, _ := json.Marshal(item["missing_required_attrs"])
		mentionCount := 1
		if mc := int(toFloat(item["mention_count"])); mc > 0 {
			mentionCount = mc
		}
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": inventoryItemsSchemaVersion,
			"chunk_seq_no":   item["chunk_seq_no"],
			"mention_count":  mentionCount,
		})
		res, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(item["inventory_item_id"])),
			strings.TrimSpace(req.Language),
			strings.TrimSpace(asString(item["item_name"])),
			strings.TrimSpace(asString(item["canonical_name"])),
			inventoryCategoriesJSON(item["item_categories"]),
			strings.TrimSpace(asString(item["manufacturer"])),
			strings.TrimSpace(asString(item["brand"])),
			strings.TrimSpace(asString(item["model_number"])),
			strings.TrimSpace(asString(item["part_number"])),
			string(normalizedSpecs),
			string(rawSpecs),
			string(standards),
			string(aliases),
			strings.TrimSpace(asString(item["evidence_quote"])),
			string(spans),
			string(flags),
			string(missing),
			strings.TrimSpace(asString(item["dedupe_key"])),
			strings.TrimSpace(asString(item["schema_version"])),
			strings.TrimSpace(asString(item["dictionary_version"])),
			toFloat(item["confidence"]),
			strings.TrimSpace(asString(item["confidence_reason"])),
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		affected, _ := res.RowsAffected()
		inserted += affected
	}
	return inserted, nil
}

func (s InventoryItemsSQLStore) DeleteInventoryItemDuplicatesByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.inventory_item_duplicates WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s InventoryItemsSQLStore) SaveInventoryItemDuplicates(ctx context.Context, req SaveInventoryItemDuplicatesRequest) (int64, error) {
	if err := s.ensureTables(ctx); err != nil {
		return 0, err
	}
	if len(req.Items) == 0 {
		return 0, nil
	}
	const stmt = `
INSERT INTO kb.inventory_item_duplicates (
    event_id, input_record_id, inventory_item_id, duplicate_of, language,
    item_name, canonical_name, item_categories, manufacturer, brand, model_number, part_number,
    normalized_specs, raw_specs, standards, aliases, evidence_quote, source_line_spans,
    validation_flags, missing_required_attrs, dedupe_key, schema_version, dictionary_version,
    confidence, confidence_reason, model_name, prompt_name, ext_info
) VALUES (
    $1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16::jsonb,$17,$18::jsonb,
    $19::jsonb,$20::jsonb,$21,$22,$23,$24,$25,$26,$27,$28::jsonb
)
ON CONFLICT (input_record_id, inventory_item_id) DO NOTHING`

	var eventIDVal any
	if id := strings.TrimSpace(req.EventID); id != "" {
		eventIDVal = id
	}
	var inserted int64
	for _, item := range req.Items {
		normalizedSpecs, _ := json.Marshal(item["normalized_specs"])
		rawSpecs, _ := json.Marshal(item["raw_specs"])
		standards, _ := json.Marshal(item["standards"])
		aliases, _ := json.Marshal(item["aliases"])
		spans, _ := json.Marshal(item["source_line_spans"])
		flags, _ := json.Marshal(item["validation_flags"])
		missing, _ := json.Marshal(item["missing_required_attrs"])
		extInfo, _ := json.Marshal(map[string]any{
			"language":       req.Language,
			"schema_version": inventoryItemsSchemaVersion,
			"chunk_seq_no":   item["chunk_seq_no"],
		})
		res, err := s.DB.ExecContext(ctx, stmt,
			eventIDVal,
			req.InputRecordID,
			strings.TrimSpace(asString(item["inventory_item_id"])),
			strings.TrimSpace(asString(item["duplicate_of"])),
			strings.TrimSpace(req.Language),
			strings.TrimSpace(asString(item["item_name"])),
			strings.TrimSpace(asString(item["canonical_name"])),
			inventoryCategoriesJSON(item["item_categories"]),
			strings.TrimSpace(asString(item["manufacturer"])),
			strings.TrimSpace(asString(item["brand"])),
			strings.TrimSpace(asString(item["model_number"])),
			strings.TrimSpace(asString(item["part_number"])),
			string(normalizedSpecs),
			string(rawSpecs),
			string(standards),
			string(aliases),
			strings.TrimSpace(asString(item["evidence_quote"])),
			string(spans),
			string(flags),
			string(missing),
			strings.TrimSpace(asString(item["dedupe_key"])),
			strings.TrimSpace(asString(item["schema_version"])),
			strings.TrimSpace(asString(item["dictionary_version"])),
			toFloat(item["confidence"]),
			strings.TrimSpace(asString(item["confidence_reason"])),
			strings.TrimSpace(req.ModelName),
			strings.TrimSpace(req.PromptName),
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		affected, _ := res.RowsAffected()
		inserted += affected
	}
	return inserted, nil
}

func ReindexInventoryItemSearchForRecord(ctx context.Context, recordID int64, logger ApiTypes.JimoLogger) error {
	db := ApiTypes.ProjectDBHandle
	if db == nil {
		return fmt.Errorf("project db handle is nil")
	}
	sourceTitle, err := fetchSearchSourceTitle(ctx, db, recordID)
	if err != nil {
		return err
	}
	rows, err := buildInventoryItemRegistryRows(ctx, db, recordID, sourceTitle)
	if err != nil {
		return err
	}
	return replaceRegistryRows(ctx, db, searchArtifactInventoryItem, recordID, rows,
		"embed_inventory_items", "MID-20260708-12", logger)
}

func buildInventoryItemRegistryRows(ctx context.Context, db *sql.DB, recordID int64, sourceTitle string) ([]kbsearch.RegistryRow, error) {
	const q = `
SELECT id, inventory_item_id, item_name, canonical_name, item_categories, manufacturer, brand,
       model_number, part_number, normalized_specs, standards, aliases, source_line_spans,
       validation_flags, missing_required_attrs, dedupe_key, confidence, confidence_reason,
       search_document
FROM kb.inventory_items
WHERE input_record_id = $1
ORDER BY id`
	// Category review status is derived from the registry at read time (not frozen
	// on instance rows) so approving a category later never requires rewriting
	// millions of item rows. Best-effort: on failure status falls back gracefully.
	categoryStatuses, statusErr := artifactCategoryRegistry{DB: db}.categoryStatuses(ctx, searchArtifactInventoryItem)
	if statusErr != nil {
		categoryStatuses = nil
	}
	rows, err := db.QueryContext(ctx, q, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	weights := appconfig.GetInventoryItemSearchWeightsConfig()
	out := make([]kbsearch.RegistryRow, 0, 16)
	excludeNamesByArtifactID := map[string][]string{}
	type pendingInventoryRegistryRow struct {
		row             kbsearch.RegistryRow
		inventoryItemID string
		baseSearchDoc   string
	}
	pendingRows := make([]pendingInventoryRegistryRow, 0, 16)
	for rows.Next() {
		var (
			id                   int64
			inventoryItemID      string
			itemName             string
			canonicalName        string
			itemCategories       []byte
			manufacturer         string
			brand                string
			modelNumber          string
			partNumber           string
			normalizedSpecs      []byte
			standards            []byte
			aliases              []byte
			sourceLineSpans      []byte
			validationFlags      []byte
			missingRequiredAttrs []byte
			dedupeKey            string
			confidence           float64
			confidenceReason     string
			searchDoc            string
		)
		if err := rows.Scan(&id, &inventoryItemID, &itemName, &canonicalName, &itemCategories, &manufacturer, &brand, &modelNumber, &partNumber, &normalizedSpecs, &standards, &aliases, &sourceLineSpans, &validationFlags, &missingRequiredAttrs, &dedupeKey, &confidence, &confidenceReason, &searchDoc); err != nil {
			return nil, err
		}
		categoryList := parseJSONStringArray(itemCategories)
		primaryCategory := ""
		if len(categoryList) > 0 {
			primaryCategory = categoryList[0]
		}
		categoryStatus := deriveInventoryCategoriesStatus(categoryStatuses, categoryList)
		validationStatus := "valid"
		if len(jsonArrayOrEmptyBytes(validationFlags)) > 2 || categoryStatus != "approved" {
			validationStatus = "needs_review"
		}
		weightedSearchDoc := buildInventoryItemSearchDocument(weights, inventoryItemSearchFields{
			ItemName:         itemName,
			CanonicalName:    canonicalName,
			ItemCategories:   categoryList,
			Manufacturer:     manufacturer,
			Brand:            brand,
			ModelNumber:      modelNumber,
			PartNumber:       partNumber,
			Aliases:          parseJSONStringArray(aliases),
			Standards:        flattenSearchJSONTerms(standards),
			NormalizedSpecs:  flattenSearchJSONTerms(normalizedSpecs),
			DedupeKey:        dedupeKey,
			ConfidenceReason: confidenceReason,
		})
		payload, _ := json.Marshal(map[string]any{
			"item_categories":        json.RawMessage(jsonArrayOrEmptyBytes(itemCategories)),
			"category_status":        categoryStatus,
			"manufacturer":           manufacturer,
			"brand":                  brand,
			"model_number":           modelNumber,
			"part_number":            partNumber,
			"normalized_specs":       json.RawMessage(jsonArrayOrEmptyBytes(normalizedSpecs)),
			"standards":              json.RawMessage(jsonArrayOrEmptyBytes(standards)),
			"aliases":                json.RawMessage(jsonArrayOrEmptyBytes(aliases)),
			"validation_flags":       json.RawMessage(jsonArrayOrEmptyBytes(validationFlags)),
			"missing_required_attrs": json.RawMessage(jsonArrayOrEmptyBytes(missingRequiredAttrs)),
			"validation_status":      validationStatus,
			"dedupe_key":             dedupeKey,
			"confidence":             confidence,
			"confidence_reason":      confidenceReason,
		})
		seq := lastDelimitedToken(inventoryItemID)
		if seq == "" {
			seq = strconv.FormatInt(id, 10)
		}
		artifactID := kbsearch.BuildArtifactID(recordID, searchArtifactInventoryItem, seq)
		excludeNamesByArtifactID[strings.TrimSpace(inventoryItemID)] = []string{itemName, canonicalName}
		baseSearchDoc := firstNonEmpty(weightedSearchDoc, searchDoc, strings.TrimSpace(strings.Join(append([]string{itemName, canonicalName}, append(categoryList, manufacturer, brand, modelNumber, partNumber, dedupeKey)...), " ")))
		pendingRows = append(pendingRows, pendingInventoryRegistryRow{
			inventoryItemID: strings.TrimSpace(inventoryItemID),
			baseSearchDoc:   baseSearchDoc,
			row: kbsearch.RegistryRow{
				ArtifactType:    searchArtifactInventoryItem,
				ArtifactID:      artifactID,
				InputRecordID:   recordID,
				SourceRowID:     &id,
				PrimaryLabel:    firstNonEmpty(itemName, canonicalName, inventoryItemID),
				SecondaryLabel:  firstNonEmpty(primaryCategory, manufacturer),
				SearchDocument:  baseSearchDoc,
				SnippetBasis:    firstNonEmpty(confidenceReason, itemName),
				SourceTitle:     sourceTitle,
				SourceFilename:  sourceTitle,
				CategoryPaths:   json.RawMessage("[]"),
				SourceLineSpans: json.RawMessage(jsonArrayOrEmptyBytes(sourceLineSpans)),
				SemanticPayload: payload,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	objectTextByID, objectTextErr := artifactObjectSearchTextExcluding(ctx, db, recordID, searchArtifactInventoryItem, excludeNamesByArtifactID)
	if objectTextErr != nil {
		objectTextByID = nil
	}
	for _, pending := range pendingRows {
		row := pending.row
		row.SearchDocument = joinUniqueSearchParts(pending.baseSearchDoc, objectTextByID[pending.inventoryItemID])
		out = append(out, row)
	}
	return out, nil
}

type inventoryItemSearchFields struct {
	ItemName         string
	CanonicalName    string
	ItemCategories   []string
	Manufacturer     string
	Brand            string
	ModelNumber      string
	PartNumber       string
	Aliases          []string
	Standards        []string
	NormalizedSpecs  []string
	DedupeKey        string
	ConfidenceReason string
}

func buildInventoryItemSearchDocument(weights appconfig.InventoryItemSearchWeightsConfig, fields inventoryItemSearchFields) string {
	parts := make([]string, 0, 16)
	parts = appendWeightedText(parts, fields.ItemName, weightToSearchRepeats(weights.ItemName))
	parts = appendWeightedText(parts, fields.CanonicalName, weightToSearchRepeats(weights.CanonicalName))
	parts = appendWeightedText(parts, strings.Join(fields.ItemCategories, " "), weightToSearchRepeats(weights.ItemCategories))
	parts = appendWeightedText(parts, fields.Manufacturer, weightToSearchRepeats(weights.Manufacturer))
	parts = appendWeightedText(parts, fields.Brand, weightToSearchRepeats(weights.Brand))
	parts = appendWeightedText(parts, fields.ModelNumber, weightToSearchRepeats(weights.ModelNumber))
	parts = appendWeightedText(parts, fields.PartNumber, weightToSearchRepeats(weights.PartNumber))
	parts = appendWeightedText(parts, strings.Join(fields.Aliases, " "), weightToSearchRepeats(weights.Aliases))
	parts = appendWeightedText(parts, strings.Join(fields.Standards, " "), weightToSearchRepeats(weights.Standards))
	parts = appendWeightedText(parts, strings.Join(fields.NormalizedSpecs, " "), weightToSearchRepeats(weights.NormalizedSpecs))
	parts = appendWeightedText(parts, fields.DedupeKey, weightToSearchRepeats(weights.DedupeKey))
	parts = appendWeightedText(parts, fields.ConfidenceReason, weightToSearchRepeats(weights.ConfidenceReason))
	return strings.TrimSpace(strings.Join(parts, " "))
}

func jsonArrayOrEmptyBytes(raw []byte) []byte {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []byte("[]")
	}
	return []byte(trimmed)
}

// ---- ChunkBatchProcessor implementation ----

func (p *InventoryItemsProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26062730) %s prompt error: %w", p.Name(), p.PromptErr)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("%s skipped: model config error", p.Name(), "record_id", recordID, "error", p.ModelErr)
		return nil
	}
	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchResults = make([]inventoryChunkOutcome, 0, len(chunks))
	return nil
}

func (p *InventoryItemsProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062731) %s chunk index %d out of range", p.Name(), chunkIdx)
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}

	chunk := p.batchChunks[chunkIdx]
	localStart := p.Now()

	p.Logger.Info("extract inventory items start",
		"record_id", p.batchRecordID,
		"chunk", chunkIdx,
	)

	inputText := canonicalChunkInputText(chunk.Lines, p.batchDocCtx)
	callID := fmt.Sprintf("%d_batch_c%d", p.batchRecordID, chunkIdx)
	payload, modelName, err := p.extractInventoryItemsWithFallback(ctx, inputText)
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	wasFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.ModelName) && strings.TrimSpace(modelName) != ""
	var chunkItems []map[string]any
	var chunkLang string
	if err == nil && payload != nil {
		if lang := strings.TrimSpace(asString(payload["language"])); lang != "" {
			chunkLang = lang
		}
		chunkItems = normalizeInventoryItemRows(payload["items"], chunk.SeqNo, p.Dictionary)
	}
	p.logInventoryItemsLLMCall(ctx, callID, []string{strings.TrimSpace(modelName)}, payload, err, localStart, p.Now(), p.batchRecordID, chunkIdx, len(p.batchChunks), len(chunkItems), 0)
	if err != nil {
		p.Logger.Warn("inventory item batch: chunk failed", "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		p.batchMu.Lock()
		p.batchResults = append(p.batchResults, inventoryChunkOutcome{failed: true, fallback: wasFallback})
		p.batchMu.Unlock()
		return nil
	}

	cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
	p.Logger.Info("extract inventory items end  ",
		"record_id", p.batchRecordID,
		"chunk", chunkIdx,
		"extracted", len(chunkItems),
		"ms_used", time.Since(localStart).Milliseconds(),
		"cache_hit", cacheHit,
		"cache_miss", cacheMiss,
	)
	p.batchMu.Lock()
	p.batchResults = append(p.batchResults, inventoryChunkOutcome{
		items:     chunkItems,
		modelName: modelName,
		language:  chunkLang,
		fallback:  wasFallback,
	})
	p.batchMu.Unlock()
	return nil
}

func (p *InventoryItemsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if len(p.batchResults) == 0 {
		p.Logger.Info("inventory items batch: no results", "record_id", p.batchRecordID)
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}

	items := make([]map[string]any, 0)
	detectedLanguage := "unknown"
	usedModel := strings.TrimSpace(p.ModelName)
	var llmCallCount, fallbackCount, failedChunks int
	for _, outcome := range p.batchResults {
		llmCallCount++
		if outcome.fallback {
			fallbackCount++
		}
		if outcome.failed {
			failedChunks++
			continue
		}
		if outcome.language != "" && detectedLanguage == "unknown" {
			detectedLanguage = outcome.language
		}
		if strings.TrimSpace(outcome.modelName) != "" {
			usedModel = strings.TrimSpace(outcome.modelName)
		}
		items = append(items, outcome.items...)
	}

	if detectedLanguage == "" {
		detectedLanguage = "unknown"
	}
	for i := range items {
		if strings.TrimSpace(asString(items[i]["inventory_item_id"])) == "" {
			items[i]["inventory_item_id"] = fmt.Sprintf("%d_inv_%d", p.batchRecordID, i+1)
		}
	}

	eventID := fmt.Sprintf("batch_%d", p.batchRecordID)
	inserted, saveErr := p.Store.SaveInventoryItems(ctx, SaveInventoryItemsRequest{
		InputRecordID: p.batchRecordID,
		EventID:       eventID,
		Language:      detectedLanguage,
		ModelName:     firstNonEmptyTrimmed(usedModel, p.ModelName),
		PromptName:    p.PromptRef,
		Items:         items,
	})
	if saveErr != nil {
		return fmt.Errorf("(MID_26062733) %s save items: %w", p.Name(), saveErr)
	}
	if err := p.persistInventoryItemObjects(ctx, p.batchRecordID, items); err != nil {
		return fmt.Errorf("(MID_26062734) %s persist inventory item objects: %w", p.Name(), err)
	}

	if reindexErr := ReindexInventoryItemSearchForRecord(ctx, p.batchRecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex inventory search failed", "record_id", p.batchRecordID, "error", reindexErr)
	}

	p.Logger.Info("inventory items batch complete",
		"record_id", p.batchRecordID,
		"inserted", inserted,
		"num_chunks", len(p.batchChunks),
	)
	return nil
}

func (p *InventoryItemsProcessor) persistInventoryItemObjects(ctx context.Context, recordID int64, items []map[string]any) error {
	if p.ObjectStore == nil {
		return nil
	}
	objects := make([]ArtifactObject, 0)
	for _, item := range items {
		itemID := strings.TrimSpace(asString(item["inventory_item_id"]))
		if itemID == "" {
			continue
		}
		objects = append(objects, normalizeArtifactObjectsForArtifact(recordID, searchArtifactInventoryItem, itemID, item)...)
	}
	if p.ObjectReconciler.Store != nil && len(objects) > 0 {
		reconciled, err := reconcileArtifactObjectsWithLLM(ctx, objects, p.ObjectReconciler, p.Logger, p.AmbiguousObjectLLMResolver, p.ResolveAmbiguousMinConfidence, objectReconcileLogSink{ProcLogger: p.ProcLogger, DocProcName: p.Name(), CallReason: p.Name()})
		if err != nil {
			return err
		}
		objects = reconciled
	}
	return p.ObjectStore.ReplaceObjectsForRecord(ctx, recordID, searchArtifactInventoryItem, objects)
}
