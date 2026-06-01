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
	CategorizePromptText string
	CategorizePromptRef  string
	CategorizePromptPath string
	CategorizePromptErr  error
	CategorizeModelRef   string
	CategorizeModelName  string
	CategorizeModelCfg   structureModelConfig
	CategorizeEnabled    bool
	FallbackModelRef     string
	FallbackModelCfgPath string
	FallbackModelErr     error
	FallbackModelName    string
	FallbackModelCfg     structureModelConfig
	BlockSize            int
	PrevOverlap          int
	NextOverlap          int
	RemoveTOC            bool
	ArtifactDir          string
	ArtifactWebDir       string
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
}

func NewProductsProcessor(inputStore DocMetadataStore, store ProductsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *ProductsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26052001")
	}
	mentionPromptText, mentionPromptRef, mentionPromptPath, mentionPromptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_PRODUCT_MENTIONS_PROMPT"},
		"prompt-extract-product-mentions-v1.md",
	)
	relationPromptText, relationPromptRef, relationPromptPath, relationPromptErr := loadProductPromptFromEnvKeys(
		[]string{"ENRICH_PRODUCT_RELATIONS_PROMPT", "EXTRACT_PRODUCTS_PROMPT", "EXTRACT_PRODUCT_PROMPT"},
		"prompt-enrich-product-relations-v1.md",
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
		"prompt-translate-products-v1.md",
	)
	categorizePromptText, categorizePromptRef, categorizePromptPath, categorizePromptErr := loadProductPromptFromEnvKeys(
		[]string{"CATEGORIZE_PRODUCTS_PROMPT"},
		"prompt-categorize-products-v1.md",
	)
	translateModelRef, _, translateModelCfg, translateModelErr := loadOptionalModelConfigFromEnvKeys(
		[]string{"TRANSLATE_PRODUCTS_MODEL_NAME", "ENRICH_PRODUCT_RELATIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	categorizeModelRef, _, categorizeModelCfg, categorizeModelErr := loadOptionalModelConfigFromEnvKeys(
		[]string{"CATEGORIZE_PRODUCTS_MODEL_NAME", "ENRICH_PRODUCT_RELATIONS_MODEL_NAME", "EXTRACT_PRODUCT_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_PRODUCT_MODEL_FALLBACK", "MODEL_DEF_FILE")
	applyStructureModelConfigToExtractor(extractor, relationModelCfg)
	prevOverlap, nextOverlap, removeTOC := blockingConfigFromViper()
	translateEnabled := translatePromptErr == nil && strings.TrimSpace(translatePromptText) != "" && translateModelErr == nil && strings.TrimSpace(translateModelCfg.ModelName) != ""
	categorizeEnabled := categorizePromptErr == nil && strings.TrimSpace(categorizePromptText) != "" && categorizeModelErr == nil && strings.TrimSpace(categorizeModelCfg.ModelName) != ""
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
		CategorizePromptText: categorizePromptText,
		CategorizePromptRef:  categorizePromptRef,
		CategorizePromptPath: categorizePromptPath,
		CategorizePromptErr:  categorizePromptErr,
		CategorizeModelRef:   categorizeModelRef,
		CategorizeModelName:  categorizeModelCfg.ModelName,
		CategorizeModelCfg:   categorizeModelCfg,
		CategorizeEnabled:    categorizeEnabled,
		FallbackModelRef:     fallbackModelRef,
		FallbackModelCfgPath: fallbackModelCfgPath,
		FallbackModelErr:     fallbackModelErr,
		FallbackModelName:    fallbackModelCfg.ModelName,
		FallbackModelCfg:     fallbackModelCfg,
		BlockSize:            envInt("INPUT_BLOCK_SIZE", DefaultBlockingBlockSize, 1),
		PrevOverlap:          prevOverlap,
		NextOverlap:          nextOverlap,
		RemoveTOC:            removeTOC,
		ArtifactDir:          strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
		ArtifactWebDir:       strings.TrimSpace(os.Getenv("ARTIFACT_WEB_DIR")),
	}
}

