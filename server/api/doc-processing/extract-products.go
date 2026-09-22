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
	"sync"
	"time"
	"unicode"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type ProductsProcessor struct {
	InputStore           DocMetadataStore
	Store                ProductsStore
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
	ModelCfg             structureModelConfig
	MentionPromptText    string
	MentionPromptRef     string
	MentionPromptPath    string
	MentionPromptErr     error
	MentionModelRef      string
	MentionModelCfgPath  string
	MentionModelErr      error
	MentionModelName     string
	MentionModelCfg      structureModelConfig
	RelationPromptText   string
	RelationPromptRef    string
	RelationPromptPath   string
	RelationPromptErr    error
	RelationModelRef     string
	RelationModelCfgPath string
	RelationModelErr     error
	RelationModelName    string
	RelationModelCfg     structureModelConfig
	TranslatePromptText  string
	TranslatePromptRef   string
	TranslatePromptPath  string
	TranslatePromptErr   error
	TranslateModelRef    string
	TranslateModelName   string
	TranslateModelCfg    structureModelConfig
	TranslateEnabled     bool
	TranslateBatchSize   int
	FallbackModelRef     string
	FallbackModelCfgPath string
	FallbackModelErr     error
	FallbackModelName    string
	FallbackModelCfg     structureModelConfig
	ArtifactDir          string
	ArtifactWebDir       string
	ProductNames         ProductNameStore
	MaxTasks             int
	Pass1Only            bool

	// batch* fields hold per-run state for the ChunkBatchProcessor path
	// (InitChunkBatch/ProcessChunk/FinalizeChunkBatch), set up by
	// InitChunkBatch and read/written by ProcessChunk (under batchMu, since
	// the coordinator calls it concurrently across chunks) and
	// FinalizeChunkBatch. Unused by the HandleEvent path.
	batchMu            sync.Mutex
	batchRecordID      int64
	batchDocCtx        string
	batchChunks        []Chunk
	batchBlocks        []Block
	batchStart         time.Time
	batchSkip          bool
	batchForce         bool
	batchMentions      []productMention
	batchFallbackCount int
	batchMentionModel  string
}

type ProductsStore interface {
	ProductsExist(ctx context.Context, inputRecordID int64) (bool, error)
	DeleteProductsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error)
	SaveProducts(ctx context.Context, req SaveProductsRequest) (int64, error)
}

type ProductsSQLStore struct {
	DB *sql.DB
}

type SaveProductsRequest struct {
	InputRecordID int64
	Products      []map[string]any
}

type productExtractionResult struct {
	Products      []map[string]any
	ModelName     string
	LLMCallCount  int
	FallbackCount int
	MentionsCount int
}

type productMention struct {
	MentionText       string
	CanonicalHint     string
	ProductTypeHint   string
	EvidenceQuote     string
	EvidenceLines     []string
	IsExplicit        bool
	Confidence        float64
	ConfidenceReason  string
	BlockIndex        int
	BlockLines        []BlockLine
	HasNormalEvidence bool
}

type productCandidate struct {
	CandidateID        string
	ProductName        string
	CanonicalName      string
	ProductTypeHint    string
	SupportingMentions []map[string]any
	SupportLines       []BlockLine
	// BlockIndex is the shared Block.Index of every mention merged into this
	// candidate, or -1 when the candidate's mentions span more than one
	// block. Only a single-block candidate can reuse that block's chunk as
	// its enrichment document (see extractProductsFromBlocksWithLLM).
	BlockIndex int
}

