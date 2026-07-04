package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// defaultCategoryResolveMaxConcurrency bounds concurrent LLM creates
// per ResolveBatch call (env CATEGORY_RESOLVE_MAX_CONCURRENCY). Set to 1 for fully
// serial behavior (debugging or rate-limited model endpoints).
const defaultCategoryResolveMaxConcurrency = 8
const defaultCreateCategoryMode = "not-use-llm"

func categoryResolveMaxConcurrency() int {
	if v := strings.TrimSpace(os.Getenv("CATEGORY_RESOLVE_MAX_CONCURRENCY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultCategoryResolveMaxConcurrency
}

func createCategoryMode() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("CREATE_CATEGORY_MODE")), "use-llm") {
		return "use-llm"
	}
	return defaultCreateCategoryMode
}

// newMetricCategoryResolver builds the category resolver used by metric indexing,
// wiring the real LLM creator from environment configuration.
func newMetricCategoryResolver(db *sql.DB, logger ApiTypes.JimoLogger) *categoryResolver {
	mode := createCategoryMode()
	var creator categoryCreator = &deterministicCategoryCreator{logger: logger}
	if mode == "use-llm" {
		// creator stays a nil interface (not a typed-nil pointer) when unavailable, so
		// the resolver's nil-creator guard fires instead of panicking on a miss.
		creator = nil
		if c, err := newLLMCategoryCreator(db, logger); err != nil {
			if logger != nil {
				logger.Warn("category resolver: LLM creator unavailable; new categories cannot be created",
					"env", "CREATE_ARTIFACT_CATEGORY_PROMPT/CREATE_ARTIFACT_CATEGORY_MODEL_NAME",
					"mode", mode,
					"error", err.Error())
			}
		} else {
			creator = c
		}
	}
	return newCategoryResolver(artifactCategoryRegistry{DB: db}, creator)
}

type deterministicCategoryCreator struct {
	logger ApiTypes.JimoLogger
}

func (c *deterministicCategoryCreator) CreateCategory(_ context.Context, rawKey, categoryType string, _ map[string]any) (createdCategory, error) {
	start := time.Now()
	normKey := normalizeCategoryKey(rawKey)
	if normKey == "" {
		return createdCategory{}, fmt.Errorf("(MID_26070301) empty category key")
	}
	displayName := strings.TrimSpace(rawKey)
	c.logCreateStart(categoryType, rawKey, defaultCreateCategoryMode, "")
	cat := createdCategory{CategoryKey: normKey}
	if displayName != "" {
		cat.DisplayNames = uniqueStrings([]string{displayName})
	}
	c.logCreateEnd(categoryType, rawKey, start)
	return cat, nil
}

func (c *deterministicCategoryCreator) logCreateStart(categoryType, rawKey, mode, modelName string) {
	if c.logger == nil {
		return
	}
	args := []any{
		"categoryType", categoryType,
		"rawKey", rawKey,
		"mode", mode,
	}
	if strings.TrimSpace(modelName) != "" {
		args = append(args, "modelName", modelName)
	}
	c.logger.Info("Create Category start", args...)
}

func (c *deterministicCategoryCreator) logCreateEnd(categoryType, rawKey string, start time.Time) {
	if c.logger == nil {
		return
	}
	c.logger.Info("Create Category end  ",
		"categoryType", categoryType,
		"rawKey", rawKey,
		"ms_used", time.Since(start).Milliseconds(),
	)
}

// llmCategoryCreator calls the CREATE_ARTIFACT_CATEGORY LLM to mint a new category
// ontology entry. Thinking is forced off, mirroring the metrics passes.
type llmCategoryCreator struct {
	extractor     LLMJSONExtractor
	promptRef     string
	promptText    string
	modelName     string
	modelCfg      structureModelConfig
	fallbackModel string
	fallbackCfg   structureModelConfig
	procLogger    DocProcLogger
	logger        ApiTypes.JimoLogger
	newLLMCallID  func() string
}

func newLLMCategoryCreator(db *sql.DB, logger ApiTypes.JimoLogger) (*llmCategoryCreator, error) {
	promptText, promptRef, _, promptErr := loadProductPromptFromEnvKeys([]string{"CREATE_ARTIFACT_CATEGORY_PROMPT"}, "")
	if promptErr != nil {
		return nil, promptErr
	}
	_, _, modelCfg, modelErr := loadModelConfigFromEnvKeys([]string{"CREATE_ARTIFACT_CATEGORY_MODEL_NAME"}, "MODEL_DEF_FILE")
	if modelErr != nil {
		return nil, modelErr
	}
	_, _, fallbackCfg, _ := loadOptionalModelConfigFromEnv("CREATE_ARTIFACT_CATEGORY_FALLBACK", "MODEL_DEF_FILE")

	timeoutSec := modelCfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 100
	}
	extractor := &llmclients.OpenAIJSONClient{
		HTTPClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
	return &llmCategoryCreator{
		extractor:     extractor,
		promptRef:     promptRef,
		promptText:    promptText,
		modelName:     modelCfg.ModelName,
		modelCfg:      forceDisableThinking(modelCfg),
		fallbackModel: fallbackCfg.ModelName,
		fallbackCfg:   forceDisableThinking(fallbackCfg),
		procLogger:    DocProcLogger{DB: db},
		logger:        logger,
		newLLMCallID:  defaultCategoryLLMCallID,
	}, nil
}

