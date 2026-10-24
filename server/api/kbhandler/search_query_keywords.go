package kbhandler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// Query keyword extraction for the BM25 side of artifact search (openspec search-query-keywords).
// An LLM turns the query into keywords (English and Chinese); the ParadeDB clause then requires each
// keyword to match in full instead of matching any word of the raw query. Any failure returns nil,
// which means "search with the raw query", so search never fails because of this step.
const (
	envSearchKeywords          = "SEARCH_QUERY_KEYWORDS"
	envSearchKeywordsModel     = "SEARCH_QUERY_KEYWORDS_MODEL_NAME"
	envSearchKeywordsPrompt    = "SEARCH_QUERY_KEYWORDS_PROMPT"
	envSearchKeywordsTimeoutMS = "SEARCH_QUERY_KEYWORDS_TIMEOUT_MS"

	defaultSearchKeywordsModel   = "deepseek-flash-processor"
	defaultSearchKeywordsPrompt  = "prompt-search-query-keywords-v1.md"
	defaultSearchKeywordsTimeout = 3 * time.Second

	searchKeywordsMaxStrings = 12
	searchKeywordsMaxRunes   = 64
	searchKeywordsCacheTTL   = 24 * time.Hour
	searchKeywordsCacheSize  = 2000
)

var (
	// Replaced in tests.
	newSearchKeywordsClient = func(modelRef string, cfg ApiTypes.LLMModelDef, logger ApiTypes.JimoLogger) (metricJSONExtractor, error) {
		return defaultNewExtractMetricsClient(modelRef, cfg, logger)
	}
	loadSearchKeywordsModel  = loadModelDefByRef
	loadSearchKeywordsPrompt = docprocessing.LoadPromptByRef
	searchKeywordsNow        = time.Now

	searchKeywords = &searchKeywordsState{cache: map[string]searchKeywordsEntry{}}
)

type searchKeywordsEntry struct {
	keywords []string
	storedAt time.Time
}

type searchKeywordsState struct {
	mu           sync.Mutex
	modelRef     string
	promptRef    string
	modelName    string
	promptText   string
	client       metricJSONExtractor
	cache        map[string]searchKeywordsEntry
	warnedSwitch string
}

// searchQueryKeywords returns the cleaned keywords for a search query, or nil when extraction is
// off or does not succeed (the caller then uses the raw query).
func searchQueryKeywords(ctx context.Context, query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	logger := loggerutil.CreateDefaultLogger("20261024-617")
	if !searchKeywordsEnabled(logger) {
		return nil
	}
	if kws, ok := searchKeywords.get(query); ok {
		logger.Debug("search query keywords cache hit", "query_len", len([]rune(query)), "keywords", len(kws))
		return kws
	}
	client, modelName, promptText, err := searchKeywords.load(logger)
	if err != nil {
		logger.Warn("search query keywords fallback", "reason", "load_failed", "error", err)
		return nil
	}

	cctx, cancel := context.WithTimeout(ctx, searchKeywordsTimeout())
	defer cancel()
	start := searchKeywordsNow()
	call := searchCallFrom(ctx)
	payload, err := client.ExtractJSON(cctx, llmclients.JSONExtractionInput{
		UserID:     call.userID,
		RunID:      call.runID,
		PromptName: llmclients.EnsurePromptName("search_query_keywords_prompt", "search_query_keywords", "MID-20261024-01", modelName),
		PromptText: promptText,
		ModelName:  modelName,
		InputText:  query,
		CallReason: "search_query_keywords",
		CallLoc:    "MID-20261024-01",
	})
	msUsed := searchKeywordsNow().Sub(start).Milliseconds()
	if err != nil {
		reason := "llm_error"
		if errors.Is(cctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		logger.Warn("search query keywords fallback", "reason", reason, "ms_used", msUsed, "model_name", modelName, "error", err)
		return nil
	}
	kws := cleanSearchKeywords(payload["keywords"])
	if len(kws) == 0 {
		logger.Warn("search query keywords fallback", "reason", "empty", "ms_used", msUsed, "model_name", modelName)
		return nil
	}
	searchKeywords.put(query, kws)
	logger.Info("search query keywords extracted", "query_len", len([]rune(query)), "keywords", len(kws),
		"ms_used", msUsed, "model_name", modelName)
	return kws
}

// searchKeywordsEnabled reads SEARCH_QUERY_KEYWORDS: "on" or "off"; anything else is off, with one
// warning per distinct bad value.
func searchKeywordsEnabled(logger ApiTypes.JimoLogger) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(envSearchKeywords)))
	switch v {
	case "on":
		return true
	case "", "off":
		return false
	}
	searchKeywords.mu.Lock()
	warn := searchKeywords.warnedSwitch != v
	searchKeywords.warnedSwitch = v
	searchKeywords.mu.Unlock()
	if warn {
		logger.Warn("invalid search query keywords switch; treated as off", "env", envSearchKeywords, "value", v)
	}
	return false
}