func NewProductsProcessor(inputStore DocMetadataStore, store ProductsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *ProductsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26052001")
	}
	mentionPromptText, mentionPromptRef, mentionPromptPath, mentionPromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_PRODUCT_MENTIONS_PROMPT"},
		"prompt-extract-product-mentions-v2.md",
	)
	relationPromptText, relationPromptRef, relationPromptPath, relationPromptErr := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_PRODUCT_MENTION_PROMPT"},
		"prompt-enrich-product-mention-v4.md",
	)
	mentionModelRef, mentionModelCfgPath, mentionModelCfg, mentionModelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_PRODUCT_MENTIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	relationModelRef, relationModelCfgPath, relationModelCfg, relationModelErr := loadModelConfigFromEnvKeys(
		[]string{"ENRICH_PRODUCT_RELATIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	translatePromptText, translatePromptRef, translatePromptPath, translatePromptErr := loadProductPromptFromEnvKeys(
		[]string{"TRANSLATE_PRODUCTS_PROMPT"},
		"prompt-translate-products-v2.md",
	)
	translateModelRef, _, translateModelCfg, translateModelErr := loadOptionalModelConfigFromEnvKeys(
		[]string{"TRANSLATION_MODEL_NAME", "ENRICH_PRODUCT_RELATIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	// Pass 3a is mechanical: translate five short fields per row, no
	// reasoning required. On a hybrid-reasoning model that thinks by default
	// it was spending ~78% of its output tokens on discarded reasoning_content
	// (measured 2026-09-22 over 14 days of `product translation` calls).
	// Disabling thinking cut output tokens 81.6% across a 6-call replay with
	// every batch's row count preserved and only wording-level differences in
	// the translations. Contrast Pass 1, where the same change cost ~20% of
	// distinct mentions -- so this is scoped to translation, not applied
	// processor-wide. Override with TRANSLATE_PRODUCTS_THINKING (set it to ""
	// for endpoints that reject the `thinking` field, e.g. local
	// llama.cpp/ollama hosts).
	translateModelCfg.ThinkingType = normalizeThinkingType(envString("TRANSLATE_PRODUCTS_THINKING", "disabled"))
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_PRODUCT_MODEL_FALLBACK", "MODEL_DEF_FILE")
	applyStructureModelConfigToExtractor(extractor, relationModelCfg)
	translateEnabled := translatePromptErr == nil && strings.TrimSpace(translatePromptText) != "" && translateModelErr == nil && strings.TrimSpace(translateModelCfg.ModelName) != ""
	return &ProductsProcessor{
		InputStore:           inputStore,
		Store:                store,
		Extractor:            extractor,
		Logger:               logger,
		ProcLogger:           DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:                  time.Now,
		PromptText:           relationPromptText,
		PromptRef:            relationPromptRef,
		PromptPath:           relationPromptPath,
		PromptErr:            relationPromptErr,
		ModelRef:             relationModelRef,
		ModelCfgPath:         relationModelCfgPath,
		ModelErr:             relationModelErr,
		ModelName:            relationModelCfg.ModelName,
		ModelCfg:             relationModelCfg,
		MentionPromptText:    mentionPromptText,
		MentionPromptRef:     mentionPromptRef,
		MentionPromptPath:    mentionPromptPath,
		MentionPromptErr:     mentionPromptErr,
		MentionModelRef:      mentionModelRef,
		MentionModelCfgPath:  mentionModelCfgPath,
		MentionModelErr:      mentionModelErr,
		MentionModelName:     mentionModelCfg.ModelName,
		MentionModelCfg:      mentionModelCfg,
		RelationPromptText:   relationPromptText,
		RelationPromptRef:    relationPromptRef,
		RelationPromptPath:   relationPromptPath,
		RelationPromptErr:    relationPromptErr,
		RelationModelRef:     relationModelRef,
		RelationModelCfgPath: relationModelCfgPath,
		RelationModelErr:     relationModelErr,
		RelationModelName:    relationModelCfg.ModelName,
		RelationModelCfg:     relationModelCfg,
		TranslatePromptText:  translatePromptText,
		TranslatePromptRef:   translatePromptRef,
		TranslatePromptPath:  translatePromptPath,
		TranslatePromptErr:   translatePromptErr,
		TranslateModelRef:    translateModelRef,
		TranslateModelName:   translateModelCfg.ModelName,
		TranslateModelCfg:    translateModelCfg,
		TranslateEnabled:     translateEnabled,
		TranslateBatchSize:   envInt("TRANSLATE_PRODUCTS_BATCH_SIZE", defaultTranslateBatchSize, 1),
		FallbackModelRef:     fallbackModelRef,
		FallbackModelCfgPath: fallbackModelCfgPath,
		FallbackModelErr:     fallbackModelErr,
		FallbackModelName:    fallbackModelCfg.ModelName,
		FallbackModelCfg:     fallbackModelCfg,
		ArtifactDir:          strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:       strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
		ProductNames:         ProductNameSQLStore{DB: ApiTypes.ProjectDBHandle},
		MaxTasks:             envInt("EXTRACT_PRODUCTS_MAX_TASKS", 1, 1),
		Pass1Only:            ExtractProductPass1OnlyFromEnv(),
	}
}

// ExtractProductPass1OnlyFromEnv resolves the EXTRACT_PRODUCT_PASS_1_ONLY
// setting (default: false). Unset (or any value that does not parse as a
// boolean) resolves to false. When true, the processor runs only Pass 1
// (mention extraction) and Deterministic Step A (merge/dedup), persisting the
// deduplicated candidates straight to kb.products and skipping Pass 2/3
// entirely -- see "Testing: Pass-1-Only Mode" in extract-products-spec.md.
// Testing only; never enable in production.
func ExtractProductPass1OnlyFromEnv() bool {
	raw := strings.TrimSpace(os.Getenv("EXTRACT_PRODUCT_PASS_1_ONLY"))
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}
	return enabled
}

func (p *ProductsProcessor) Name() string { return "extract_products" }

func (p *ProductsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26052002) parse event payload: %w", err)
	}
	ctx = withLLMRecordID(ctx, evt.RecordID)
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.MentionPromptErr != nil {
		return fmt.Errorf("(MID_26052003) load product mentions prompt %q: %w", p.MentionPromptRef, p.MentionPromptErr)
	}
	if p.RelationPromptErr != nil {
		return fmt.Errorf("(MID_26052003) load product relations prompt %q: %w", p.RelationPromptRef, p.RelationPromptErr)
	}
	if p.InputStore == nil {
		return errors.New("(MID_26052004) input store is nil")
	}
	if p.Store == nil {
		return errors.New("(MID_26052005) products store is nil")
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("(MID_26052006) load kb.inputs record %d: %w", evt.RecordID, err)
	}
	if p.MentionModelErr != nil {
		p.Logger.Warn("products extraction skipped: mention model config error",
			"record_id", evt.RecordID, "model_ref", p.MentionModelRef, "error", p.MentionModelErr)
		p.persistProductsStatus(ctx, rec, start, p.MentionModelErr)
		return nil
	}
	if p.RelationModelErr != nil {
		p.Logger.Warn("products extraction skipped: relation model config error",
			"record_id", evt.RecordID, "model_ref", p.RelationModelRef, "error", p.RelationModelErr)
		p.persistProductsStatus(ctx, rec, start, p.RelationModelErr)
		return nil
	}

	inputFilename := filepath.Base(strings.TrimSpace(rec.ResultFilename))
	if inputFilename == "" {
		inputFilename = fmt.Sprintf("record_%d", evt.RecordID)
	}

	if evt.Force {
		_, _ = p.Store.DeleteProductsByInputRecordID(ctx, evt.RecordID)
	} else {
		exists, err := p.Store.ProductsExist(ctx, evt.RecordID)
		if err != nil {
			p.persistProductsStatus(ctx, rec, start, err)
			p.Logger.Error("products exist check error", "error", err, "record_id", evt.RecordID)
			return nil
		}
		if exists {
			p.Logger.Info("products extraction skipped", "record_id", evt.RecordID, "reason", "products already exist and force=false")
			reindexExistingSearchOnSkip(ctx, searchArtifactProduct, evt.RecordID, p.Logger, ReindexProductSearchForRecord)
			p.persistProductsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	blocks, chunks, err := p.resolveProductChunkBlocks(evt, rec)
	if err != nil {
		p.persistProductsStatus(ctx, rec, start, err)
		p.Logger.Error("resolveProductChunkBlocks error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	if len(blocks) == 0 {
		err := fmt.Errorf("(MID_26052007) no chunks found for record_id=%d", evt.RecordID)
		p.persistProductsStatus(ctx, rec, start, err)
		return nil
	}

	p.Logger.Info("Before calling LLM",
		"num_blocks", len(blocks),
		"record_id", evt.RecordID,
		"filename", inputFilename)

	result, err := p.extractProductsFromBlocksWithLLM(ctx, blocks, chunks, buildDocContextLine(rec))
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistProducts(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistProductsStatus(ctx, rec, start, err)
		return nil
	}

	// product_name_id / category_paths are already resolved on result.Products
	// by resolveProductNamesAndCategories (called from
	// extractProductsFromBlocksWithLLM, after Pass 3a translation so a newly
	// proposed row can be enriched with its English name too).
	outputRows := p.buildProductOutputRows(result.Products, start, len(blocks), result.ModelName)
	for i := range outputRows {
		outputRows[i]["product_rel_id"] = fmt.Sprintf("%d_prd_%d", evt.RecordID, i+1)
	}

	inserted, err := p.Store.SaveProducts(ctx, SaveProductsRequest{
		InputRecordID: evt.RecordID,
		Products:      outputRows,
	})
	if err != nil {
		p.persistProductsStatus(ctx, rec, start, err)
		p.Logger.Error("save products error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	if err := p.indexProductsInTree(evt.RecordID, outputRows); err != nil {
		p.Logger.Error("index products tree error", "error", err, "record_id", evt.RecordID)
		p.persistProductsStatus(ctx, rec, start, err)
		return nil
	}
	if err := p.writeProductsArtifact(evt.RecordID, rec, outputRows); err != nil {
		p.Logger.Error("write products artifact error", "error", err, "record_id", evt.RecordID)
		p.persistProductsStatus(ctx, rec, start, err)
		return nil
	}
	if reindexErr := ReindexProductSearchForRecord(ctx, evt.RecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex product search registry failed", "record_id", evt.RecordID, "error", reindexErr)
	}
	// Derive chunk -> product "has-part-component" line_overlap edges (registry-sourced ids).
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, evt.RecordID, searchArtifactProduct, RelationHasPartComponent, blocks); connErr != nil {
		p.Logger.Warn("write has-part-component connections failed", "record_id", evt.RecordID, "error", connErr)
	}
	p.Logger.Info("products extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"products_count", len(outputRows),
		"blocks", len(blocks),
	)
	p.persistProductsStatus(ctx, rec, start, nil)
	p.logProductsSummary(ctx, start, p.Now(), result, len(blocks))
	return nil
}

// resolveProductChunkBlocks loads the persisted .chunks artifact (chunking
// must have run first) and adapts it to the []Block shape the rest of this
// file's mention/candidate pipeline and WriteLineOverlapConnectionsFromRegistry
// already expect, via the same chunksToBlocks adapter extract_metrics uses.
// extract_products previously re-blocked the raw line file directly (reading
// Blocks, not Chunks); switched to chunks so it shares the one chunk set
// every other Phase B extractor reads, instead of maintaining its own
// independent re-blocking of the document.
func (p *ProductsProcessor) resolveProductChunkBlocks(evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Block, []Chunk, error) {
	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, nil, fmt.Errorf("(MID_26052010) resolve input file for record_id=%d: %w", evt.RecordID, err)
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("(MID_26052011) input file not exist: %s", inputPath)
		}
		return nil, nil, fmt.Errorf("(MID_26052012) read input file: %w", err)
	}
	lines, err := ParseInputLines(body)
	if err != nil {
		return nil, nil, fmt.Errorf("(MID_26052014) parse line file for record_id=%d: %w", evt.RecordID, err)
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	chunks, err := loadChunksFromArtifactFile(p.ArtifactDir, evt.RecordID, artifactBase+".chunks", lines)
	if err != nil {
		return nil, nil, fmt.Errorf("(MID_26052013) load chunks for record_id=%d: %w", evt.RecordID, err)
	}
	return chunksToBlocks(chunks), chunks, nil
}

// InitChunkBatch implements ChunkBatchProcessor: it validates config and the
// force=false "already extracted" skip (mirroring HandleEvent), then resets
// per-run batch state. It must not make LLM calls.
func (p *ProductsProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.MentionPromptErr != nil {
		return fmt.Errorf("(MID_26091210) %s mention prompt error: %w", p.Name(), p.MentionPromptErr)
	}
	if p.RelationPromptErr != nil {
		return fmt.Errorf("(MID_26091211) %s relation prompt error: %w", p.Name(), p.RelationPromptErr)
	}
	p.batchSkip = false
	if p.MentionModelErr != nil {
		p.Logger.Warn("products extraction skipped: mention model config error",
			"record_id", recordID, "model_ref", p.MentionModelRef, "error", p.MentionModelErr)
		p.batchSkip = true
		return nil
	}
	if p.RelationModelErr != nil {
		p.Logger.Warn("products extraction skipped: relation model config error",
			"record_id", recordID, "model_ref", p.RelationModelRef, "error", p.RelationModelErr)
		p.batchSkip = true
		return nil
	}

	force, _ := docProcessorFlagsFromContext(ctx)
	p.batchForce = force
	if !force {
		exists, err := p.Store.ProductsExist(ctx, recordID)
		if err != nil {
			return fmt.Errorf("(MID_26091212) %s check products exist: %w", p.Name(), err)
		}
		if exists {
			p.Logger.Info("products extraction skipped", "record_id", recordID, "reason", "products already exist and force=false")
			reindexExistingSearchOnSkip(ctx, searchArtifactProduct, recordID, p.Logger, ReindexProductSearchForRecord)
			p.batchSkip = true
			return nil
		}
	}

	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchBlocks = chunksToBlocks(chunks)
	p.batchDocCtx = docCtx
	p.batchMentions = nil
	p.batchFallbackCount = 0
	p.batchMentionModel = strings.TrimSpace(p.MentionModelName)
	return nil
}

// ProcessChunk implements ChunkBatchProcessor: pass 1 (mention extraction)
// for exactly one chunk, called by the coordinator with LLM_CALL_STAGGER
// between calls across processors so this chunk's canonicalChunkInputText
// document lands back-to-back with other processors' calls for it.
func (p *ProductsProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if p.batchSkip {
		return nil
	}
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26091213) %s chunk index %d out of range (len=%d)", p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	chunk := p.batchChunks[chunkIdx]
	block := chunksToBlocks([]Chunk{chunk})[0]
	mentions, modelName, didFallback, err := p.extractProductMentionsForChunk(ctx, eventIDFromContext(ctx), chunkIdx, len(p.batchChunks), block, chunk, p.batchDocCtx)
	if err != nil {
		if isCtxStopped(ctx) {
			return ErrPipelineStopped
		}
		p.Logger.Warn("extract_products chunk failed", "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	p.batchMu.Lock()
	p.batchMentions = append(p.batchMentions, mentions...)
	if didFallback {
		p.batchFallbackCount++
	}
	if m := strings.TrimSpace(modelName); m != "" {
		p.batchMentionModel = m
	}
	p.batchMu.Unlock()
	return nil
}

// FinalizeChunkBatch implements ChunkBatchProcessor: pass 2 (relation
// enrichment, reusing the single-block fast path so candidates ride the
// cache ProcessChunk already warmed), translation, name/category resolution,
// and the same save/index/artifact/reindex steps HandleEvent runs.
func (p *ProductsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if p.batchSkip {
		return nil
	}
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		return fmt.Errorf("(MID_26091214) %s load kb.inputs record %d: %w", p.Name(), p.batchRecordID, err)
	}
	eventID := eventIDFromContext(ctx)
	maxTasks := p.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 1
	}
	llmCallCount := len(p.batchChunks)
	fallbackCount := p.batchFallbackCount
	mentionsCount := len(p.batchMentions)

	candidates := mergeProductMentionCandidates(p.batchMentions)
	p.Logger.Info("Merged product mention candidates",
		"mentions_count", mentionsCount,
		"candidate_count", len(candidates),
		"record_stage", "post_merge",
	)

	chunksBySeq := make(map[int]Chunk, len(p.batchChunks))
	for _, c := range p.batchChunks {
		chunksBySeq[c.SeqNo] = c
	}

	var result productExtractionResult
	if p.Pass1Only {
		p.Logger.Warn("EXTRACT_PRODUCT_PASS_1_ONLY is set; skipping Pass 2/3 and persisting Pass 1 candidates directly",
			"candidate_count", len(candidates),
		)
		result = productExtractionResult{
			Products:      buildPass1OnlyProductRows(candidates),
			ModelName:     firstNonEmptyTrimmed(p.batchMentionModel, p.MentionModelName),
			LLMCallCount:  llmCallCount,
			FallbackCount: fallbackCount,
			MentionsCount: mentionsCount,
		}
		if p.batchForce {
			_, _ = p.Store.DeleteProductsByInputRecordID(ctx, p.batchRecordID)
		}
		outputRows := p.buildProductOutputRows(result.Products, p.batchStart, len(p.batchBlocks), result.ModelName)
		for i := range outputRows {
			outputRows[i]["product_rel_id"] = fmt.Sprintf("%d_prd_%d", p.batchRecordID, i+1)
		}
		inserted, err := p.Store.SaveProducts(ctx, SaveProductsRequest{
			InputRecordID: p.batchRecordID,
			Products:      outputRows,
		})
		if err != nil {
			p.persistProductsStatus(ctx, rec, p.batchStart, err)
			return fmt.Errorf("(MID_26092101) %s save products (pass-1-only): %w", p.Name(), err)
		}
		runID, _ := runIDFromContext(ctx)
		p.Logger.Info("products extracted (pass-1-only)",
			"record_id", p.batchRecordID,
			"run_id", runID,
			"inserted_rows", inserted,
			"products_count", len(outputRows),
			"blocks", len(p.batchBlocks),
		)
		p.persistProductsStatus(ctx, rec, p.batchStart, nil)
		p.logProductsSummary(ctx, p.batchStart, p.Now(), result, len(p.batchBlocks))
		return nil
	}

	usedRelationModel := strings.TrimSpace(p.RelationModelName)
	pass2Results, pass2Err := p.enrichProductCandidatesWithLLM(ctx, eventID, candidates, chunksBySeq, p.batchDocCtx, maxTasks)
	if pass2Err != nil {
		if isCtxStopped(ctx) || errors.Is(pass2Err, ErrPipelineStopped) {
			p.stopAndPersistProducts(context.Background(), rec, p.batchStart)
			return ErrPipelineStopped
		}
		p.persistProductsStatus(ctx, rec, p.batchStart, pass2Err)
		return fmt.Errorf("(MID_26091215) %s enrich product relations: %w", p.Name(), pass2Err)
	}

	products := make([]map[string]any, 0, len(candidates))
	for _, r := range pass2Results {
		products = append(products, r.rows...)
		llmCallCount++
		if r.didFallback {
			fallbackCount++
		}
		if r.modelName != "" {
			usedRelationModel = r.modelName
		}
	}

	preDedupeCount := len(products)
	products = dedupeFinalProductRows(products)
	p.Logger.Info("Deduped final product relation rows",
		"rows_before_dedup", preDedupeCount,
		"rows_after_dedup", len(products),
		"record_stage", "post_relation_dedup",
	)
	if p.TranslateEnabled {
		p.Logger.Info("Starting product translation pass",
			"row_count", len(products),
			"model_name", p.TranslateModelName,
			"prompt_name", p.TranslatePromptRef,
		)
		var translateErr error
		products, translateErr = p.translateProductRows(ctx, products, eventID, &llmCallCount, &fallbackCount)
		if translateErr != nil {
			p.persistProductsStatus(ctx, rec, p.batchStart, translateErr)
			return fmt.Errorf("(MID_26091216) %s translate products: %w", p.Name(), translateErr)
		}
	}
	p.Logger.Info("Starting product name resolution + catalog categorization",
		"row_count", len(products),
	)
	var resolveErr error
	products, resolveErr = p.resolveProductNamesAndCategories(ctx, products)
	if resolveErr != nil {
		p.persistProductsStatus(ctx, rec, p.batchStart, resolveErr)
		return fmt.Errorf("(MID_26091217) %s resolve product names: %w", p.Name(), resolveErr)
	}

	result = productExtractionResult{
		Products:      products,
		ModelName:     firstNonEmptyTrimmed(usedRelationModel, p.batchMentionModel, p.RelationModelName, p.ModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		MentionsCount: mentionsCount,
	}

	if p.batchForce {
		_, _ = p.Store.DeleteProductsByInputRecordID(ctx, p.batchRecordID)
	}
	outputRows := p.buildProductOutputRows(result.Products, p.batchStart, len(p.batchBlocks), result.ModelName)
	for i := range outputRows {
		outputRows[i]["product_rel_id"] = fmt.Sprintf("%d_prd_%d", p.batchRecordID, i+1)
	}
	inserted, err := p.Store.SaveProducts(ctx, SaveProductsRequest{
		InputRecordID: p.batchRecordID,
		Products:      outputRows,
	})
	if err != nil {
		p.persistProductsStatus(ctx, rec, p.batchStart, err)
		return fmt.Errorf("(MID_26091218) %s save products: %w", p.Name(), err)
	}
	if err := p.indexProductsInTree(p.batchRecordID, outputRows); err != nil {
		p.persistProductsStatus(ctx, rec, p.batchStart, err)
		return fmt.Errorf("(MID_26091219) %s index products tree: %w", p.Name(), err)
	}
	if err := p.writeProductsArtifact(p.batchRecordID, rec, outputRows); err != nil {
		p.persistProductsStatus(ctx, rec, p.batchStart, err)
		return fmt.Errorf("(MID_26091220) %s write products artifact: %w", p.Name(), err)
	}
	if reindexErr := ReindexProductSearchForRecord(ctx, p.batchRecordID, p.Logger); reindexErr != nil {
		p.Logger.Warn("reindex product search registry failed", "record_id", p.batchRecordID, "error", reindexErr)
	}
	if connErr := WriteLineOverlapConnectionsFromRegistry(ctx, p.batchRecordID, searchArtifactProduct, RelationHasPartComponent, p.batchBlocks); connErr != nil {
		p.Logger.Warn("write has-part-component connections failed", "record_id", p.batchRecordID, "error", connErr)
	}
	p.Logger.Info("products extracted",
		"record_id", p.batchRecordID,
		"inserted_rows", inserted,
		"products_count", len(outputRows),
		"blocks", len(p.batchBlocks),
	)
	p.persistProductsStatus(ctx, rec, p.batchStart, nil)
	p.logProductsSummary(ctx, p.batchStart, p.Now(), result, len(p.batchBlocks))
	return nil
}

// productPass1Result is one Pass 1 (mention extraction) task's outcome,
// collected by runConcurrent and merged in block-index order afterward.
type productPass1Result struct {
	mentions    []productMention
	modelName   string
	didFallback bool
}

// productPass2Result is one Pass 2 (relation enrichment) task's outcome.
type productPass2Result struct {
	rows        []map[string]any
	modelName   string
	didFallback bool
}

func (p *ProductsProcessor) extractProductsFromBlocksWithLLM(ctx context.Context, blocks []Block, chunks []Chunk, docCtx string) (productExtractionResult, error) {
	eventID := eventIDFromContext(ctx)
	usedMentionModel := strings.TrimSpace(p.MentionModelName)
	maxTasks := p.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 1
	}
	// chunksBySeq looks a block/candidate up by Block.Index (== Chunk.SeqNo,
	// per chunksToBlocks) so both passes below can send that chunk's
	// canonicalChunkInputText as their document -- the same bytes every other
	// chunk-based processor sends for it, forming the stable, cacheable
	// prefix DeepSeek's prompt cache needs. See input_lines.go's
	// canonicalChunkInputText doc comment.
	chunksBySeq := make(map[int]Chunk, len(chunks))
	for _, c := range chunks {
		chunksBySeq[c.SeqNo] = c
	}

	// Pass 1: concurrent per-block mention extraction (mirrors
	// extract_metrics' extractMetricsFromChunksWithLLM). Each block is
	// independent -- no cross-block state -- so this is the same fan-out
	// extract_metrics already uses; MaxTasks defaults to 1 (sequential,
	// today's behavior) until EXTRACT_PRODUCTS_MAX_TASKS raises it.
	pass1Results, pass1Err := runConcurrent(ctx, maxTasks, len(blocks), func(concCtx context.Context, idx int) (productPass1Result, error) {
		block := blocks[idx]
		if isCtxStopped(concCtx) {
			return productPass1Result{}, ErrPipelineStopped
		}
		chunk, ok := chunksBySeq[block.Index]
		if !ok {
			return productPass1Result{}, fmt.Errorf("(MID_26052040) extract product mentions: no chunk with seq %d (chunks=%d)", block.Index, len(chunks))
		}
		mentions, modelName, didFallback, err := p.extractProductMentionsForChunk(concCtx, eventID, idx, len(blocks), block, chunk, docCtx)
		if err != nil {
			if isCtxStopped(concCtx) {
				return productPass1Result{}, ErrPipelineStopped
			}
			p.Logger.Error("failed extracting product mentions", "error", err)
			return productPass1Result{}, fmt.Errorf("(MID_26052020) extract product mentions via llm: %w", err)
		}
		return productPass1Result{
			mentions:    mentions,
			modelName:   strings.TrimSpace(modelName),
			didFallback: didFallback,
		}, nil
	})
	if pass1Err != nil {
		if isCtxStopped(ctx) || errors.Is(pass1Err, ErrPipelineStopped) {
			return productExtractionResult{}, ErrPipelineStopped
		}
		return productExtractionResult{}, pass1Err
	}

	mentions := make([]productMention, 0, len(blocks))
	llmCallCount := len(blocks)
	fallbackCount := 0
	for _, r := range pass1Results {
		mentions = append(mentions, r.mentions...)
		if r.didFallback {
			fallbackCount++
		}
		if r.modelName != "" {
			usedMentionModel = r.modelName
		}
	}

	candidates := mergeProductMentionCandidates(mentions)
	p.Logger.Info("Merged product mention candidates",
		"mentions_count", len(mentions),
		"candidate_count", len(candidates),
		"record_stage", "post_merge",
	)

	if p.Pass1Only {
		p.Logger.Warn("EXTRACT_PRODUCT_PASS_1_ONLY is set; skipping Pass 2/3 and persisting Pass 1 candidates directly",
			"candidate_count", len(candidates),
		)
		return productExtractionResult{
			Products:      buildPass1OnlyProductRows(candidates),
			ModelName:     usedMentionModel,
			LLMCallCount:  llmCallCount,
			FallbackCount: fallbackCount,
			MentionsCount: len(mentions),
		}, nil
	}

	// Pass 2: concurrent per-candidate relation enrichment, same fan-out.
	usedRelationModel := strings.TrimSpace(p.RelationModelName)
	pass2Results, pass2Err := p.enrichProductCandidatesWithLLM(ctx, eventID, candidates, chunksBySeq, docCtx, maxTasks)
	if pass2Err != nil {
		if isCtxStopped(ctx) || errors.Is(pass2Err, ErrPipelineStopped) {
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, ErrPipelineStopped
		}
		return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, pass2Err
	}

	products := make([]map[string]any, 0, len(candidates))
	for _, r := range pass2Results {
		products = append(products, r.rows...)
		llmCallCount++
		if r.didFallback {
			fallbackCount++
		}
		if r.modelName != "" {
			usedRelationModel = r.modelName
		}
	}

	preDedupeCount := len(products)
	products = dedupeFinalProductRows(products)
	p.Logger.Info("Deduped final product relation rows",
		"rows_before_dedup", preDedupeCount,
		"rows_after_dedup", len(products),
		"record_stage", "post_relation_dedup",
	)
	if p.TranslateEnabled {
		p.Logger.Info("Starting product translation pass",
			"row_count", len(products),
			"model_name", p.TranslateModelName,
			"prompt_name", p.TranslatePromptRef,
		)
		var translateErr error
		products, translateErr = p.translateProductRows(ctx, products, eventID, &llmCallCount, &fallbackCount)
		if translateErr != nil {
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, translateErr
		}
	}
	p.Logger.Info("Starting product name resolution + catalog categorization",
		"row_count", len(products),
	)
	var resolveErr error
	products, resolveErr = p.resolveProductNamesAndCategories(ctx, products)
	if resolveErr != nil {
		return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, resolveErr
	}
	return productExtractionResult{
		Products:      products,
		ModelName:     firstNonEmptyTrimmed(usedRelationModel, usedMentionModel, p.RelationModelName, p.ModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		MentionsCount: len(mentions),
	}, nil
}

// extractProductMentionsForChunk runs pass 1 for exactly one chunk/block.
// Shared by the direct HandleEvent path (extractProductsFromBlocksWithLLM's
// pass 1 loop) and the per-chunk batching coordinator path (ProcessChunk), so
// both send the identical canonicalChunkInputText document for a given
// chunk -- required for the coordinator's cross-processor cache sharing.
func (p *ProductsProcessor) extractProductMentionsForChunk(
	ctx context.Context,
	eventID string,
	idx int,
	total int,
	block Block,
	chunk Chunk,
	docCtx string,
) ([]productMention, string, bool, error) {
	callStart := p.Now()
	p.Logger.Info("extract product mentions - begin",
		"idx", idx,
		"total", total,
		"model_name", p.MentionModelName,
		"prompt_name", p.MentionPromptRef,
	)
	// Document is canonicalChunkInputText(chunk, docCtx) so this call rides
	// the coordinator's cross-processor cache for this chunk; the schema
	// and block index are task-specific, so they go in the <TASK> suffix.
	inputText := canonicalChunkInputText(chunk.Lines, docCtx)
	taskText := p.MentionPromptText + "\n\n" + buildProductMentionsTaskPrompt(block.Index)
	payload, modelName, err := p.extractProductPayloadWithFallback(ctx,
		"extract products", inputText,
		taskText, p.MentionPromptRef,
		p.MentionModelName, p.MentionModelCfg)
	p.logLLMCall(ctx, fmt.Sprintf("%s_p1_b%d", eventID, idx), "extract_product_mentions", 1, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.MentionPromptRef), nil, err, callStart, p.Now())
	if err != nil {
		return nil, modelName, false, err
	}
	raw, _ := payload["mentions"].([]any)
	mentions := normalizeProductMentions(raw, block)
	didFallback := strings.TrimSpace(modelName) != strings.TrimSpace(p.MentionModelName) && strings.TrimSpace(modelName) != ""
	cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
	outputTokens := outputTokenCount(p.Extractor)
	p.Logger.Info("extract product mentions - end",
		"mention count", len(mentions),
		"cache_hit", cacheHit,
		"cache_miss", cacheMiss,
		"output_tokens", outputTokens,
		"ms_used", time.Since(callStart).Milliseconds())
	return mentions, modelName, didFallback, nil
}

