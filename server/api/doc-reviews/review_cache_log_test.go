package docreviews

import (
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestReviewLLMCacheTokenHelpersReadLastJSONUsage(t *testing.T) {
	fake := &fakeJSONExtractor{
		lastUsage: &llmclients.Usage{
			PromptCacheHitTokens:  321,
			PromptCacheMissTokens: 45,
		},
	}

	if got := reviewLLMCacheHitTokens(fake); got != 321 {
		t.Fatalf("hit tokens=%d, want 321", got)
	}
	if got := reviewLLMCacheMissTokens(fake); got != 45 {
		t.Fatalf("miss tokens=%d, want 45", got)
	}
}

func TestReviewLLMCacheTokenHelpersDefaultToZero(t *testing.T) {
	fake := &fakeJSONExtractor{}

	if got := reviewLLMCacheHitTokens(fake); got != 0 {
		t.Fatalf("hit tokens=%d, want 0", got)
	}
	if got := reviewLLMCacheMissTokens(fake); got != 0 {
		t.Fatalf("miss tokens=%d, want 0", got)
	}
}
