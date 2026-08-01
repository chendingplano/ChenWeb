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

const testMethodsPrompt = `Extract explicitly stated test or measurement procedures. Return JSON {"procedures":[{"procedure_name":"string","definition":"string","metric_names":["string"],"source_line_spans":["line"]}]}. Do not infer procedures or metric links; use only source lines.`

// TestMethodsProcessor is the ADR §8.2 routed, chunk-based procedure harvester.
// It produces review candidates only; activation remains the release workflow.
type TestMethodsProcessor struct {
	Extractor     LLMJSONExtractor
	CandidateSink OntologyCandidateSink
	ModelName     string
	batchRecordID int64
	batchChunks   []Chunk
	batchDocCtx   string
	batchMentions []testMethodMention
	mu            sync.Mutex
}

func NewTestMethodsProcessor(extractor LLMJSONExtractor) *TestMethodsProcessor {
	p := &TestMethodsProcessor{Extractor: extractor, ModelName: strings.TrimSpace(os.Getenv("EXTRACT_TEST_METHODS_MODEL_NAME"))}
	if ApiTypes.ProjectDBHandle != nil {
		p.CandidateSink = ontologycandidates.CandidateStore{DB: ApiTypes.ProjectDBHandle}
	}
	return p
}
func (p *TestMethodsProcessor) Name() string { return "extract_test_methods" }
func (p *TestMethodsProcessor) HandleEvent(context.Context, []byte) error {
	return fmt.Errorf("%s requires chunk batching", p.Name())
}
func (p *TestMethodsProcessor) InitChunkBatch(_ context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.Extractor == nil || p.CandidateSink == nil || p.ModelName == "" {
		return fmt.Errorf("%s is not configured", p.Name())
	}
	p.batchRecordID, p.batchChunks, p.batchDocCtx, p.batchMentions = recordID, chunks, docCtx, nil
	return nil
}
func (p *TestMethodsProcessor) ProcessChunk(ctx context.Context, idx int) error {
	if idx < 0 || idx >= len(p.batchChunks) {
		return fmt.Errorf("%s chunk index out of range", p.Name())
	}
	payload, err := p.Extractor.ExtractJSON(ctx, newLLMJSONInput(ctx, p.Name(), testMethodsPrompt, p.ModelName, canonicalChunkInputText(p.batchChunks[idx].Lines, p.batchDocCtx), p.Name(), "P4-TEST-METHODS"))
	if err != nil {
		return err
	}
	p.mu.Lock()
	p.batchMentions = append(p.batchMentions, parseTestMethodMentions(payload)...)
	p.mu.Unlock()
	return nil
}
func (p *TestMethodsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	for _, mention := range p.batchMentions {
		cs, err := buildTestMethodCandidates(p.batchRecordID, mention)
		if err != nil {
			return err
		}
		for _, c := range cs {
			if _, err := p.CandidateSink.CreateCandidate(ctx, c); err != nil {
				return err
			}
		}
	}
	return nil
}