// enrichProductCandidatesWithLLM runs pass 2 (concurrent per-candidate
// relation enrichment) for a set of merged candidates. Shared by the direct
// HandleEvent path and FinalizeChunkBatch, so both single-block candidates
// reuse pass 1's canonicalChunkInputText document (see productCandidate.
// BlockIndex) the same way.
func (p *ProductsProcessor) enrichProductCandidatesWithLLM(
	ctx context.Context,
	eventID string,
	candidates []productCandidate,
	chunksBySeq map[int]Chunk,
	docCtx string,
	maxTasks int,
) ([]productPass2Result, error) {
	var mu sync.Mutex
	productsSoFar := 0

	p.Logger.Info("enrich product",
		"total candidates", len(candidates),
		"model_name", p.RelationModelName,
		"prompt_name", p.RelationPromptRef,
	)

	return runConcurrent(ctx, maxTasks, len(candidates), func(concCtx context.Context, idx int) (productPass2Result, error) {
		candidate := candidates[idx]
		if isCtxStopped(concCtx) {
			return productPass2Result{}, ErrPipelineStopped
		}
		callStart := p.Now()
		p.Logger.Info("enrich product - begin",
			"idx", idx,
			"total", len(candidates),
			"candidate_id", candidate.CandidateID,
		)
		var inputText, taskText string
		if chunk, ok := chunksBySeq[candidate.BlockIndex]; ok {
			// Candidate's evidence lives entirely in one block: send that
			// chunk's canonical text as the document, byte-identical to what
			// pass 1 already sent for it, so this call rides pass 1's
			// already-warm cache instead of paying for a bespoke,
			// never-repeating candidate blob.
			inputText = canonicalChunkInputText(chunk.Lines, docCtx)
			taskText = p.RelationPromptText + "\n\n" + buildProductRelationTaskPrompt(candidate)
		} else {
			// Candidate's mentions span multiple blocks -- no single chunk to
			// send as the document, so fall back to the merged source-lines
			// blob (uncacheable, same as before).
			inputText = buildProductRelationUserPrompt(candidate)
			taskText = p.RelationPromptText
		}
		payload, modelName, err := p.extractProductPayloadWithFallback(concCtx,
			"product relations", inputText,
			taskText, p.RelationPromptRef,
			p.RelationModelName, p.RelationModelCfg)
		p.logLLMCall(ctx, fmt.Sprintf("%s_p2_c%d", eventID, idx), "enrich_product_relations", 2, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.RelationPromptRef), nil, err, callStart, p.Now())
		if err != nil {
			if isCtxStopped(concCtx) {
				return productPass2Result{}, ErrPipelineStopped
			}
			p.Logger.Error("failed enriching product relations", "error", err, "candidate_id", candidate.CandidateID)
			return productPass2Result{}, fmt.Errorf("(MID_26052020) enrich product relations via llm: %w", err)
		}
		raw, _ := payload["products"].([]any)
		normalized := normalizeProductList(raw)
		mu.Lock()
		productsSoFar += len(normalized)
		soFar := productsSoFar
		mu.Unlock()
		cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
		outputTokens := outputTokenCount(p.Extractor)
		p.Logger.Info("enrich product - end",
			"candidate_id", candidate.CandidateID,
			"rows", len(normalized),
			"products_so_far", soFar,
			"cache_hit", cacheHit,
			"cache_miss", cacheMiss,
			"output_tokens", outputTokens,
			"ms_used", time.Since(callStart).Milliseconds())
		return productPass2Result{
			rows:        normalized,
			modelName:   strings.TrimSpace(modelName),
			didFallback: strings.TrimSpace(modelName) != strings.TrimSpace(p.RelationModelName) && strings.TrimSpace(modelName) != "",
		}, nil
	})
}

