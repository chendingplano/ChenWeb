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

func categoryResolveMaxConcurrency() int {
	if v := strings.TrimSpace(os.Getenv("CATEGORY_RESOLVE_MAX_CONCURRENCY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultCategoryResolveMaxConcurrency
}

// newMetricCategoryResolver builds the category resolver used by metric indexing,
// wiring the real LLM creator from environment configuration.
func newMetricCategoryResolver(db *sql.DB, logger ApiTypes.JimoLogger) *categoryResolver {
	// creator stays a nil interface (not a typed-nil pointer) when unavailable, so
	// the resolver's nil-creator guard fires instead of panicking on a miss.
	var creator categoryCreator
	if c, err := newLLMCategoryCreator(db, logger); err != nil {
		if logger != nil {
			logger.Warn("category resolver: LLM creator unavailable; new categories cannot be created",
				"env", "CREATE_ARTIFACT_CATEGORY_PROMPT/CREATE_ARTIFACT_CATEGORY_MODEL_NAME", "error", err.Error())
		}
	} else {
		creator = c
	}
	return newCategoryResolver(artifactCategoryRegistry{DB: db}, creator)
}

// llmCategoryCreator calls the CREATE_ARTIFACT_CATEGORY LLM to mint a new category
// ontology entry. Thinking is forced off, mirroring the metrics passes.
type llmCategoryCreator struct {
	extractor     LLMJSONExtractor
	promptRef	  string
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
		promptRef:	   promptRef,
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
	return parseCreateCategoryResponse(payload)
}

func (c *llmCategoryCreator) invoke(ctx context.Context, inputText, rawKey, categoryType, modelName string, cfg structureModelConfig) (map[string]any, error) {
	applyStructureModelConfigToExtractor(c.extractor, cfg)
	callID := ""
	if c.newLLMCallID != nil {
		callID = strings.TrimSpace(c.newLLMCallID())
	}
	c.logger.Info("Create Category start",
		"categoryType", categoryType,
		"rawKey", rawKey,
		"modelName", modelName,
		"promptName", c.promptRef,
	)
	start := time.Now()
	payload, err := c.extractor.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		PromptText: c.promptText,
		ModelName:  modelName,
		InputText:  inputText,
	})

	c.logger.Info("Create Category end  ",
		"categoryType", categoryType,
		"rawKey", rawKey,
		"ms_used", time.Since(start).Milliseconds(),
	)

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
	rec := DocProcLogRecord{
		CallReason:    "create_artifact_category",
		DocProcName:   "create_artifact_category",
		ModelNames:    []string{modelName},
		PromptName:    "CREATE_ARTIFACT_CATEGORY_PROMPT",
		LLMCallID:     nullableStringPtr(callID),
		ActivityName:  nullableStringPtr("create_artifact_category"),
		ArtifactJSON:  artifactStr,
		Errors:        errStr,
		ExtraInfoJSON: &extraStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
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
