package docreviews

type llmJSONUsageReporter interface {
	LastJSONUsage() *LLMUsage
}

func reviewLLMCacheHitTokens(client LLMJSONExtractor) int {
	usage := lastReviewJSONUsage(client)
	if usage == nil {
		return 0
	}
	return usage.PromptCacheHitTokens
}

func reviewLLMCacheMissTokens(client LLMJSONExtractor) int {
	usage := lastReviewJSONUsage(client)
	if usage == nil {
		return 0
	}
	return usage.PromptCacheMissTokens
}

func lastReviewJSONUsage(client LLMJSONExtractor) *LLMUsage {
	reporter, ok := client.(llmJSONUsageReporter)
	if !ok {
		return nil
	}
	return reporter.LastJSONUsage()
}