func (p *ProductsProcessor) extractProductPayloadWithFallback(
	ctx context.Context,
	opr string,
	inputText string,
	promptText string,
	promptRef string,
	modelName string,
	cfg structureModelConfig) (map[string]any, string, error) {
	primaryStart := time.Now()
	payload, err := p.extractProductPayload(ctx, opr, inputText, promptText, promptRef, modelName, cfg)
	if err == nil {
		return payload, strings.TrimSpace(modelName), nil
	}

	var payloadVal any = "nil"
	if payload != nil {
		payloadVal = payload
	}
	p.Logger.Warn("primary LLM failed",
		"action", opr,
		"error", err,
		"model_name", modelName,
		"fallback_model", p.FallbackModelName,
		"payload", payloadVal,
		"ms_used", time.Since(primaryStart).Milliseconds())

	if isEmptyProductExtractionError(err) {
		p.Logger.Warn("primary products extraction returned empty JSON; treating as empty result without fallback",
			"model_name", modelName,
			"error", err,
			"prompt_name", promptRef,
		)
		return map[string]any{"products": []any{}, "mentions": []any{}}, strings.TrimSpace(modelName), nil
	}

	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, strings.TrimSpace(modelName),
			fmt.Errorf("(MID_26052021) extract products failed and fallback model not available, err:%w, model_name:%s", err, modelName)
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26052022) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	p.Logger.Warn("primary products; retrying fallback model",
		"action", opr,
		"primary_model", modelName,
		"fallback_model", fallbackModelName,
		"error", err,
		"prompt_name", promptRef,
	)

	payload, fallbackErr := p.extractProductPayload(ctx, opr, inputText, promptText, promptRef, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if isEmptyProductExtractionError(fallbackErr) {
			p.Logger.Warn("fallback products extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", promptRef,
			)
			return map[string]any{"products": []any{}, "mentions": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26052023) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func isEmptyProductExtractionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return strings.Contains(msg, "unexpected end of JSON input") &&
		strings.Contains(msg, "json:{[]}")
}

func (p *ProductsProcessor) extractProductPayload(
	ctx context.Context,
	opr string,
	inputText string,
	promptText string,
	promptRef string,
	modelName string,
	cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)

	startTime := time.Now()
	// p.Logger.Info("extract products - begin",
	// 	"action", opr,
	// 	"model", modelName,
	// 	"prompt_name", promptRef,
	// )

	callReason := strings.TrimSpace(opr)
	if callReason == "" {
		callReason = "extract_products"
	}
	in := newLLMJSONInput(ctx, promptRef, promptText, modelName, inputText, callReason, "MID-CWB-EXTRACT-PRODUCTS")
	var (
		payload map[string]any
		err     error
	)
	if structuredExtractor, ok := p.Extractor.(LLMStructuredJSONExtractor); ok {
		var result *llmclients.StructuredOutputResult
		result, err = structuredExtractor.ExtractStructuredJSON(ctx, in, productExtractionContract())
		if result != nil {
			payload = result.Parsed
		}
	} else {
		payload, err = p.Extractor.ExtractJSON(ctx, in)
	}
	if err != nil {
		if payload == nil {
			return nil, fmt.Errorf("(MID_26052030) failed extracting products, error:%w", err)
		}
		return nil, fmt.Errorf("(MID_26052031) failed extracting products, error:%w, payload:%v", err, payload)
	}

	payload = normalizeProductPayload(payload)
	if len(payload) == 0 {
		return nil, errors.New("(MID_26052032) empty llm json object")
	}
	if raw, ok := payload["mentions"]; ok {
		items, ok := normalizeProductItems(raw)
		if !ok {
			return nil, fmt.Errorf("(MID_26052034) llm output field 'mentions' must be an array, JSON:%v", payload)
		}
		payload["mentions"] = items
		return payload, nil
	}
	raw, ok := payload["products"]
	if !ok {
		return nil, fmt.Errorf("(MID_26052033) llm output must contain 'products' or 'mentions', JSON:%v", payload)
	}
	items, ok := normalizeProductItems(raw)
	if !ok {
		return nil, fmt.Errorf("(MID_26052034) llm output field 'products' must be an array, JSON:%v", payload)
	}
	payload["products"] = items

	cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
	p.Logger.Info("extract products - end",
		"action", opr,
		"ms_used", time.Since(startTime).Milliseconds(),
		"cache_hit", cacheHit,
		"cache_miss", cacheMiss)
	return payload, nil
}

func normalizeProductPayload(payload map[string]any) map[string]any {
	if len(payload) == 0 {
		return payload
	}
	if _, ok := payload["products"]; ok {
		return payload
	}
	if looksLikeProductRecord(payload) {
		return map[string]any{"products": []any{payload}}
	}
	return payload
}

func normalizeProductItems(value any) ([]any, bool) {
	switch v := value.(type) {
	case []any:
		return v, true
	case map[string]any:
		return []any{v}, true
	default:
		return nil, false
	}
}

func looksLikeProductRecord(payload map[string]any) bool {
	if len(payload) == 0 {
		return false
	}
	for _, key := range []string{"product_name", "canonical_name", "product_type", "relation_type", "product_summary", "relation_summary"} {
		if strings.TrimSpace(asString(payload[key])) != "" {
			return true
		}
	}
	return false
}

// buildProductMentionsTaskPrompt returns the mentions-pass schema and block
// index only -- the block's lines are no longer duplicated here, since the
// canonicalChunkInputText document (sent separately, see
// extractProductsFromBlocksWithLLM) already carries every line.
func buildProductMentionsTaskPrompt(blockIndex int) string {
	schema := map[string]any{
		"mentions": []map[string]any{{
			"mention_text":      "string",
			"canonical_hint":    "string or null",
			"product_type_hint": "specific_product|product_class|component|material|software|system|equipment|consumable|packaging|other|unknown",
			"evidence_quote":    "short supporting quote from the input",
			"evidence_lines":    []string{"32", "35-45"},
			"is_explicit":       true,
			"confidence":        0.0,
			"confidence_reason": "brief reason",
		}},
	}
	schemaJSON, _ := json.Marshal(schema)
	return "Return JSON only. Use exactly this top-level schema:\n" + string(schemaJSON) +
		"\n\nBlock index: " + strconv.Itoa(blockIndex)
}

