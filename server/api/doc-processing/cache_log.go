package docprocessing

import (
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// llmJSONUsageReporter is implemented by LLM clients (e.g. *llmclients.OpenAIJSONClient)
// that expose the provider usage from their most recent JSON call, including the DeepSeek
// prompt-cache counters. Mirrors docreviews.llmJSONUsageReporter.
type llmJSONUsageReporter interface {
	LastJSONUsage() *llmclients.Usage
}

// extractorCacheTokens reads the provider prompt-cache counters from the most recent LLM
// call made through extractor, for stamping onto a kb.doc_proc_logs llm_call entry.
//
// It must be called immediately after the LLM call whose tokens are being recorded, because
// LastJSONUsage reflects only the client's last call (per-client, last-call state). When the
// extractor does not report usage, both results are nil so the log columns stay NULL.
//
// See ADR 2026062501 (DeepSeek prompt cache) — the same counters persisted to llm_usage_event
// are now also surfaced on the doc-processing log.
func extractorCacheTokens(extractor any) (hit, miss *int64) {
	reporter, ok := extractor.(llmJSONUsageReporter)
	if !ok {
		return nil, nil
	}
	usage := reporter.LastJSONUsage()
	if usage == nil {
		return nil, nil
	}
	h := int64(usage.PromptCacheHitTokens)
	m := int64(usage.PromptCacheMissTokens)
	return &h, &m
}
