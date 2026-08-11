package docprocessing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	ontologycandidates "github.com/chendingplano/deepdoc/server/api/ontology/candidates"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type MetricDefinitionsProcessor struct {
	Extractor     LLMJSONExtractor
	CandidateSink OntologyCandidateSink
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

func NewMetricDefinitionsProcessor(e LLMJSONExtractor) *MetricDefinitionsProcessor {
	promptText, promptRef, promptPath, promptErr := loadProductPromptFromEnvKeys(
		[]string{"EXTRACT_METRIC_DEFINITIONS_PROMPT"},
		"prompt-extract-metric-definitions-v2.md",
	)
	p := &MetricDefinitionsProcessor{Extractor: e, ModelName: strings.TrimSpace(os.Getenv("EXTRACT_METRIC_DEFINITIONS_MODEL_NAME")), PromptText: promptText, PromptRef: promptRef, PromptPath: promptPath, PromptErr: promptErr}
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
	if p.Extractor == nil || p.CandidateSink == nil || p.ModelName == "" || p.PromptErr != nil {
		return fmt.Errorf("%s is not configured", p.Name())
	}
	p.batchRecordID, p.batchChunks, p.batchDocCtx, p.mentions = id, ch, doc, nil
	return nil
}
func (p *MetricDefinitionsProcessor) ProcessChunk(ctx context.Context, i int) error {
	if i < 0 || i >= len(p.batchChunks) {
		return fmt.Errorf("%s chunk index out of range", p.Name())
	}
	raw, err := p.Extractor.ExtractJSON(ctx, newLLMJSONInput(ctx, p.Name(), p.PromptText, p.ModelName, canonicalChunkInputText(p.batchChunks[i].Lines, p.batchDocCtx), p.Name(), "P4-METRIC-DEFINITIONS"))
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.mentions = append(p.mentions, parseMetricDefinitionMentions(raw)...)
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