// buildProductRelationUserPrompt is the multi-block fallback document: used
// only when a candidate's mentions span more than one block, so there's no
// single chunk to send as the canonical document and the merged source lines
// must be included here instead.
func buildProductRelationUserPrompt(candidate productCandidate) string {
	candidateJSON, _ := json.Marshal(map[string]any{
		"candidate_id":        candidate.CandidateID,
		"product_name":        candidate.ProductName,
		"canonical_name":      candidate.CanonicalName,
		"product_type_hint":   candidate.ProductTypeHint,
		"supporting_mentions": candidate.SupportingMentions,
	})
	return "Return JSON only.\n\nCandidate:\n" + string(candidateJSON) +
		"\n\nSource lines (JSON array):\n" + blockLinesToJSON(candidate.SupportLines)
}

// buildProductRelationTaskPrompt is the single-block task suffix: the
// candidate JSON only, no source lines, since the canonicalChunkInputText
// document already carries every line of that candidate's one block.
func buildProductRelationTaskPrompt(candidate productCandidate) string {
	candidateJSON, _ := json.Marshal(map[string]any{
		"candidate_id":        candidate.CandidateID,
		"product_name":        candidate.ProductName,
		"canonical_name":      candidate.CanonicalName,
		"product_type_hint":   candidate.ProductTypeHint,
		"supporting_mentions": candidate.SupportingMentions,
	})
	return "Return JSON only.\n\nCandidate:\n" + string(candidateJSON)
}

func normalizeProductMentions(items []any, block Block) []productMention {
	out := make([]productMention, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		evidenceLines := normalizeProductEvidenceLines(raw["evidence_lines"])
		out = append(out, productMention{
			MentionText:       strings.TrimSpace(asString(raw["mention_text"])),
			CanonicalHint:     strings.TrimSpace(asString(raw["canonical_hint"])),
			ProductTypeHint:   strings.TrimSpace(asString(raw["product_type_hint"])),
			EvidenceQuote:     strings.TrimSpace(asString(raw["evidence_quote"])),
			EvidenceLines:     evidenceLines,
			IsExplicit:        toBool(raw["is_explicit"]),
			Confidence:        toFloat(raw["confidence"]),
			ConfidenceReason:  strings.TrimSpace(asString(raw["confidence_reason"])),
			BlockIndex:        block.Index,
			BlockLines:        append([]BlockLine(nil), block.Lines...),
			HasNormalEvidence: mentionHasNormalEvidence(block, evidenceLines, strings.TrimSpace(asString(raw["evidence_quote"]))),
		})
	}
	return out
}

func mentionHasNormalEvidence(block Block, spans []string, quote string) bool {
	lineNums := make(map[int]struct{})
	for _, span := range spans {
		start, end, ok := parseCompactLineSpan(span)
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

func parseCompactLineSpan(span string) (int, int, bool) {
	span = strings.TrimSpace(span)
	if span == "" {
		return 0, 0, false
	}
	if strings.Contains(span, "-") {
		parts := strings.SplitN(span, "-", 2)
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

func mergeProductMentionCandidates(mentions []productMention) []productCandidate {
	type bucket struct {
		mentions []productMention
	}
	grouped := map[string]*bucket{}
	order := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		key := normalizedProductCandidateKey(mention.CanonicalHint, mention.MentionText)
		if key == "" {
			continue
		}
		if grouped[key] == nil {
			grouped[key] = &bucket{}
			order = append(order, key)
		}
		grouped[key].mentions = append(grouped[key].mentions, mention)
	}

	out := make([]productCandidate, 0, len(order))
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
		productName := ""
		canonicalName := ""
		productType := "unknown"
		typeCounts := map[string]int{}
		blockIndex := b.mentions[0].BlockIndex
		sameBlock := true
		for _, mention := range b.mentions {
			if mention.BlockIndex != blockIndex {
				sameBlock = false
			}
			if productName == "" || (mention.HasNormalEvidence && productName == b.mentions[0].MentionText) {
				productName = firstNonEmptyTrimmed(mention.MentionText, productName)
			}
			canonicalName = firstNonEmptyTrimmed(canonicalName, mention.CanonicalHint)
			if t := strings.TrimSpace(mention.ProductTypeHint); t != "" {
				typeCounts[t]++
			}
			supportMentions = append(supportMentions, map[string]any{
				"mention_text":   mention.MentionText,
				"evidence_quote": mention.EvidenceQuote,
				"evidence_lines": mention.EvidenceLines,
				"is_explicit":    mention.IsExplicit,
				"confidence":     mention.Confidence,
			})
			for _, line := range mention.BlockLines {
				lineKey := fmt.Sprintf("%d:%d:%s", line.PageNumber, line.LineNumber, line.Content)
				existing, exists := lineMap[lineKey]
				if !exists || (existing.Flag != "n" && line.Flag == "n") {
					lineMap[lineKey] = line
				}
			}
		}
		for candidateType, count := range typeCounts {
			if count > typeCounts[productType] {
				productType = candidateType
			}
		}
		if productType == "" {
			productType = "unknown"
		}
		if canonicalName == "" {
			canonicalName = productName
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
		candidateBlockIndex := -1
		if sameBlock {
			candidateBlockIndex = blockIndex
		}
		out = append(out, productCandidate{
			CandidateID:        fmt.Sprintf("cand_%d", len(out)+1),
			ProductName:        productName,
			CanonicalName:      canonicalName,
			ProductTypeHint:    productType,
			SupportingMentions: supportMentions,
			SupportLines:       supportLines,
			BlockIndex:         candidateBlockIndex,
		})
	}
	return out
}

func normalizedProductCandidateKey(canonicalHint string, mentionText string) string {
	base := firstNonEmptyTrimmed(canonicalHint, mentionText)
	base = strings.ToLower(strings.TrimSpace(base))
	base = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r):
			return unicode.ToLower(r)
		case unicode.IsNumber(r):
			return r
		case unicode.IsSpace(r):
			return r
		default:
			return ' '
		}
	}, base)
	return strings.Join(strings.Fields(base), " ")
}

// dedupeFinalProductRows merges rows that describe the same product-relation
// pair. Two rows are duplicates when they share canonical_name + relation_type
// AND their evidence_lines overlap — Pass 2 (an LLM call) can paraphrase
// requirement_text differently across rows derived from the same underlying
// evidence, so exact requirement_text equality alone is not a reliable
// dedup signal (see extract-products-spec.md, Deterministic Step B).
func dedupeFinalProductRows(products []map[string]any) []map[string]any {
	groupKey := func(product map[string]any) string {
		return strings.Join([]string{
			normalizedProductCandidateKey(asString(product["canonical_name"]), asString(product["product_name"])),
			strings.ToLower(strings.TrimSpace(asString(product["relation_type"]))),
		}, "|")
	}
	lineSet := func(product map[string]any) map[string]bool {
		set := map[string]bool{}
		for _, line := range toStringSlice(product["evidence_lines"]) {
			set[line] = true
		}
		return set
	}
	overlaps := func(a, b map[string]bool) bool {
		for line := range a {
			if b[line] {
				return true
			}
		}
		return false
	}

	type cluster struct {
		row   map[string]any
		lines map[string]bool
	}
	clustersByKey := map[string][]*cluster{}
	order := make([]*cluster, 0, len(products))

	for _, product := range products {
		key := groupKey(product)
		lines := lineSet(product)

		var target *cluster
		for _, c := range clustersByKey[key] {
			if overlaps(c.lines, lines) {
				target = c
				break
			}
		}
		if target == nil {
			cloned := map[string]any{}
			for k, v := range product {
				cloned[k] = v
			}
			target = &cluster{row: cloned, lines: lines}
			clustersByKey[key] = append(clustersByKey[key], target)
			order = append(order, target)
			continue
		}

		for line := range lines {
			target.lines[line] = true
		}
		existing := target.row
		mergedLines := toStringSlice(existing["evidence_lines"])
		for _, line := range toStringSlice(product["evidence_lines"]) {
			mergedLines = appendUniqueString(mergedLines, line)
		}
		existing["evidence_lines"] = mergedLines
		if toFloat(product["confidence"]) > toFloat(existing["confidence"]) {
			existing["confidence"] = product["confidence"]
			existing["confidence_reason"] = product["confidence_reason"]
			existing["requirement_text"] = product["requirement_text"]
		}
		if strings.TrimSpace(asString(existing["evidence_quote"])) == "" {
			existing["evidence_quote"] = product["evidence_quote"]
		}
	}

	out := make([]map[string]any, 0, len(order))
	for _, c := range order {
		out = append(out, c.row)
	}
	return out
}

func normalizeProductList(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		details, _ := raw["relation_details"].(map[string]any)
		if details == nil {
			details = map[string]any{}
		}

		evidenceQuote := strings.TrimSpace(asString(raw["evidence_quote"]))
		if evidenceQuote == "" {
			if evMap, ok := raw["evidence"].(map[string]any); ok {
				evidenceQuote = strings.TrimSpace(asString(evMap["quote"]))
			}
		}

		thresholds := details["thresholds_or_parameters"]
		if thresholds == nil {
			thresholds = []any{}
		}
		relatedProducts := details["related_products"]
		if relatedProducts == nil {
			relatedProducts = []any{}
		}

		relationSummary := strings.TrimSpace(asString(raw["product_summary"]))
		if relationSummary == "" {
			relationSummary = strings.TrimSpace(asString(raw["relation_summary"]))
		}
		relationSummaryEn := strings.TrimSpace(asString(raw["product_summary_en"]))
		if relationSummaryEn == "" {
			relationSummaryEn = strings.TrimSpace(asString(raw["relation_summary_en"]))
		}

		out = append(out, map[string]any{
			"product_name":         strings.TrimSpace(asString(raw["product_name"])),
			"product_name_en":      strings.TrimSpace(asString(raw["product_name_en"])),
			"canonical_name":       strings.TrimSpace(asString(raw["canonical_name"])),
			"canonical_name_en":    strings.TrimSpace(asString(raw["canonical_name_en"])),
			"product_type":         strings.TrimSpace(asString(raw["product_type"])),
			"relation_type":        strings.TrimSpace(asString(raw["relation_type"])),
			"relation_summary":     relationSummary,
			"relation_summary_en":  relationSummaryEn,
			"evidence_quote":       evidenceQuote,
			"evidence_lines":       normalizeProductEvidenceLines(raw["evidence_lines"]),
			"obligation_level":     strings.TrimSpace(asString(details["obligation_level"])),
			"requirement_text":     strings.TrimSpace(asString(details["requirement_text"])),
			"requirement_text_en":  strings.TrimSpace(asString(details["requirement_text_en"])),
			"conditions":           toStringSlice(details["conditions"]),
			"exceptions":           toStringSlice(details["exceptions"]),
			"parameters":           thresholds,
			"related_products":     relatedProducts,
			"responsible_actor":    strings.TrimSpace(asString(details["responsible_actor"])),
			"confidence":           toFloat(raw["confidence"]),
			"confidence_reason":    strings.TrimSpace(asString(raw["confidence_reason"])),
			"confidence_reason_en": strings.TrimSpace(asString(raw["confidence_reason_en"])),
			"category_paths":       raw["category_paths"],
			"category_paths_en":    raw["category_paths_en"],
		})
	}
	return out
}