func (p *ProductsProcessor) Name() string { return "extract_products" }

func (p *ProductsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26052002) parse event payload: %w", err)
	}
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
			p.persistProductsStatus(ctx, rec, start, nil)
			return nil
		}
	}

	blocks, err := p.resolveProductBlocks(ctx, evt, rec)
	if err != nil {
		p.persistProductsStatus(ctx, rec, start, err)
		p.Logger.Error("resolveBlocks error", "error", err, "record_id", evt.RecordID)
		return nil
	}
	if len(blocks) == 0 {
		err := fmt.Errorf("(MID_26052007) no blocks found for record_id=%d", evt.RecordID)
		p.persistProductsStatus(ctx, rec, start, err)
		return nil
	}

	p.Logger.Info("Before calling LLM",
		"num_blocks", len(blocks),
		"record_id", evt.RecordID,
		"filename", inputFilename)

	result, err := p.extractProductsFromBlocksWithLLM(ctx, blocks)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			p.stopAndPersistProducts(context.Background(), rec, start)
			return ErrPipelineStopped
		}
		p.persistProductsStatus(ctx, rec, start, err)
		return nil
	}

	outputRows := p.buildProductOutputRows(result.Products, start, len(blocks), result.ModelName)
	for i := range outputRows {
		outputRows[i]["product_rel_id"] = fmt.Sprintf("%d_%d", evt.RecordID, i+1)
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

func (p *ProductsProcessor) resolveProductBlocks(ctx context.Context, evt LineFileGeneratedEvent, rec DocMetadataInputRecord) ([]Block, error) {
	if buf := BlockBufferFromContext(ctx); buf != nil {
		return buf.Blocks, nil
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		return nil, fmt.Errorf("(MID_26052010) resolve input file for record_id=%d: %w", evt.RecordID, err)
	}
	body, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("(MID_26052011) input file not exist: %s", inputPath)
		}
		return nil, fmt.Errorf("(MID_26052012) read input file: %w", err)
	}
	buf, err := buildBlocks(body, p.productBlockSize(), p.PrevOverlap, p.NextOverlap, p.RemoveTOC)
	if err != nil {
		return nil, fmt.Errorf("(MID_26052013) build blocks: %w", err)
	}
	return buf.Blocks, nil
}

func (p *ProductsProcessor) productBlockSize() int {
	if p.BlockSize >= 1 {
		return p.BlockSize
	}
	return DefaultBlockingBlockSize
}

