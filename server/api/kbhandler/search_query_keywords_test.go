package kbhandler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

type stubKeywordExtractor struct {
	calls   int
	last    llmclients.JSONExtractionInput
	payload map[string]any
	err     error
	block   bool // wait for the context to end, as a slow model would
}

func (s *stubKeywordExtractor) ExtractJSON(ctx context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	s.calls++
	s.last = in
	if s.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.payload, s.err
}

// useStubKeywordExtractor turns extraction on with a stub model, prompt and client, and resets the
// shared state. It returns the model config the client factory received.
func useStubKeywordExtractor(t *testing.T, stub *stubKeywordExtractor) *ApiTypes.LLMModelDef {
	t.Helper()
	t.Setenv(envSearchKeywordsModel, "test-search-model")
	oldClient, oldModel, oldPrompt, oldNow, oldState := newSearchKeywordsClient, loadSearchKeywordsModel, loadSearchKeywordsPrompt, searchKeywordsNow, searchKeywords
	t.Cleanup(func() {
		newSearchKeywordsClient, loadSearchKeywordsModel, loadSearchKeywordsPrompt, searchKeywordsNow, searchKeywords = oldClient, oldModel, oldPrompt, oldNow, oldState
	})
	t.Setenv(envSearchKeywords, "on")
	got := &ApiTypes.LLMModelDef{}
	loadSearchKeywordsModel = func(string) (ApiTypes.LLMModelDef, error) {
		return ApiTypes.LLMModelDef{ModelName: "stub-model", ThinkingType: "enabled"}, nil
	}
	loadSearchKeywordsPrompt = func(string) (string, string, string, error) { return "prompt", "p", "p", nil }
	newSearchKeywordsClient = func(_ string, cfg ApiTypes.LLMModelDef, _ ApiTypes.JimoLogger) (metricJSONExtractor, error) {
		*got = cfg
		return stub, nil
	}
	searchKeywords = &searchKeywordsState{cache: map[string]searchKeywordsEntry{}}
	return got
}

func keywordPayload(pairs ...string) map[string]any {
	var items []any
	for i := 0; i+1 < len(pairs); i += 2 {
		items = append(items, map[string]any{"en": pairs[i], "zh": pairs[i+1]})
	}
	return map[string]any{"keywords": items}
}

func TestSearchQueryKeywordsOffMakesNoCall(t *testing.T) {
	stub := &stubKeywordExtractor{payload: keywordPayload("adrenaline", "肾上腺素")}
	useStubKeywordExtractor(t, stub)
	for _, v := range []string{"", "off", "yes"} {
		t.Setenv(envSearchKeywords, v)
		if kws := searchQueryKeywords(context.Background(), "adrenaline dose"); kws != nil {
			t.Fatalf("switch %q: keywords=%v", v, kws)
		}
	}
	if stub.calls != 0 {
		t.Fatalf("calls=%d", stub.calls)
	}
}

func TestSearchKeywordsRequiresModelEnvironment(t *testing.T) {
	t.Setenv(envSearchKeywordsModel, "  ")
	s := &searchKeywordsState{}
	if _, _, _, err := s.load(nil); err == nil || !strings.Contains(err.Error(), envSearchKeywordsModel) {
		t.Fatalf("expected missing model configuration error, got %v", err)
	}
}

func TestSearchQueryKeywordsExtractsWithoutReasoningAndCaches(t *testing.T) {
	stub := &stubKeywordExtractor{payload: keywordPayload("adrenaline", "肾上腺素", "child", "儿童")}
	cfg := useStubKeywordExtractor(t, stub)

	kws := searchQueryKeywords(context.Background(), "how much intramuscular adrenaline for a child")
	if strings.Join(kws, "|") != "adrenaline|肾上腺素|child|儿童" {
		t.Fatalf("keywords=%v", kws)
	}
	if cfg.ThinkingType != "disabled" {
		t.Fatalf("thinking_type=%q", cfg.ThinkingType)
	}
	searchQueryKeywords(context.Background(), "  how much intramuscular adrenaline for a child ")
	if stub.calls != 1 {
		t.Fatalf("repeated query should hit the cache; calls=%d", stub.calls)
	}
}