// buildPass1OnlyProductRows converts deduplicated Pass 1 candidates directly
// into product-relation rows for EXTRACT_PRODUCT_PASS_1_ONLY mode, skipping
// Pass 2 (relation enrichment) and Pass 3 (translation + name/category
// resolution) entirely. Candidates are already deduplicated by Deterministic
// Step A (mergeProductMentionCandidates), so no further dedup is needed here.
// relation_type, requirement/translation fields, and category_paths are left
// unset -- buildProductOutputRows defaults them for any row missing them.
func buildPass1OnlyProductRows(candidates []productCandidate) []map[string]any {
	out := make([]map[string]any, 0, len(candidates))
	for _, c := range candidates {
		evidenceQuote := ""
		var evidenceLines []string
		confidence := 0.0
		for _, m := range c.SupportingMentions {
			if evidenceQuote == "" {
				evidenceQuote = strings.TrimSpace(asString(m["evidence_quote"]))
			}
			evidenceLines = append(evidenceLines, normalizeProductEvidenceLines(m["evidence_lines"])...)
			if conf := toFloat(m["confidence"]); conf > confidence {
				confidence = conf
			}
		}
		out = append(out, map[string]any{
			"product_name":      c.ProductName,
			"canonical_name":    c.CanonicalName,
			"product_type":      c.ProductTypeHint,
			"evidence_quote":    evidenceQuote,
			"evidence_lines":    uniqueStrings(evidenceLines),
			"confidence":        confidence,
			"confidence_reason": "",
		})
	}
	return out
}

func normalizeProductEvidenceLines(value any) []string {
	switch v := value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := strings.TrimSpace(asString(item))
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (p *ProductsProcessor) logLLMCall(
	ctx context.Context,
	callID, activity string,
	pass int,
	modelNames []string,
	promptName string,
	payload map[string]any,
	callErr error,
	start, end time.Time,
) {
	if p.ProcLogger.DB == nil {
		return
	}
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
	cacheHit, cacheMiss := extractorCacheTokens(p.Extractor)
	rec := DocProcLogRecord{
		DocProcName:           p.Name(),
		ModelNames:            modelNames,
		PromptName:            promptName,
		Pass:                  &pass,
		LLMCallID:             &callID,
		ActivityName:          &activity,
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	if err := p.ProcLogger.LogLLMCall(ctx, rec, "MID-26052810"); err != nil {
		p.Logger.Warn("failed to write llm_call log", "call_id", callID, "error", err)
	}
}

func (p *ProductsProcessor) logProductsSummary(ctx context.Context, start, end time.Time, result productExtractionResult, numBlocks int) {
	if p.ProcLogger.DB == nil {
		return
	}
	modelNames := make([]string, 0, 3)
	for _, n := range []string{
		strings.TrimSpace(p.MentionModelName),
		strings.TrimSpace(p.RelationModelName),
		strings.TrimSpace(p.FallbackModelName),
	} {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if !slices.Contains(modelNames, n) {
			modelNames = append(modelNames, n)
		}
	}
	if p.TranslateEnabled && strings.TrimSpace(p.TranslateModelName) != "" {
		if !slices.Contains(modelNames, strings.TrimSpace(p.TranslateModelName)) {
			modelNames = append(modelNames, strings.TrimSpace(p.TranslateModelName))
		}
	}
	promptName := firstNonEmptyTrimmed(p.MentionPromptRef, p.RelationPromptRef)
	extraInfo, _ := json.Marshal(map[string]interface{}{
		"total_products": len(result.Products),
		"mentions_count": result.MentionsCount,
		"llm_call_count": result.LLMCallCount,
		"fallback_count": result.FallbackCount,
		"num_blocks":     numBlocks,
	})
	extraStr := string(extraInfo)
	if err := p.ProcLogger.LogSummary(ctx, EntryTypeExtractProducts, DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}, "MID-26052810"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

const defaultTranslateBatchSize = 10

// translateProductRows batch-translates products' name/summary/requirement
// fields, N rows per LLM call (default 10, TRANSLATE_PRODUCTS_BATCH_SIZE),
// with batches run concurrently (p.MaxTasks) -- previously one row per call,
// sequential. Each request row carries an "idx" (its position within the
// batch) that the LLM must echo back; returned rows are matched to the row
// they translate by that idx rather than by position, so if the model drops
// or reorders a row, the rows it did return still land on the right product
// instead of the whole batch being discarded. A batch that fails outright,
// or a row within a batch whose idx is missing/invalid, is logged and left
// untranslated (same graceful-degrade contract as before, now scoped to a
// batch instead of a single row).
func (p *ProductsProcessor) translateProductRows(ctx context.Context, products []map[string]any, eventID string, llmCallCount *int, fallbackCount *int) ([]map[string]any, error) {
	if len(products) == 0 {
		return products, nil
	}
	batchSize := p.TranslateBatchSize
	if batchSize <= 0 {
		batchSize = defaultTranslateBatchSize
	}
	maxTasks := p.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 1
	}
	numBatches := (len(products) + batchSize - 1) / batchSize

	type translateBatchOutcome struct {
		calls     int
		fallbacks int
	}
	outcomes, err := runConcurrent(ctx, maxTasks, numBatches, func(concCtx context.Context, b int) (translateBatchOutcome, error) {
		start := b * batchSize
		end := min(start+batchSize, len(products))
		if isCtxStopped(concCtx) {
			return translateBatchOutcome{}, ErrPipelineStopped
		}
		callStart := p.Now()
		items := make([]map[string]any, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, map[string]any{
				"idx":               i - start,
				"product_name":      products[i]["product_name"],
				"canonical_name":    products[i]["canonical_name"],
				"product_summary":   products[i]["relation_summary"],
				"requirement_text":  products[i]["requirement_text"],
				"confidence_reason": products[i]["confidence_reason"],
			})
		}
		rowInput, _ := json.Marshal(map[string]any{"products": items})
		payload, modelName, err := p.extractProductPayloadWithFallback(concCtx,
			"product translation", string(rowInput),
			p.TranslatePromptText, p.TranslatePromptRef,
			p.TranslateModelName, p.TranslateModelCfg)
		p.logLLMCall(ctx, fmt.Sprintf("%s_p3_t%d", eventID, b), "translate_products", 3, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.TranslatePromptRef), nil, err, callStart, p.Now())
		outcome := translateBatchOutcome{calls: 1}
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.TranslateModelName) && strings.TrimSpace(modelName) != "" {
			outcome.fallbacks = 1
		}
		if err != nil {
			if isCtxStopped(concCtx) {
				return outcome, ErrPipelineStopped
			}
			p.Logger.Warn("translate product batch failed; keeping batch untranslated", "error", err, "batch", b, "batch_size", end-start)
			return outcome, nil
		}
		raw, _ := payload["products"].([]any)
		if len(raw) != end-start {
			p.Logger.Warn("translate product batch returned mismatched row count; applying rows that can be matched by idx",
				"batch", b, "want", end-start, "got", len(raw), "raw", raw)
		}
		for _, item := range raw {
			first, _ := item.(map[string]any)
			if first == nil {
				continue
			}
			idxNum, ok := first["idx"].(float64)
			if !ok {
				p.Logger.Warn("translate product batch item missing valid idx; skipping", "batch", b, "item", first)
				continue
			}
			idx := int(idxNum)
			if idx < 0 || idx >= end-start {
				p.Logger.Warn("translate product batch item idx out of range; skipping", "batch", b, "idx", idx, "batch_size", end-start)
				continue
			}
			row := products[start+idx]
			for _, key := range []string{"product_name_en", "canonical_name_en", "product_summary_en", "requirement_text_en", "confidence_reason_en"} {
				switch key {
				case "product_summary_en":
					row["relation_summary_en"] = strings.TrimSpace(asString(first[key]))
				default:
					row[key] = strings.TrimSpace(asString(first[key]))
				}
			}
		}
		return outcome, nil
	})
	for _, o := range outcomes {
		*llmCallCount += o.calls
		*fallbackCount += o.fallbacks
	}
	if err != nil && (isCtxStopped(ctx) || errors.Is(err, ErrPipelineStopped)) {
		return products, ErrPipelineStopped
	}
	return products, nil
}

// resolveProductNamesAndCategories resolves each row's product_name against
// kb.product_names (see product_names_resolve.go) and, on a match, copies
// that row's catalog category (sub_catalog/category_l1/category_l2) into
// category_paths -- replacing the old free-form LLM categorization pass.
// A 'proposed' (unmatched) name has no catalog category, so category_paths
// stays empty for it; that is the intended signal that it awaits curation,
// not a failure. Resolution is a handful of plain SQL lookups per distinct
// name (no LLM), run concurrently (p.MaxTasks) across distinct names.
func (p *ProductsProcessor) resolveProductNamesAndCategories(ctx context.Context, products []map[string]any) ([]map[string]any, error) {
	if len(products) == 0 {
		return products, nil
	}
	uniqueNames := make([]string, 0, len(products))
	rowsByName := make(map[string][]int, len(products))
	for i, row := range products {
		name := strings.TrimSpace(asString(row["product_name"]))
		if name == "" {
			continue
		}
		if _, ok := rowsByName[name]; !ok {
			uniqueNames = append(uniqueNames, name)
		}
		rowsByName[name] = append(rowsByName[name], i)
	}
	if len(uniqueNames) == 0 {
		return products, nil
	}

	maxTasks := p.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 1
	}
	resolutions, err := runConcurrent(ctx, maxTasks, len(uniqueNames), func(concCtx context.Context, i int) (productNameResolution, error) {
		if isCtxStopped(concCtx) {
			return productNameResolution{}, ErrPipelineStopped
		}
		name := uniqueNames[i]
		var nameEN string
		if rows := rowsByName[name]; len(rows) > 0 {
			nameEN = strings.TrimSpace(asString(products[rows[0]]["product_name_en"]))
		}
		return p.resolveProductName(concCtx, name, nameEN), nil
	})
	if err != nil {
		if isCtxStopped(ctx) || errors.Is(err, ErrPipelineStopped) {
			return products, ErrPipelineStopped
		}
		return products, nil
	}

	for i, name := range uniqueNames {
		res := resolutions[i]
		paths := categoryPathsFromCatalog(res)
		for _, rowIdx := range rowsByName[name] {
			row := products[rowIdx]
			if res.ID > 0 {
				row["product_name_id"] = res.ID
			}
			// Authoritative, not additive: Pass 2's own prompt asks the LLM
			// for its own free-form category_paths/category_paths_en (see
			// prompt-enrich-product-mention-v2.md's "Extract Category Paths"
			// section) -- this pass replaces that with the catalog lookup
			// regardless, including clearing it to nil/empty when there is no
			// catalog match, rather than only overwriting on a hit and
			// silently leaving the LLM's guess in place on a miss.
			if len(paths) > 0 {
				row["category_paths"] = paths
			} else {
				delete(row, "category_paths")
			}
			delete(row, "category_paths_en")
		}
	}
	return products, nil
}