func (p *ProductsProcessor) extractProductsFromBlocksWithLLM(ctx context.Context, blocks []Block) (productExtractionResult, error) {
	eventID := eventIDFromContext(ctx)
	mentions := make([]productMention, 0, len(blocks))
	usedMentionModel := strings.TrimSpace(p.MentionModelName)
	llmCallCount := 0
	fallbackCount := 0
	for idx, block := range blocks {
		if isCtxStopped(ctx) {
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, ErrPipelineStopped
		}
		callStart := p.Now()
		p.Logger.Info("extract product mentions - begin",
			"idx", idx,
			"total", len(blocks),
			"model_name", p.MentionModelName,
			"prompt_name", p.MentionPromptRef,
		)
		payload, modelName, err := p.extractProductPayloadWithFallback(ctx,
			"extract products", buildProductMentionsUserPrompt(block),
			p.MentionPromptText, p.MentionPromptRef,
			p.MentionModelName, p.MentionModelCfg)
		llmCallCount++
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.MentionModelName) && strings.TrimSpace(modelName) != "" {
			fallbackCount++
		}
		p.logLLMCall(ctx, fmt.Sprintf("%s_p1_b%d", eventID, idx), "extract_product_mentions", 1, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.MentionPromptRef), nil, err, callStart, p.Now())
		if err != nil {
			p.Logger.Error("failed extracting product mentions", "error", err)
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, fmt.Errorf("(MID_26052020) extract product mentions via llm: %w", err)
		}
		usedMentionModel = strings.TrimSpace(modelName)
		raw, _ := payload["mentions"].([]any)
		mentions = append(mentions, normalizeProductMentions(raw, block)...)
		p.Logger.Info("extract product mentions - end",
			"mentions_so_far", len(mentions),
			"model_name", p.MentionModelName,
			"prompt_name", p.MentionPromptRef,
			"ms_used", time.Since(callStart).Milliseconds())
	}

	candidates := mergeProductMentionCandidates(mentions)
	p.Logger.Info("Merged product mention candidates",
		"mentions_count", len(mentions),
		"candidate_count", len(candidates),
		"record_stage", "post_merge",
	)
	products := make([]map[string]any, 0, len(candidates))
	usedRelationModel := strings.TrimSpace(p.RelationModelName)
	for idx, candidate := range candidates {
		if isCtxStopped(ctx) {
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, ErrPipelineStopped
		}
		callStart := p.Now()
		p.Logger.Info("Start enriching product candidate",
			"idx", idx,
			"total", len(candidates),
			"candidate_id", candidate.CandidateID,
			"model_name", p.RelationModelName,
			"prompt_name", p.RelationPromptRef,
		)
		payload, modelName, err := p.extractProductPayloadWithFallback(ctx,
			"product relations", buildProductRelationUserPrompt(candidate),
			p.RelationPromptText, p.RelationPromptRef,
			p.RelationModelName, p.RelationModelCfg)
		llmCallCount++
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.RelationModelName) && strings.TrimSpace(modelName) != "" {
			fallbackCount++
		}
		p.logLLMCall(ctx, fmt.Sprintf("%s_p2_c%d", eventID, idx), "enrich_product_relations", 2, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.RelationPromptRef), nil, err, callStart, p.Now())
		if err != nil {
			p.Logger.Error("failed enriching product relations", "error", err, "candidate_id", candidate.CandidateID)
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, fmt.Errorf("(MID_26052020) enrich product relations via llm: %w", err)
		}
		usedRelationModel = strings.TrimSpace(modelName)
		raw, _ := payload["products"].([]any)
		normalized := normalizeProductList(raw)
		products = append(products, normalized...)
		p.Logger.Info("extract products - end",
			"candidate_id", candidate.CandidateID,
			"rows", len(normalized),
			"products_so_far", len(products),
			"ms_used", time.Since(callStart).Milliseconds())
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
	if p.CategorizeEnabled {
		p.Logger.Info("Starting product categorization pass",
			"row_count", len(products),
			"model_name", p.CategorizeModelName,
			"prompt_name", p.CategorizePromptRef,
		)
		var categorizeErr error
		products, categorizeErr = p.categorizeProductRows(ctx, products, eventID, &llmCallCount, &fallbackCount)
		if categorizeErr != nil {
			return productExtractionResult{LLMCallCount: llmCallCount, FallbackCount: fallbackCount, MentionsCount: len(mentions)}, categorizeErr
		}
	}
	return productExtractionResult{
		Products:      products,
		ModelName:     firstNonEmptyTrimmed(usedRelationModel, usedMentionModel, p.RelationModelName, p.ModelName),
		LLMCallCount:  llmCallCount,
		FallbackCount: fallbackCount,
		MentionsCount: len(mentions),
	}, nil
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
	p.Logger.Info("extract products - begin",
		"action", opr,
		"model", modelName,
		"prompt_name", promptRef,
	)

	in := llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  inputText,
	}
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

	p.Logger.Info("extract products - end",
		"action", opr,
		"ms_used", time.Since(startTime).Milliseconds())
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

