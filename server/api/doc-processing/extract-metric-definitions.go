package docprocessing

import (
	"context"
	"fmt"
	"sync"
	"time"

	ontologycandidates "github.com/chendingplano/deepdoc/server/api/ontology/candidates"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type MetricDefinitionsProcessor struct {
	Extractor     LLMJSONExtractor
	CandidateSink OntologyCandidateSink
	Logger        ApiTypes.JimoLogger
	ModelRef      string
	ModelCfgPath  string
	ModelErr      error
	ModelName     string
	PromptText    string
	PromptRef     string
	PromptPath    string
	PromptErr     error
	batchRecordID int64
	batchChunks   []Chunk
	batchDocCtx   string
	mentions      []metricDefinitionMention
	mu            sync.Mutex
}

func NewMetricDefinitionsProcessor(e LLMJSONExtractor, logger ApiTypes.JimoLogger) *MetricDefinitionsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26081101")
	}
	promptText, promptRef, promptPath, promptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_METRIC_DEFINITIONS_PROMPT"},
		"prompt-extract-metric-definitions-v2.md",
	)
	modelRef, modelCfgPath, modelCfg, modelErr := loadModelConfigFromEnvKeys(
		[]string{"EXTRACT_METRIC_DEFINITIONS_MODEL_NAME"},
		"MODEL_DEF_FILE",
	)
	applyStructureModelConfigToExtractor(e, modelCfg)
	p := &MetricDefinitionsProcessor{Extractor: e, Logger: logger, ModelRef: modelRef, ModelCfgPath: modelCfgPath, ModelErr: modelErr, ModelName: modelCfg.ModelName, PromptText: promptText, PromptRef: promptRef, PromptPath: promptPath, PromptErr: promptErr}
	if ApiTypes.ProjectDBHandle != nil {
		p.CandidateSink = ontologycandidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	}
	return p
}
func (p *MetricDefinitionsProcessor) Name() string { return "extract_metric_definitions" }
func (p *MetricDefinitionsProcessor) HandleEvent(context.Context, []byte) error {
	return fmt.Errorf("%s requires chunk batching", p.Name())
}
func (p *MetricDefinitionsProcessor) InitChunkBatch(_ context.Context, id int64, ch []Chunk, doc string) error {
	if p.Extractor == nil || p.CandidateSink == nil || p.ModelName == "" || p.PromptErr != nil || p.ModelErr != nil {
		return fmt.Errorf("%s is not configured", p.Name())
	}
	p.batchRecordID, p.batchChunks, p.batchDocCtx, p.mentions = id, ch, doc, nil
	return nil
}
func (p *MetricDefinitionsProcessor) ProcessChunk(ctx context.Context, i int) error {
	if i < 0 || i >= len(p.batchChunks) {
		return fmt.Errorf("%s chunk index out of range", p.Name())
	}
	callStart := time.Now()
	p.Logger.Info("extract metric definitions start (batch)",
		"record_id", p.batchRecordID,
		"chunk", i,
		"total", len(p.batchChunks),
		"model_name", p.ModelName,
		"prompt_name", p.PromptRef,
	)
	raw, err := p.Extractor.ExtractJSON(ctx, newLLMJSONInput(ctx, p.Name(), p.PromptText, p.ModelName, canonicalChunkInputText(p.batchChunks[i].Lines, p.batchDocCtx), p.Name(), "P4-METRIC-DEFINITIONS"))
	if err != nil {
		p.Logger.Warn("extract metric definitions end (batch)",
			"record_id", p.batchRecordID,
			"chunk", i,
			"ms_used", time.Since(callStart).Milliseconds(),
			"error", err,
		)
		return err
	}
	mentions := parseMetricDefinitionMentions(raw)
	cacheHit, cacheMiss := cacheTokenCounts(p.Extractor)
	p.Logger.Info("extract metric definitions end (batch)",
		"record_id", p.batchRecordID,
		"chunk", i,
		"extracted_definitions", len(mentions),
		"cache_hit", cacheHit,
		"cache_miss", cacheMiss,
		"ms_used", time.Since(callStart).Milliseconds(),
	)
	p.mu.Lock()
	p.mentions = append(p.mentions, mentions...)
	p.mu.Unlock()
	return nil
}
func (p *MetricDefinitionsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	for _, m := range p.mentions {
		c, err := buildMetricDefinitionCandidate(p.batchRecordID, m)
		if err != nil {
			return err
		}
		if _, err = p.CandidateSink.CreateCandidate(ctx, c); err != nil {
			return err
		}
	}
	return nil
}