// categoryPathsFromCatalog builds the existing category_paths shape
// (semantic-chunking.go's CategoryPathEntry/CategoryPathNode) from a
// kb.product_names catalog match, so downstream consumers (indexProductsInTree)
// need no changes. Confidence is 1.0 throughout: this is a deterministic
// catalog lookup, not a model guess. Returns nil when the matched row (or a
// newly proposed one) carries no catalog category.
func categoryPathsFromCatalog(res productNameResolution) []CategoryPathEntry {
	var nodes []CategoryPathNode
	for _, name := range []string{res.SubCatalog, res.CategoryL1, res.CategoryL2} {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		nodes = append(nodes, CategoryPathNode{Name: name, Confidence: 1.0})
	}
	if len(nodes) == 0 {
		return nil
	}
	return []CategoryPathEntry{{Nodes: nodes, PathConfidence: 1.0}}
}

func (p *ProductsProcessor) buildProductOutputRows(products []map[string]any, now time.Time, numBlocks int, modelName string) []map[string]any {
	out := make([]map[string]any, 0, len(products))
	timeText := now.Format(defaultDocMetaStatusTime)
	for _, product := range products {
		out = append(out, map[string]any{
			"product_name":         strings.TrimSpace(asString(product["product_name"])),
			"product_name_en":      strings.TrimSpace(asString(product["product_name_en"])),
			"canonical_name":       strings.TrimSpace(asString(product["canonical_name"])),
			"canonical_name_en":    strings.TrimSpace(asString(product["canonical_name_en"])),
			"product_type":         strings.TrimSpace(asString(product["product_type"])),
			"relation_type":        strings.TrimSpace(asString(product["relation_type"])),
			"relation_summary":     strings.TrimSpace(asString(product["relation_summary"])),
			"relation_summary_en":  strings.TrimSpace(asString(product["relation_summary_en"])),
			"evidence_quote":       strings.TrimSpace(asString(product["evidence_quote"])),
			"evidence_lines":       product["evidence_lines"],
			"obligation_level":     strings.TrimSpace(asString(product["obligation_level"])),
			"requirement_text":     strings.TrimSpace(asString(product["requirement_text"])),
			"requirement_text_en":  strings.TrimSpace(asString(product["requirement_text_en"])),
			"conditions":           product["conditions"],
			"exceptions":           product["exceptions"],
			"parameters":           product["parameters"],
			"related_products":     product["related_products"],
			"responsible_actor":    strings.TrimSpace(asString(product["responsible_actor"])),
			"confidence":           toFloat(product["confidence"]),
			"confidence_reason":    strings.TrimSpace(asString(product["confidence_reason"])),
			"confidence_reason_en": strings.TrimSpace(asString(product["confidence_reason_en"])),
			"category_paths":       product["category_paths"],
			"category_paths_en":    product["category_paths_en"],
			"product_name_id":      product["product_name_id"],
			"status":               "active",
			"model_name":           strings.TrimSpace(firstNonEmptyTrimmed(modelName, p.ModelName)),
			"prompt_name":          strings.TrimSpace(p.PromptRef),
			"num_blocks":           numBlocks,
			"create_time":          timeText,
			"modify_time":          timeText,
			"public_info":          map[string]any{},
			"private_info":         map[string]any{},
			"notes":                "",
			"error_msg":            "",
		})
	}
	return out
}

func (p *ProductsProcessor) indexProductsInTree(recordID int64, products []map[string]any) error {
	dir := strings.TrimSpace(p.ArtifactWebDir)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR"))
	}
	if dir == "" {
		return errors.New("(MID_26052040) missing ARTIFACT_WEB_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052041) invalid record_id: %d", recordID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("(MID_26052042) create artifact web dir: %w", err)
	}
	if err := removeProductTreeRecord(dir, recordID); err != nil {
		return fmt.Errorf("(MID_26052043) remove old product tree entries for record %d: %w", recordID, err)
	}
	now := p.Now()
	for _, product := range products {
		productRelID := strings.TrimSpace(asString(product["product_rel_id"]))
		if productRelID == "" {
			continue
		}
		for _, pair := range pairCategoryPathEntries(product["category_paths"], product["category_paths_en"]) {
			if err := writeProductTreeEntry(p.Logger, dir, productRelID, pair.Index, pair.Original, now); err != nil {
				return fmt.Errorf("(MID_26052044) index product %s path: %w", productRelID, err)
			}
		}
	}
	return nil
}

func writeProductTreeEntry(logger ApiTypes.JimoLogger, treeRootDir string, productRelID string, indexEntry CategoryPathEntry, originalEntry CategoryPathEntry, now time.Time) error {
	leafDir, err := categoryTreeLeafDirForEntry(logger, treeRootDir, indexEntry, originalEntry, now)
	if err != nil {
		return err
	}
	if leafDir == "" {
		return nil
	}
	return upsertProductToLeafDir(leafDir, productRelID)
}

func upsertProductToLeafDir(leafDir string, productRelID string) error {
	filePath := filepath.Join(leafDir, "products.txt")
	existing := make([]string, 0)
	if bs, err := os.ReadFile(filePath); err == nil {
		for _, row := range strings.Split(string(bs), "\n") {
			row = strings.TrimSpace(row)
			if row != "" {
				existing = append(existing, row)
			}
		}
	}
	existing = appendUniqueString(existing, productRelID)
	sort.Strings(existing)
	return os.WriteFile(filePath, []byte(strings.Join(existing, "\n")), 0o644)
}

func removeProductTreeRecord(treeRootDir string, recordID int64) error {
	prefix := strconv.FormatInt(recordID, 10) + "_"
	return filepath.WalkDir(treeRootDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "products.txt" {
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

func (p *ProductsProcessor) writeProductsArtifact(recordID int64, rec DocMetadataInputRecord, products []map[string]any) error {
	artifactDir := strings.TrimSpace(p.ArtifactDir)
	if artifactDir == "" {
		artifactDir = strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	}
	if artifactDir == "" {
		return errors.New("(MID_26052050) missing ARTIFACT_DIR")
	}
	if recordID <= 0 {
		return fmt.Errorf("(MID_26052051) invalid record_id: %d", recordID)
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
		return fmt.Errorf("(MID_26052052) create artifact dir: %w", err)
	}

	outPath := filepath.Join(outDir, filenameRoot+"_"+parserName+".products")
	fileRecords := make([]map[string]any, 0, len(products))
	for _, row := range products {
		fileRecords = append(fileRecords, buildProductFileRecord(row))
	}
	bs, err := json.MarshalIndent(fileRecords, "", "  ")
	if err != nil {
		return fmt.Errorf("(MID_26052053) marshal products: %w", err)
	}
	if err := os.WriteFile(outPath, bs, 0o644); err != nil {
		return fmt.Errorf("(MID_26052054) write products artifact: %w", err)
	}
	return nil
}

func buildProductFileRecord(row map[string]any) map[string]any {
	return map[string]any{
		"product_rel_id":       strings.TrimSpace(asString(row["product_rel_id"])),
		"product_name":         strings.TrimSpace(asString(row["product_name"])),
		"product_name_en":      strings.TrimSpace(asString(row["product_name_en"])),
		"canonical_name":       strings.TrimSpace(asString(row["canonical_name"])),
		"canonical_name_en":    strings.TrimSpace(asString(row["canonical_name_en"])),
		"product_type":         strings.TrimSpace(asString(row["product_type"])),
		"relation_type":        strings.TrimSpace(asString(row["relation_type"])),
		"relation_summary":     strings.TrimSpace(asString(row["relation_summary"])),
		"relation_summary_en":  strings.TrimSpace(asString(row["relation_summary_en"])),
		"evidence_quote":       strings.TrimSpace(asString(row["evidence_quote"])),
		"evidence_lines":       row["evidence_lines"],
		"obligation_level":     strings.TrimSpace(asString(row["obligation_level"])),
		"requirement_text":     strings.TrimSpace(asString(row["requirement_text"])),
		"requirement_text_en":  strings.TrimSpace(asString(row["requirement_text_en"])),
		"conditions":           row["conditions"],
		"exceptions":           row["exceptions"],
		"parameters":           row["parameters"],
		"related_products":     row["related_products"],
		"responsible_actor":    strings.TrimSpace(asString(row["responsible_actor"])),
		"confidence":           toFloat(row["confidence"]),
		"confidence_reason":    strings.TrimSpace(asString(row["confidence_reason"])),
		"confidence_reason_en": strings.TrimSpace(asString(row["confidence_reason_en"])),
		"category_paths":       row["category_paths"],
		"category_paths_en":    row["category_paths_en"],
		"model_name":           strings.TrimSpace(asString(row["model_name"])),
		"prompt_name":          strings.TrimSpace(asString(row["prompt_name"])),
		"create_time":          strings.TrimSpace(asString(row["create_time"])),
	}
}

type productsStatusParams struct {
	RecordID      int64
	FileType      string
	InputFilename string
	Start         time.Time
	DurationMs    int64
	ProcStatus    string
	ProcErr       error
}

func (p *ProductsProcessor) persistProductsStatus(ctx context.Context, rec DocMetadataInputRecord, start time.Time, procErr error) {
	errMsg := (*string)(nil)
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	statusRaw, err := appendProductsStatus(rec.StatusRaw, productsStatusParams{
		RecordID:      rec.ID,
		FileType:      detectProductsFileType(rec),
		InputFilename: strings.TrimSpace(rec.ResultFilename),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcErr:       procErr,
	})
	if err != nil {
		p.Logger.Error("failed building products status", "record_id", rec.ID, "error", err)
		return
	}
	if err := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); err != nil {
		p.Logger.Error("failed persisting products status", "record_id", rec.ID, "error", err)
	}
}

func detectProductsFileType(rec DocMetadataInputRecord) string {
	for _, candidate := range []string{rec.FileName, rec.StagingFilename, rec.ResultFilename} {
		ext := strings.ToLower(strings.TrimSpace(filepath.Ext(strings.TrimSpace(candidate))))
		if ext != "" {
			return strings.TrimPrefix(ext, ".")
		}
	}
	return ""
}

func (p *ProductsProcessor) stopAndPersistProducts(ctx context.Context, rec DocMetadataInputRecord, start time.Time) {
	statusRaw, err := appendProductsStatus(rec.StatusRaw, productsStatusParams{
		RecordID:      rec.ID,
		FileType:      detectProductsFileType(rec),
		InputFilename: strings.TrimSpace(rec.ResultFilename),
		Start:         start,
		DurationMs:    time.Since(start).Milliseconds(),
		ProcStatus:    "stopped",
	})
	if err != nil {
		p.Logger.Error("(MID_26052841) failed building products stopped status", "record_id", rec.ID, "error", err)
		return
	}
	if updateErr := p.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
	}); updateErr != nil {
		p.Logger.Error("(MID_26052842) failed persisting products stopped status", "record_id", rec.ID, "error", updateErr)
	}
	p.Logger.Info("(MID_26052843) extract_products stopped by user request", "record_id", rec.ID)
}