func buildProductMentionsUserPrompt(block Block) string {
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
		"\n\nBlock index: " + strconv.Itoa(block.Index) +
		"\n\nInput block lines (JSON array):\n" + blockLinesToJSON(block.Lines)
}

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
		for _, mention := range b.mentions {
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
		out = append(out, productCandidate{
			CandidateID:        fmt.Sprintf("cand_%d", len(out)+1),
			ProductName:        productName,
			CanonicalName:      canonicalName,
			ProductTypeHint:    productType,
			SupportingMentions: supportMentions,
			SupportLines:       supportLines,
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

func dedupeFinalProductRows(products []map[string]any) []map[string]any {
	grouped := map[string]map[string]any{}
	order := make([]string, 0, len(products))
	for _, product := range products {
		key := strings.Join([]string{
			normalizedProductCandidateKey(asString(product["canonical_name"]), asString(product["product_name"])),
			strings.ToLower(strings.TrimSpace(asString(product["relation_type"]))),
			strings.ToLower(strings.TrimSpace(asString(product["requirement_text"]))),
		}, "|")
		if existing, ok := grouped[key]; ok {
			mergedLines := toStringSlice(existing["evidence_lines"])
			for _, line := range toStringSlice(product["evidence_lines"]) {
				mergedLines = appendUniqueString(mergedLines, line)
			}
			existing["evidence_lines"] = mergedLines
			if toFloat(product["confidence"]) > toFloat(existing["confidence"]) {
				existing["confidence"] = product["confidence"]
				existing["confidence_reason"] = product["confidence_reason"]
			}
			if strings.TrimSpace(asString(existing["evidence_quote"])) == "" {
				existing["evidence_quote"] = product["evidence_quote"]
			}
			continue
		}
		cloned := map[string]any{}
		for k, v := range product {
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
	rec := DocProcLogRecord{
		DocProcName:  p.Name(),
		ModelNames:   modelNames,
		PromptName:   promptName,
		Pass:         &pass,
		LLMCallID:    &callID,
		ActivityName: &activity,
		ArtifactJSON: artifactStr,
		Errors:       errStr,
		MSUsed:       int64Ptr(end.Sub(start).Milliseconds()),
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
	if p.CategorizeEnabled && strings.TrimSpace(p.CategorizeModelName) != "" {
		if !slices.Contains(modelNames, strings.TrimSpace(p.CategorizeModelName)) {
			modelNames = append(modelNames, strings.TrimSpace(p.CategorizeModelName))
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
	if err := p.ProcLogger.LogSummary(ctx, "extract_products", DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    modelNames,
		PromptName:    promptName,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}, "MID-26052810"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func (p *ProductsProcessor) translateProductRows(ctx context.Context, products []map[string]any, eventID string, llmCallCount *int, fallbackCount *int) ([]map[string]any, error) {
	for i := range products {
		if isCtxStopped(ctx) {
			return products, ErrPipelineStopped
		}
		callStart := p.Now()
		rowInput, _ := json.Marshal(map[string]any{
			"products": []map[string]any{{
				"product_name":      products[i]["product_name"],
				"canonical_name":    products[i]["canonical_name"],
				"product_summary":   products[i]["relation_summary"],
				"requirement_text":  products[i]["requirement_text"],
				"confidence_reason": products[i]["confidence_reason"],
			}},
		})
		payload, modelName, err := p.extractProductPayloadWithFallback(ctx,
			"product translation", string(rowInput),
			p.TranslatePromptText, p.TranslatePromptRef,
			p.TranslateModelName, p.TranslateModelCfg)
		*llmCallCount++
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.TranslateModelName) && strings.TrimSpace(modelName) != "" {
			*fallbackCount++
		}
		p.logLLMCall(ctx, fmt.Sprintf("%s_p3_t%d", eventID, i), "translate_products", 3, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.TranslatePromptRef), nil, err, callStart, p.Now())
		if err != nil {
			p.Logger.Warn("translate product row failed; keeping untranslated row", "error", err, "product_name", products[i]["product_name"])
			continue
		}
		raw, _ := payload["products"].([]any)
		if len(raw) == 0 {
			continue
		}
		first, _ := raw[0].(map[string]any)
		if first == nil {
			continue
		}
		for _, key := range []string{"product_name_en", "canonical_name_en", "product_summary_en", "requirement_text_en", "confidence_reason_en"} {
			switch key {
			case "product_summary_en":
				products[i]["relation_summary_en"] = strings.TrimSpace(asString(first[key]))
			default:
				products[i][key] = strings.TrimSpace(asString(first[key]))
			}
		}
	}
	return products, nil
}

func (p *ProductsProcessor) categorizeProductRows(ctx context.Context, products []map[string]any, eventID string, llmCallCount *int, fallbackCount *int) ([]map[string]any, error) {
	for i := range products {
		if isCtxStopped(ctx) {
			return products, ErrPipelineStopped
		}
		callStart := p.Now()
		rowInput, _ := json.Marshal(map[string]any{
			"products": []map[string]any{{
				"product_name":    products[i]["product_name"],
				"canonical_name":  products[i]["canonical_name"],
				"product_type":    products[i]["product_type"],
				"relation_type":   products[i]["relation_type"],
				"product_summary": products[i]["relation_summary"],
				"evidence_quote":  products[i]["evidence_quote"],
			}},
		})
		payload, modelName, err := p.extractProductPayloadWithFallback(ctx,
			"product categorize", string(rowInput),
			p.CategorizePromptText, p.CategorizePromptRef,
			p.CategorizeModelName, p.CategorizeModelCfg)
		*llmCallCount++
		if strings.TrimSpace(modelName) != strings.TrimSpace(p.CategorizeModelName) && strings.TrimSpace(modelName) != "" {
			*fallbackCount++
		}
		p.logLLMCall(ctx, fmt.Sprintf("%s_p4_c%d", eventID, i), "categorize_products", 4, []string{strings.TrimSpace(modelName)}, strings.TrimSpace(p.CategorizePromptRef), nil, err, callStart, p.Now())
		if err != nil {
			p.Logger.Warn("categorize product row failed; keeping uncategorized row", "error", err, "product_name", products[i]["product_name"])
			continue
		}
		raw, _ := payload["products"].([]any)
		if len(raw) == 0 {
			continue
		}
		first, _ := raw[0].(map[string]any)
		if first == nil {
			continue
		}
		products[i]["category_paths"] = first["category_paths"]
		products[i]["category_paths_en"] = first["category_paths_en"]
	}
	return products, nil
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
	modelPath, err = resolveModelsFilePath(modelsFileEnv)
	if err != nil {
		return modelRefValue, "", structureModelConfig{}, err
	}
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRefValue, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042103) read %s failed: %w", modelPath, err)
	}
	parsed := ApiTypes.LLMModelsFile{}
	if err := parseTOMLMap(raw, &parsed); err != nil {
		return modelRefValue, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042104) parse %s failed: %w", modelPath, err)
	}
	modelDef, ok := parsed[modelRefValue]
	if !ok {
		return modelRefValue, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042105) model %q from %s not found in %s", modelRefValue, modelRefKey, modelPath)
	}
	if strings.TrimSpace(modelDef.ModelName) == "" {
		return modelRefValue, modelPath, structureModelConfig{}, fmt.Errorf("(MID_26042106) model %q in %s missing model_name", modelRefValue, modelPath)
	}
	cfg = structureModelConfig{
		ModelName:    strings.TrimSpace(modelDef.ModelName),
		APIKey:       strings.TrimSpace(modelDef.APIKey),
		BaseURL:      strings.TrimSpace(modelDef.BaseURL),
		TimeoutSec:   modelDef.TimeoutSec,
		ThinkingType: normalizeThinkingType(strings.TrimSpace(modelDef.ThinkingType)),
	}
	return modelRefValue, modelPath, cfg, nil
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
	error_msg
) VALUES (
	$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,$15,$16::jsonb,$17::jsonb,$18::jsonb,$19::jsonb,$20,$21,$22,$23,$24::jsonb,$25::jsonb,$26,$27,$28,$29::jsonb,$30::jsonb,$31,$32
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
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
