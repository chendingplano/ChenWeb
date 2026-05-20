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

type ProductsProcessor struct {
	InputStore           DocMetadataStore
	Store                ProductsStore
	Extractor            LLMJSONExtractor
	Logger               ApiTypes.JimoLogger
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
	Products  []map[string]any
	ModelName string
}

func NewProductsProcessor(inputStore DocMetadataStore, store ProductsStore, extractor LLMJSONExtractor, logger ApiTypes.JimoLogger) *ProductsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26052001")
	}
	promptText, promptRef, promptPath, promptErr := loadProductsPromptFromEnv()
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnv("EXTRACT_PRODUCT_MODEL_NAME", "EXTRACT_PRODUCT_MODELS_FILE")
	fallbackModelRef, fallbackModelCfgPath, fallbackModelCfg, fallbackModelErr := loadOptionalModelConfigFromEnv("EXTRACT_PRODUCT_MODEL_FALLBACK", "EXTRACT_PRODUCT_MODELS_FILE")
	applyStructureModelConfigToExtractor(extractor, modelCfg)
	prevOverlap, nextOverlap, removeTOC := blockingConfigFromViper()
	return &ProductsProcessor{
		InputStore:           inputStore,
		Store:                store,
		Extractor:            extractor,
		Logger:               logger,
		Now:                  time.Now,
		PromptText:           promptText,
		PromptRef:            promptRef,
		PromptPath:           promptPath,
		PromptErr:            promptErr,
		ModelRef:             modelRef,
		ModelCfgPath:         modelCfgPath,
		ModelErr:             modelErr,
		ModelName:            modelCfg.ModelName,
		ModelCfg:             modelCfg,
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
	p.Logger.Info("Extract Products handle event")
	start := p.Now()
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("(MID_26052002) parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}
	if p.PromptErr != nil {
		return fmt.Errorf("(MID_26052003) load products prompt %q: %w", p.PromptRef, p.PromptErr)
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
	if p.ModelErr != nil {
		p.Logger.Warn("products extraction skipped: model config error",
			"record_id", evt.RecordID, "model_ref", p.ModelRef, "error", p.ModelErr)
		p.persistProductsStatus(ctx, rec, start, p.ModelErr)
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
	p.Logger.Info("products extracted",
		"record_id", evt.RecordID,
		"inserted_rows", inserted,
		"products_count", len(outputRows),
		"blocks", len(blocks),
	)
	p.persistProductsStatus(ctx, rec, start, nil)
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
	products := make([]map[string]any, 0, len(blocks))
	usedModelName := strings.TrimSpace(p.ModelName)

	for idx, block := range blocks {
		startTime := time.Now()
		p.Logger.Info("Start extracting products",
			"idx", idx,
			"total", len(blocks),
			"model_name", p.ModelName,
			"prompt_name", p.PromptRef,
		)
		payload, modelName, err := p.extractProductPayloadWithFallback(ctx, block)
		if err != nil {
			p.Logger.Error("failed extracting products", "error", err)
			return productExtractionResult{}, fmt.Errorf("(MID_26052020) extract products via llm: %w", err)
		}
		usedModelName = strings.TrimSpace(modelName)
		raw, _ := payload["products"].([]any)
		products = append(products, normalizeProductList(raw)...)
		p.Logger.Info("LLM responded", 
			"number of products", len(products), 
			"model name", p.ModelName,
			"prompt name", p.PromptRef,
			"ms_used", time.Since(startTime).Milliseconds())
	}
	return productExtractionResult{
		Products:  products,
		ModelName: firstNonEmptyTrimmed(usedModelName, p.ModelName),
	}, nil
}

func (p *ProductsProcessor) extractProductPayloadWithFallback(ctx context.Context, block Block) (map[string]any, string, error) {
	primaryStart := time.Now()
	payload, err := p.extractProductPayload(ctx, block, p.ModelName, p.ModelCfg)
	if err == nil {
		return payload, strings.TrimSpace(p.ModelName), nil
	}

	var payloadVal any = "nil"
	if payload != nil {
		payloadVal = payload
	}
	p.Logger.Warn("primary LLM failed extracting products",
		"error", err,
		"model_name", p.ModelName,
		"fallback_model", p.FallbackModelName,
		"payload", payloadVal,
		"ms_used", time.Since(primaryStart).Milliseconds())

	fallbackModelName := strings.TrimSpace(p.FallbackModelName)
	if fallbackModelName == "" {
		return nil, strings.TrimSpace(p.ModelName),
			fmt.Errorf("(MID_26052021) extract products failed and fallback model not available, err:%w, model_name:%s", err, p.ModelName)
	}
	if p.FallbackModelErr != nil {
		return nil, fallbackModelName, fmt.Errorf("(MID_26052022) primary extraction failed and fallback model %q is unavailable: %w", p.FallbackModelRef, err)
	}

	p.Logger.Warn("primary products extraction failed; retrying fallback model",
		"primary_model", p.ModelName,
		"fallback_model", fallbackModelName,
		"error", err,
		"prompt_name", p.PromptRef,
	)

	payload, fallbackErr := p.extractProductPayload(ctx, block, fallbackModelName, p.FallbackModelCfg)
	if fallbackErr != nil {
		if isEmptyFallbackProductExtractionError(fallbackErr) {
			p.Logger.Warn("fallback products extraction returned empty JSON; treating as empty result",
				"fallback_model", fallbackModelName,
				"error", fallbackErr,
				"prompt_name", p.PromptRef,
			)
			return map[string]any{"products": []any{}}, fallbackModelName, nil
		}
		return nil, fallbackModelName, fmt.Errorf("(MID_26052023) primary extraction failed: %w; fallback extraction failed: %v", err, fallbackErr)
	}
	return payload, fallbackModelName, nil
}

func isEmptyFallbackProductExtractionError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.TrimSpace(err.Error())
	return strings.Contains(msg, "unexpected end of JSON input") &&
		strings.Contains(msg, "json:{[]}")
}

func (p *ProductsProcessor) extractProductPayload(ctx context.Context, block Block, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(p.Extractor, cfg)
	p.Logger.Info("To extract products",
		"model", modelName,
		"prompt_name", p.PromptRef,
	)

	payload, err := p.Extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: p.PromptText,
		ModelName:  modelName,
		InputText:  buildProductUserPrompt(block),
	})
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
	raw, ok := payload["products"]
	if !ok {
		return nil, fmt.Errorf("(MID_26052033) llm output field 'products' must be an array, JSON:%v", payload)
	}
	items, ok := normalizeProductItems(raw)
	if !ok {
		return nil, fmt.Errorf("(MID_26052034) llm output field 'products' must be an array, JSON:%v", payload)
	}
	payload["products"] = items
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

func buildProductUserPrompt(block Block) string {
	schema := map[string]any{
		"products": []map[string]any{{
			"product_name":        "string",
			"product_name_en":     "string or null",
			"canonical_name":      "string",
			"canonical_name_en":   "string",
			"product_type":        "specific_product|product_class|component|material|software|system|equipment|consumable|packaging|other",
			"relation_type":       "scope|regulated_object|requirement_target|performance_requirement|design_requirement|material_requirement|testing_requirement|certification_requirement|usage_condition|installation_requirement|maintenance_requirement|storage_requirement|prohibited_product|exempted_product|component_of|contains_product|compatible_with|replacement_or_alternative|measurement_object|risk_source|other",
			"product_summary":    "one concise sentence describing how the input relates to this product",
			"product_summary_en": "one concise sentence in English",
			"evidence_quote":      "short supporting quote from the input",
			"evidence_lines":      []string{"32", "35-45"},
			"relation_details": map[string]any{
				"obligation_level":         "mandatory|recommended|permitted|prohibited|conditional|descriptive|unknown",
				"requirement_text":         "string",
				"requirement_text_en":      "string or null",
				"conditions":               []string{"condition 1"},
				"exceptions":               []string{"exception 1"},
				"thresholds_or_parameters": []map[string]any{{"name": "string", "value": "string", "unit": "string or null"}},
				"related_products":         []map[string]any{{"product_name": "string", "relationship": "component_of|contains_product|compatible_with|replacement_or_alternative|compared_with|other"}},
				"responsible_actor":        "string or null",
			},
			"confidence":           0.0,
			"confidence_reason":    "brief reason",
			"confidence_reason_en": "brief reason in English",
			"discriminators": []map[string]any{{
				"intent":  "short interpretation of user need",
				"domain":  []string{"domain1", "domain2"},
				"discriminators": []map[string]any{{
					"category":   "lexical|synonym|abbreviation|metadata|structural|graph|heuristic",
					"value":      "string",
					"confidence": 0.0,
					"reason":     "why this helps discriminate",
				}},
				"exploration_plan": []string{"ordered recommended exploration steps"},
			}},
			"discriminators_en": []any{},
			"category_paths": []map[string]any{{
				"category_path": []map[string]any{{
					"name":       "string",
					"keywords":   []string{"string"},
					"confidence": 0.0,
				}},
				"path_keywords":   []string{"string"},
				"path_confidence": 0.0,
			}},
			"category_paths_en": []any{},
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

func (p *ProductsProcessor) buildProductOutputRows(products []map[string]any, now time.Time, numBlocks int, modelName string) []map[string]any {
	out := make([]map[string]any, 0, len(products))
	timeText := now.Format(defaultDocMetaStatusTime)
	for _, product := range products {
		out = append(out, map[string]any{
			"product_name":        strings.TrimSpace(asString(product["product_name"])),
			"product_name_en":     strings.TrimSpace(asString(product["product_name_en"])),
			"canonical_name":      strings.TrimSpace(asString(product["canonical_name"])),
			"canonical_name_en":   strings.TrimSpace(asString(product["canonical_name_en"])),
			"product_type":        strings.TrimSpace(asString(product["product_type"])),
			"relation_type":       strings.TrimSpace(asString(product["relation_type"])),
			"relation_summary":    strings.TrimSpace(asString(product["relation_summary"])),
			"relation_summary_en": strings.TrimSpace(asString(product["relation_summary_en"])),
			"evidence_quote":      strings.TrimSpace(asString(product["evidence_quote"])),
			"evidence_lines":      product["evidence_lines"],
			"obligation_level":    strings.TrimSpace(asString(product["obligation_level"])),
			"requirement_text":    strings.TrimSpace(asString(product["requirement_text"])),
			"requirement_text_en": strings.TrimSpace(asString(product["requirement_text_en"])),
			"conditions":          product["conditions"],
			"exceptions":          product["exceptions"],
			"parameters":          product["parameters"],
			"related_products":    product["related_products"],
			"responsible_actor":   strings.TrimSpace(asString(product["responsible_actor"])),
			"confidence":           toFloat(product["confidence"]),
			"confidence_reason":    strings.TrimSpace(asString(product["confidence_reason"])),
			"confidence_reason_en": strings.TrimSpace(asString(product["confidence_reason_en"])),
			"category_paths":       product["category_paths"],
			"category_paths_en":   product["category_paths_en"],
			"status":              "active",
			"model_name":          strings.TrimSpace(firstNonEmptyTrimmed(modelName, p.ModelName)),
			"prompt_name":         strings.TrimSpace(p.PromptRef),
			"num_blocks":          numBlocks,
			"create_time":         timeText,
			"modify_time":         timeText,
			"public_info":         map[string]any{},
			"private_info":        map[string]any{},
			"notes":               "",
			"error_msg":           "",
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
		if pathsRaw, ok := product["category_paths"].([]any); ok {
			for _, entry := range parseCategoryPathsArray(pathsRaw) {
				if err := writeProductTreeEntry(p.Logger, dir, productRelID, entry, now); err != nil {
					return fmt.Errorf("(MID_26052044) index product %s path: %w", productRelID, err)
				}
			}
		}
		if pathsEnRaw, ok := product["category_paths_en"].([]any); ok {
			for _, entry := range parseCategoryPathsArray(pathsEnRaw) {
				if err := writeProductTreeEntry(p.Logger, dir, productRelID, entry, now); err != nil {
					return fmt.Errorf("(MID_26052045) index product %s en path: %w", productRelID, err)
				}
			}
		}
	}
	return nil
}

func writeProductTreeEntry(logger ApiTypes.JimoLogger, treeRootDir string, productRelID string, pathEntry CategoryPathEntry, now time.Time) error {
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
	return upsertProductToLeafDir(currentDir, productRelID)
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
		"product_rel_id":      strings.TrimSpace(asString(row["product_rel_id"])),
		"product_name":        strings.TrimSpace(asString(row["product_name"])),
		"product_name_en":     strings.TrimSpace(asString(row["product_name_en"])),
		"canonical_name":      strings.TrimSpace(asString(row["canonical_name"])),
		"canonical_name_en":   strings.TrimSpace(asString(row["canonical_name_en"])),
		"product_type":        strings.TrimSpace(asString(row["product_type"])),
		"relation_type":       strings.TrimSpace(asString(row["relation_type"])),
		"relation_summary":    strings.TrimSpace(asString(row["relation_summary"])),
		"relation_summary_en": strings.TrimSpace(asString(row["relation_summary_en"])),
		"evidence_quote":      strings.TrimSpace(asString(row["evidence_quote"])),
		"evidence_lines":      row["evidence_lines"],
		"obligation_level":    strings.TrimSpace(asString(row["obligation_level"])),
		"requirement_text":    strings.TrimSpace(asString(row["requirement_text"])),
		"requirement_text_en": strings.TrimSpace(asString(row["requirement_text_en"])),
		"conditions":          row["conditions"],
		"exceptions":          row["exceptions"],
		"parameters":          row["parameters"],
		"related_products":    row["related_products"],
		"responsible_actor":   strings.TrimSpace(asString(row["responsible_actor"])),
		"confidence":           toFloat(row["confidence"]),
		"confidence_reason":    strings.TrimSpace(asString(row["confidence_reason"])),
		"confidence_reason_en": strings.TrimSpace(asString(row["confidence_reason_en"])),
		"category_paths":       row["category_paths"],
		"category_paths_en":   row["category_paths_en"],
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

func loadProductsPromptFromEnv() (promptText string, promptRef string, promptPath string, promptErr error) {
	for _, key := range []string{"EXTRACT_PRODUCTS_PROMPT", "EXTRACT_PRODUCT_PROMPT"} {
		promptRef = strings.TrimSpace(os.Getenv(key))
		if promptRef != "" {
			break
		}
	}
	if promptRef == "" {
		promptRef = "prompt-extract-products.md"
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
			req.InputRecordID,                                                               // $1
			strings.TrimSpace(asString(product["product_rel_id"])),                         // $2
			strings.TrimSpace(asString(product["product_name"])),                            // $3
			strings.TrimSpace(asString(product["product_name_en"])),                         // $4
			strings.TrimSpace(asString(product["canonical_name"])),                          // $5
			strings.TrimSpace(asString(product["canonical_name_en"])),                       // $6
			strings.TrimSpace(asString(product["product_type"])),                            // $7
			strings.TrimSpace(asString(product["relation_type"])),                           // $8
			strings.TrimSpace(asString(product["relation_summary"])),                        // $9
			strings.TrimSpace(asString(product["relation_summary_en"])),                     // $10
			strings.TrimSpace(asString(product["evidence_quote"])),                          // $11
			string(evidenceLinesJSON),                                                       // $12
			strings.TrimSpace(asString(product["obligation_level"])),                        // $13
			strings.TrimSpace(asString(product["requirement_text"])),                        // $14
			strings.TrimSpace(asString(product["requirement_text_en"])),                     // $15
			string(conditionsJSON),                                                          // $16
			string(exceptionsJSON),                                                          // $17
			string(parametersJSON),                                                          // $18
			string(relatedProductsJSON),                                                     // $19
			strings.TrimSpace(asString(product["responsible_actor"])),                       // $20
			toFloat(product["confidence"]),                                                  // $21
			strings.TrimSpace(asString(product["confidence_reason"])),                       // $22
			strings.TrimSpace(asString(product["confidence_reason_en"])),                    // $23
			string(categoryPathsJSON),                                                       // $24
			string(categoryPathsEnJSON),                                                     // $25
			strings.TrimSpace(firstNonEmptyTrimmed(asString(product["status"]), "active")),  // $26
			strings.TrimSpace(asString(product["model_name"])),                              // $27
			strings.TrimSpace(asString(product["prompt_name"])),                             // $28
			string(publicInfoJSON),                                                          // $29
			string(privateInfoJSON),                                                         // $30
			strings.TrimSpace(asString(product["notes"])),                                   // $31
			strings.TrimSpace(asString(product["error_msg"])),                               // $32
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}