func appendProductsStatus(raw string, p productsStatusParams) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"record_id":      strconv.FormatInt(p.RecordID, 10),
		"file_type":      strings.ToLower(strings.TrimSpace(p.FileType)),
		"operation":      "extract_products",
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
		if op != "extract_products" && op != "extract-products" {
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
func loadProductsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	return loadProductPromptFromEnvKeys([]string{"ENRICH_PRODUCT_RELATIONS_PROMPT", "EXTRACT_PRODUCTS_PROMPT", "EXTRACT_PRODUCT_PROMPT"}, "prompt-enrich-product-relations-v1.md")
}
*/

func loadProductPromptFromEnvKeys(envKeys []string, defaultRef string) (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range envKeys {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = defaultRef
	}

	return loadPromptByRef(promptRef)
}

// loadPromptByRef resolves a prompt by its ref (file name or absolute path),
// searching the standard candidate locations. Shared by the env-based loader
// and by config-driven callers (e.g. the doc-review per-aspect config).
func loadPromptByRef(promptRef string) (promptText string, promptRefOut string, promptPath string, promptErr error) {
	promptRef = strings.TrimSpace(promptRef)
	if promptRef == "" {
		return "", "", "", fmt.Errorf("(MID_26052064) empty prompt ref")
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
			lastErr = fmt.Errorf("(MID_26052060) failed reading file. Path:%s, error:%w", candidate, err)
			continue
		}
		text := strings.TrimSpace(string(bs))
		if text == "" {
			return "", promptRef, candidate, fmt.Errorf("(MID_26052061) prompt file is empty")
		}
		return text, promptRef, candidate, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("(MID_26052062) no candidate path available")
	}
	return "", promptRef, "", fmt.Errorf("(MID_26052063) prompt file not found: %w", lastErr)
}

func loadModelConfigFromEnvKeys(modelRefEnvs []string, modelsFileEnv string) (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	modelRefValue := ""
	modelRefKey := ""
	for _, key := range modelRefEnvs {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			modelRefValue = value
			modelRefKey = key
			break
		}
	}
	if modelRefValue == "" {
		return "", "", structureModelConfig{}, fmt.Errorf("missing one of %s", strings.Join(modelRefEnvs, ", "))
	}
	_ = modelRefKey
	return loadModelConfigByRef(modelRefValue, modelsFileEnv)
}

// loadModelConfigByRef resolves a model profile by its ref (the key in the
// models definition file). Shared by the env-based loader and by config-driven
// callers (e.g. the doc-review per-aspect config, where the ref comes from the
// `model` field in doc-review.local.toml).
func loadModelConfigByRef(modelRef, modelsFileEnv string) (modelRefOut string, modelPath string, cfg structureModelConfig, err error) {
	modelRef = strings.TrimSpace(modelRef)
	if modelRef == "" {
		return "", "", structureModelConfig{}, fmt.Errorf("(MID_26042109) empty model ref")
	}
	modelPath, err = resolveModelsFilePath(modelsFileEnv)
	if err != nil {
		return modelRef, "", structureModelConfig{}, err
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042103) read %s failed: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042104) parse %s failed: %w", modelPath, err)
	}
	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042105) model %q not found in %s", modelRef, modelPath)
	}
	if strings.TrimSpace(modelDef.ModelName) == "" {
		return modelRef, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042106) model %q in %s missing model_name", modelRef, modelPath)
	}
	llmclients.RegisterModelBudget(modelDef)
	cfg = structureModelConfig{
		ProfileName:          modelRef,
		Host:                 strings.TrimSpace(modelDef.Host),
		ModelName:            strings.TrimSpace(modelDef.ModelName),
		APIKey:               strings.TrimSpace(modelDef.APIKey),
		BaseURL:              strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec:           modelDef.TimeoutSec,
		ThinkingType:         normalizeThinkingType(strings.TrimSpace(modelDef.ThinkingType)),
		MaxInflight:          modelDef.MaxInflight,
		MaxRequestsPerMinute: modelDef.MaxRequestsPerMinute,
		MaxTokensPerMinute:   modelDef.MaxTokensPerMinute,
		TokenReservePerCall:  modelDef.TokenReservePerCall,
	}
	return modelRef, modelPath, cfg, nil
}

func loadOptionalModelConfigFromEnvKeys(modelRefEnvs []string, modelsFileEnv string) (modelRef string, modelPath string, cfg structureModelConfig, err error) {
	for _, key := range modelRefEnvs {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return loadModelConfigFromEnvKeys(modelRefEnvs, modelsFileEnv)
		}
	}
	return "", "", structureModelConfig{}, nil
}

func (s ProductsSQLStore) ensureReady() error {
	if s.DB == nil {
		return fmt.Errorf("(MID_26052070) db is nil")
	}
	return nil
}

func (s ProductsSQLStore) ProductsExist(ctx context.Context, inputRecordID int64) (bool, error) {
	if err := s.ensureReady(); err != nil {
		return false, err
	}
	const q = `SELECT 1 FROM kb.products WHERE input_record_id = $1 LIMIT 1`
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

func (s ProductsSQLStore) DeleteProductsByInputRecordID(ctx context.Context, inputRecordID int64) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kb.products WHERE input_record_id = $1`, inputRecordID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s ProductsSQLStore) SaveProducts(ctx context.Context, req SaveProductsRequest) (int64, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	if len(req.Products) == 0 {
		return 0, nil
	}

	const stmt = `
INSERT INTO kb.products (
	input_record_id,
	product_rel_id,
	product_name,
	product_name_en,
	canonical_name,
	canonical_name_en,
	product_type,
	relation_type,
	relation_summary,
	relation_summary_en,
	evidence_quote,
	evidence_lines,
	obligation_level,
	requirement_text,
	requirement_text_en,
	conditions,
	exceptions,
	parameters,
	related_products,
	responsible_actor,
	confidence,
	confidence_reason,
	confidence_reason_en,
	category_paths,
	category_paths_en,
	status,
	model_name,
	prompt_name,
	public_info,
	private_info,
	notes,
	error_msg,
	product_name_id
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,$15,$16::jsonb,$17::jsonb,$18::jsonb,$19::jsonb,$20,$21,$22,$23,$24::jsonb,$25::jsonb,$26,$27,$28,$29::jsonb,$30::jsonb,$31,$32,$33
)
ON CONFLICT (input_record_id, product_rel_id) DO UPDATE SET
	product_name = EXCLUDED.product_name,
	product_name_en = EXCLUDED.product_name_en,
	canonical_name = EXCLUDED.canonical_name,
	canonical_name_en = EXCLUDED.canonical_name_en,
	product_type = EXCLUDED.product_type,
	relation_type = EXCLUDED.relation_type,
	relation_summary = EXCLUDED.relation_summary,
	relation_summary_en = EXCLUDED.relation_summary_en,
	evidence_quote = EXCLUDED.evidence_quote,
	evidence_lines = EXCLUDED.evidence_lines,
	obligation_level = EXCLUDED.obligation_level,
	requirement_text = EXCLUDED.requirement_text,
	requirement_text_en = EXCLUDED.requirement_text_en,
	conditions = EXCLUDED.conditions,
	exceptions = EXCLUDED.exceptions,
	parameters = EXCLUDED.parameters,
	related_products = EXCLUDED.related_products,
	responsible_actor = EXCLUDED.responsible_actor,
	confidence = EXCLUDED.confidence,
	confidence_reason = EXCLUDED.confidence_reason,
	confidence_reason_en = EXCLUDED.confidence_reason_en,
	category_paths = EXCLUDED.category_paths,
	category_paths_en = EXCLUDED.category_paths_en,
	status = EXCLUDED.status,
	model_name = EXCLUDED.model_name,
	prompt_name = EXCLUDED.prompt_name,
	public_info = EXCLUDED.public_info,
	private_info = EXCLUDED.private_info,
	notes = EXCLUDED.notes,
	error_msg = EXCLUDED.error_msg,
	product_name_id = EXCLUDED.product_name_id,
	modify_time = NOW()`

	var inserted int64
	for _, product := range req.Products {
		evidenceLinesJSON, _ := json.Marshal(product["evidence_lines"])
		conditionsJSON, _ := json.Marshal(product["conditions"])
		exceptionsJSON, _ := json.Marshal(product["exceptions"])
		parametersJSON, _ := json.Marshal(product["parameters"])
		relatedProductsJSON, _ := json.Marshal(product["related_products"])
		categoryPathsJSON, _ := json.Marshal(product["category_paths"])
		categoryPathsEnJSON, _ := json.Marshal(product["category_paths_en"])
		publicInfoJSON, _ := json.Marshal(product["public_info"])
		privateInfoJSON, _ := json.Marshal(product["private_info"])

		_, err := s.DB.ExecContext(ctx, stmt,
			req.InputRecordID, // $1
			strings.TrimSpace(asString(product["product_rel_id"])),                         // $2
			strings.TrimSpace(asString(product["product_name"])),                           // $3
			strings.TrimSpace(asString(product["product_name_en"])),                        // $4
			strings.TrimSpace(asString(product["canonical_name"])),                         // $5
			strings.TrimSpace(asString(product["canonical_name_en"])),                      // $6
			strings.TrimSpace(asString(product["product_type"])),                           // $7
			strings.TrimSpace(asString(product["relation_type"])),                          // $8
			strings.TrimSpace(asString(product["relation_summary"])),                       // $9
			strings.TrimSpace(asString(product["relation_summary_en"])),                    // $10
			strings.TrimSpace(asString(product["evidence_quote"])),                         // $11
			string(evidenceLinesJSON),                                                      // $12
			strings.TrimSpace(asString(product["obligation_level"])),                       // $13
			strings.TrimSpace(asString(product["requirement_text"])),                       // $14
			strings.TrimSpace(asString(product["requirement_text_en"])),                    // $15
			string(conditionsJSON),                                                         // $16
			string(exceptionsJSON),                                                         // $17
			string(parametersJSON),                                                         // $18
			string(relatedProductsJSON),                                                    // $19
			strings.TrimSpace(asString(product["responsible_actor"])),                      // $20
			toFloat(product["confidence"]),                                                 // $21
			strings.TrimSpace(asString(product["confidence_reason"])),                      // $22
			strings.TrimSpace(asString(product["confidence_reason_en"])),                   // $23
			string(categoryPathsJSON),                                                      // $24
			string(categoryPathsEnJSON),                                                    // $25
			strings.TrimSpace(firstNonEmptyTrimmed(asString(product["status"]), "active")), // $26
			strings.TrimSpace(asString(product["model_name"])),                             // $27
			strings.TrimSpace(asString(product["prompt_name"])),                            // $28
			string(publicInfoJSON),                                                         // $29
			string(privateInfoJSON),                                                        // $30
			strings.TrimSpace(asString(product["notes"])),                                  // $31
			strings.TrimSpace(asString(product["error_msg"])),                              // $32
			nullableProductNameID(product["product_name_id"]),                              // $33
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