func (c *llmCategoryCreator) CreateCategory(ctx context.Context, rawKey, categoryType string, evidence map[string]any) (createdCategory, error) {
	input := map[string]any{
		"category_type":    categoryType,
		"raw_category_key": rawKey,
		"context":          evidence,
	}
	inputJSON, _ := json.Marshal(input)

	payload, err := c.invoke(ctx, string(inputJSON), rawKey, categoryType, c.modelName, c.modelCfg)
	if err != nil && strings.TrimSpace(c.fallbackModel) != "" {
		payload, err = c.invoke(ctx, string(inputJSON), rawKey, categoryType, c.fallbackModel, c.fallbackCfg)
	}
	if err != nil {
		return createdCategory{}, err
	}
	cat, parseErr := parseCreateCategoryResponse(payload)
	if parseErr != nil {
		// LLM returned a structurally valid response but omitted category_key/canonical_key
		// (e.g. a bare translations dict). Fall back to the normalized raw key so the
		// category is still created with minimal metadata rather than being silently dropped.
		if c.logger != nil {
			c.logger.Warn("category creator: LLM response missing key, falling back to raw key",
				"raw_key", rawKey, "category_type", categoryType, "error", parseErr.Error())
		}
		cat = createdCategory{CategoryKey: normalizeCategoryKey(rawKey)}
	}
	return cat, nil
}

func (c *llmCategoryCreator) invoke(ctx context.Context, inputText, rawKey, categoryType, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(c.extractor, cfg)
	promptName := firstNonEmptyTrimmed(c.promptRef, "CREATE_ARTIFACT_CATEGORY_PROMPT")
	callID := ""
	if c.newLLMCallID != nil {
		callID = strings.TrimSpace(c.newLLMCallID())
	}
	deterministicLogger := deterministicCategoryCreator{logger: c.logger}
	deterministicLogger.logCreateStart(categoryType, rawKey, "use-llm", modelName)
	start := time.Now()
	catIn := newLLMJSONInput(
		ctx,
		promptName,
		c.promptText,
		modelName,
		inputText,
		"create_artifact_category",
		"MID-CWB-CREATE-ARTIFACT-CATEGORY",
	)
	// Category creation is task-first, not document-first: the large repeated content here is
	// the prompt template (identical every call), while the per-call input (category key +
	// context) is small and varies. Keeping the template as the system prefix lets DeepSeek
	// reuse it across the many category calls in a run. See ADR 2026062701.
	catIn.DocumentFirst = false
	payload, err := c.extractor.ExtractJSON(ctx, catIn)

	deterministicLogger.logCreateEnd(categoryType, rawKey, start)

	c.logCategoryLLMCall(ctx, callID, rawKey, categoryType, modelName, inputText, payload, err, start, time.Now())
	return payload, err
}

func defaultCategoryLLMCallID() string {
	return fmt.Sprintf("create_artifact_category_%d", time.Now().UnixNano())
}

func (c *llmCategoryCreator) logCategoryLLMCall(
	ctx context.Context,
	callID string,
	rawKey string,
	categoryType string,
	modelName string,
	inputText string,
	payload map[string]any,
	callErr error,
	start time.Time,
	end time.Time,
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
	extraInfo := map[string]any{
		"raw_category_key": rawKey,
		"category_type":    categoryType,
		"input_text":       inputText,
	}
	extraBytes, _ := json.Marshal(extraInfo)
	extraStr := string(extraBytes)
	cacheHit, cacheMiss := extractorCacheTokens(c.extractor)
	rec := DocProcLogRecord{
		CallReason:            "create_artifact_category",
		DocProcName:           "create_artifact_category",
		ModelNames:            []string{modelName},
		PromptName:            "CREATE_ARTIFACT_CATEGORY_PROMPT",
		LLMCallID:             nullableStringPtr(callID),
		ActivityName:          nullableStringPtr(fmt.Sprintf("category_%s", categoryType)),
		ArtifactJSON:          artifactStr,
		Errors:                errStr,
		ExtraInfoJSON:         &extraStr,
		MSUsed:                int64Ptr(end.Sub(start).Milliseconds()),
		PromptCacheHitTokens:  cacheHit,
		PromptCacheMissTokens: cacheMiss,
	}
	logErr := c.procLogger.LogLLMCall(ctx, rec, "MID-26060501")
	if logErr != nil {
		c.logger.Error("failed log llm call", "error", logErr, "record", rec)
	}

	// c.printCategoryLLMDebugLog(callID, rawKey, categoryType, modelName, rec, logErr == nil, logErr)
}

/*
func (c *llmCategoryCreator) printCategoryLLMDebugLog(
	callID string,
	rawKey string,
	categoryType string,
	modelName string,
	rec DocProcLogRecord,
	logInserted bool,
	logErr error,
) {
	fields := []any{
		"call_id", callID,
		"raw_category_key", rawKey,
		"category_type", categoryType,
		"model_name", modelName,
		"log_inserted", logInserted,
	}
	// if rec.ArtifactJSON != nil {
	// 	fields = append(fields, "artifact_json", *rec.ArtifactJSON)
	// }
	if rec.Errors != nil {
		fields = append(fields, "llm_error", *rec.Errors)
	}
	if rec.ExtraInfoJSON != nil {
		fields = append(fields, "extra_info", *rec.ExtraInfoJSON)
	}
	if logErr != nil {
		fields = append(fields, "log_error", logErr.Error())
	}
	if c.logger != nil {
		c.logger.Info("artifact category llm_call", fields...)
		return
	}
	fmt.Printf("artifact category llm_call: %+v\n", fields)
}
*/