func TestSearchQueryKeywordsFailureFallsBackAndIsNotCached(t *testing.T) {
	stub := &stubKeywordExtractor{err: errors.New("402 Payment Required")}
	useStubKeywordExtractor(t, stub)
	for i := 0; i < 2; i++ {
		if kws := searchQueryKeywords(context.Background(), "cuff pressure"); kws != nil {
			t.Fatalf("keywords=%v", kws)
		}
	}
	if stub.calls != 2 {
		t.Fatalf("failures must not be cached; calls=%d", stub.calls)
	}

	stub.err, stub.payload = nil, map[string]any{"keywords": []any{"...", ""}}
	if kws := searchQueryKeywords(context.Background(), "cuff pressure"); kws != nil {
		t.Fatalf("nothing usable should fall back; keywords=%v", kws)
	}
}

func TestSearchQueryKeywordsTimeoutFallsBack(t *testing.T) {
	stub := &stubKeywordExtractor{block: true}
	useStubKeywordExtractor(t, stub)
	t.Setenv(envSearchKeywordsTimeoutMS, "20")
	start := time.Now()
	if kws := searchQueryKeywords(context.Background(), "cuff pressure"); kws != nil {
		t.Fatalf("keywords=%v", kws)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout not applied: %v", elapsed)
	}
}

func TestSearchQueryKeywordsLoadFailureFallsBack(t *testing.T) {
	stub := &stubKeywordExtractor{payload: keywordPayload("cuff", "袖带")}
	useStubKeywordExtractor(t, stub)
	loadSearchKeywordsModel = func(ref string) (ApiTypes.LLMModelDef, error) {
		return ApiTypes.LLMModelDef{}, fmt.Errorf("profile %q not found", ref)
	}
	if kws := searchQueryKeywords(context.Background(), "cuff pressure"); kws != nil || stub.calls != 0 {
		t.Fatalf("keywords=%v calls=%d", kws, stub.calls)
	}
}

func TestCleanSearchKeywords(t *testing.T) {
	long := strings.Repeat("a", searchKeywordsMaxRunes+1)
	got := cleanSearchKeywords([]any{
		map[string]any{"en": " MPE ", "zh": "最大允许误差"},
		map[string]any{"en": "MPE", "zh": ""},
		map[string]any{"en": "--", "zh": "；"},
		long,
		"IP65",
	})
	if strings.Join(got, "|") != "MPE|最大允许误差|IP65" {
		t.Fatalf("cleaned=%v", got)
	}

	var many []any
	for i := 0; i < 20; i++ {
		many = append(many, fmt.Sprintf("kw%d", i))
	}
	if n := len(cleanSearchKeywords(many)); n != searchKeywordsMaxStrings {
		t.Fatalf("kept %d", n)
	}
	if cleanSearchKeywords("not a list") != nil {
		t.Fatal("non-list input should yield nil")
	}
}

func TestSearchKeywordsCacheExpiresAndEvictsOldest(t *testing.T) {
	useStubKeywordExtractor(t, &stubKeywordExtractor{})
	now := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	searchKeywordsNow = func() time.Time { return now }

	searchKeywords.put("q", []string{"a"})
	now = now.Add(searchKeywordsCacheTTL + time.Minute)
	if _, ok := searchKeywords.get("q"); ok {
		t.Fatal("entry older than the TTL should expire")
	}

	for i := 0; i < searchKeywordsCacheSize; i++ {
		now = now.Add(time.Second)
		searchKeywords.put(fmt.Sprintf("q%d", i), []string{"a"})
	}
	searchKeywords.put("newest", []string{"a"})
	if len(searchKeywords.cache) != searchKeywordsCacheSize {
		t.Fatalf("cache size=%d", len(searchKeywords.cache))
	}
	if _, ok := searchKeywords.get("q0"); ok {
		t.Fatal("oldest entry should be evicted")
	}
	if _, ok := searchKeywords.get("newest"); !ok {
		t.Fatal("newest entry missing")
	}
}