func searchKeywordsTimeout() time.Duration {
	if ms, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envSearchKeywordsTimeoutMS))); err == nil && ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return defaultSearchKeywordsTimeout
}

// load returns the client, model name and prompt, (re)building them when the configured model or
// prompt changes. Reasoning is always disabled: the call is on the search's critical path.
func (s *searchKeywordsState) load(logger ApiTypes.JimoLogger) (metricJSONExtractor, string, string, error) {
	modelRef := envOrDefault(envSearchKeywordsModel, defaultSearchKeywordsModel)
	promptRef := envOrDefault(envSearchKeywordsPrompt, defaultSearchKeywordsPrompt)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client != nil && s.modelRef == modelRef && s.promptRef == promptRef {
		return s.client, s.modelName, s.promptText, nil
	}
	cfg, err := loadSearchKeywordsModel(modelRef)
	if err != nil {
		return nil, "", "", fmt.Errorf("model %q: %w", modelRef, err)
	}
	cfg.ThinkingType = "disabled"
	promptText, _, _, err := loadSearchKeywordsPrompt(promptRef)
	if err != nil {
		return nil, "", "", fmt.Errorf("prompt %q: %w", promptRef, err)
	}
	client, err := newSearchKeywordsClient(modelRef, cfg, logger)
	if err != nil {
		return nil, "", "", fmt.Errorf("client for %q: %w", modelRef, err)
	}
	s.modelRef, s.promptRef, s.modelName, s.promptText, s.client = modelRef, promptRef, cfg.ModelName, promptText, client
	return client, cfg.ModelName, promptText, nil
}

func (s *searchKeywordsState) get(query string) ([]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[query]
	if !ok {
		return nil, false
	}
	if searchKeywordsNow().Sub(e.storedAt) > searchKeywordsCacheTTL {
		delete(s.cache, query)
		return nil, false
	}
	return e.keywords, true
}

// put stores a successful extraction, evicting the oldest entry when the cache is full.
func (s *searchKeywordsState) put(query string, kws []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.cache[query]; !exists && len(s.cache) >= searchKeywordsCacheSize {
		oldest, oldestAt := "", time.Time{}
		for k, e := range s.cache {
			if oldest == "" || e.storedAt.Before(oldestAt) {
				oldest, oldestAt = k, e.storedAt
			}
		}
		delete(s.cache, oldest)
	}
	s.cache[query] = searchKeywordsEntry{keywords: kws, storedAt: searchKeywordsNow()}
}

// cleanSearchKeywords flattens [{"en","zh"}] (or plain strings) into at most 12 distinct strings,
// dropping empty ones, ones longer than 64 characters and ones without a letter or digit (they
// produce no BM25 tokens).
func cleanSearchKeywords(raw any) []string {
	items, _ := raw.([]any)
	var out []string
	seen := map[string]bool{}
	add := func(v any) {
		s, _ := v.(string)
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || len([]rune(s)) > searchKeywordsMaxRunes || !strings.ContainsFunc(s, isKeywordRune) {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, it := range items {
		switch v := it.(type) {
		case map[string]any:
			add(v["en"])
			add(v["zh"])
		default:
			add(v)
		}
	}
	if len(out) > searchKeywordsMaxStrings {
		out = out[:searchKeywordsMaxStrings]
	}
	return out
}

// isKeywordRune covers letters in any script (including CJK) and digits.
func isKeywordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
